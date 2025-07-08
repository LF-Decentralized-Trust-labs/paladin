import { randomBytes } from "crypto";
import { ethers } from "ethers";
import {
  IGroupInfo,
  IGroupInfoUnresolved,
  TransactionType,
} from "../interfaces";
import {
  IPrivacyGroup,
  IPrivacyGroupEVMCall,
  IPrivacyGroupEVMTXInput,
  IPrivacyGroupResume,
} from "../interfaces/privacygroups";
import PaladinClient from "../paladin";
import { PaladinVerifier } from "../verifier";
import * as penteJSON from "./abis/PentePrivacyGroup.json";
import { TransactionWrapper } from "../transaction";

export interface PenteGroupTransactionInput {
  from: string;
  methodAbi: ethers.JsonFragment;
  to: string;
  data: {
    [key: string]: any;
  };
}

export interface PenteContractTransactionInput {
  from: string;
  function: string;
  data?: {
    [key: string]: any;
  };
}

export interface PenteDeploy {
  abi: ReadonlyArray<ethers.JsonFragment>;
  bytecode: string;
  from: string;
  inputs?: any;
}

export const penteGroupABI = {
  name: "group",
  type: "tuple",
  components: [
    { name: "salt", type: "bytes32" },
    { name: "members", type: "string[]" },
  ],
};

export interface PentePrivacyGroupParams {
  members: (string | PaladinVerifier)[];
  salt?: string;
  evmVersion?: string;
  endorsementType?: string;
  externalCallsEnabled?: boolean;
  additionalProperties?: {
    [x: string]: unknown;
  };
}

export interface PenteApproveTransitionParams {
  txId: string;
  delegate: string;
  transitionHash: string;
  signatures: string[];
}

export const newGroupSalt = () =>
  "0x" + Buffer.from(randomBytes(32)).toString("hex");

export const resolveGroup = (
  group: IGroupInfo | IGroupInfoUnresolved
): IGroupInfo => {
  const members: string[] = [];
  for (const member of group.members) {
    if (typeof member === "string") {
      members.push(member);
    } else {
      members.push(member.lookup);
    }
  }
  return { members, salt: group.salt };
};

export class PentePrivacyGroupWrapper {
  public tx: Promise<TransactionWrapper | undefined>;

  constructor(
    private paladin: PaladinClient,
    private group: IPrivacyGroup | Promise<IPrivacyGroup>
  ) {
    this.tx = Promise.resolve(group).then((group) =>
      group.genesisTransaction
        ? new TransactionWrapper(paladin, group.genesisTransaction)
        : undefined
    );
  }

  async waitForReceipt(waitMs?: number, full = false) {
    const tx = await this.tx;
    return tx?.waitForReceipt(waitMs, full);
  }

  async waitForDeploy(waitMs?: number) {
    const group = await this.group;
    const receipt = await this.waitForReceipt(waitMs);
    group.contractAddress = receipt?.contractAddress;
    return group.contractAddress
      ? new PentePrivacyGroup(this.paladin, group)
      : undefined;
  }
}

export class PenteFactory {
  constructor(private paladin: PaladinClient, public readonly domain: string) {}

  using(paladin: PaladinClient) {
    return new PenteFactory(paladin, this.domain);
  }

  newPrivacyGroup(input: PentePrivacyGroupParams) {
    return new PentePrivacyGroupWrapper(
      this.paladin,
      this.paladin.createPrivacyGroup({
        domain: this.domain,
        members: input.members.map((m) => m.toString()),
        configuration: {
          evmVersion: input.evmVersion,
          endorsementType: input.endorsementType,
          externalCallsEnabled:
            input.externalCallsEnabled === true
              ? "true"
              : input.externalCallsEnabled === false
              ? "false"
              : undefined,
        },
      })
    );
  }

  async resumePrivacyGroup(input: IPrivacyGroupResume) {
    const existingGroup = await this.paladin.getPrivacyGroupById(
      this.domain,
      input.id
    );
    return existingGroup.contractAddress === undefined
      ? undefined
      : new PentePrivacyGroup(this.paladin, existingGroup);
  }
}

export class PentePrivacyGroup {
  public readonly address: string;
  public readonly salt: string;
  public readonly members: string[];

  constructor(
    private paladin: PaladinClient,
    public readonly group: IPrivacyGroup
  ) {
    if (group.contractAddress === undefined) {
      throw new Error(
        `Supplied group '${group.id}' is missing a contract address. Check transaction ${group.genesisTransaction}`
      );
    }
    this.address = group.contractAddress;
    this.salt = group.id; // when bypassing privacy group helper functionality, and directly building Pente private transactions
    this.members = group.members;
  }

  using(paladin: PaladinClient) {
    return new PentePrivacyGroup(paladin, this.group);
  }

  deploy(params: PenteDeploy, txOptions?: Partial<IPrivacyGroupEVMTXInput>) {
    // Find the constructor in the ABI
    const constructor: ethers.JsonFragment = params.abi.find(
      (entry) => entry.type === "constructor"
    ) || { type: "constructor", inputs: [] };

    const transaction: IPrivacyGroupEVMTXInput = {
      ...txOptions,
      domain: this.group.domain,
      group: this.group.id,
      from: params.from,
      input: params.inputs,
      bytecode: params.bytecode,
      function: constructor,
    };

    return new PentePrivateDeployWrapper(
      this.paladin,
      this.paladin.sendPrivacyGroupTransaction(transaction)
    );
  }

  // sendTransaction functions in the contract (write)
  sendTransaction(
    transaction: PenteGroupTransactionInput,
    txOptions?: Partial<IPrivacyGroupEVMTXInput>
  ) {
    return new TransactionWrapper(
      this.paladin,
      this.paladin.sendPrivacyGroupTransaction({
        ...txOptions,
        domain: this.group.domain,
        group: this.group.id,
        from: transaction.from,
        to: transaction.to,
        input: transaction.data,
        function: transaction.methodAbi,
      })
    );
  }

  // call functions in the contract (read-only)
  async call(
    transaction: PenteGroupTransactionInput,
    txOptions?: Partial<IPrivacyGroupEVMCall>
  ) {
    return this.paladin.callPrivacyGroup({
      ...txOptions,
      domain: this.group.domain,
      group: this.group.id,
      from: transaction.from || "sdk.default",
      to: transaction.to,
      input: transaction.data,
      function: transaction.methodAbi,
    });
  }

  async approveTransition(
    from: PaladinVerifier,
    data: PenteApproveTransitionParams
  ) {
    return this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: penteJSON.abi,
      function: "approveTransition",
      to: this.address,
      from: from.lookup,
      data,
    });
  }
}

export class PentePrivateDeployWrapper extends TransactionWrapper {
  async waitForDeploy(waitMs?: number) {
    const receipt = await this.waitForReceipt(waitMs, true);
    return receipt?.domainReceipt !== undefined &&
      "receipt" in receipt.domainReceipt
      ? receipt.domainReceipt.receipt.contractAddress
      : undefined;
  }
}

export abstract class PentePrivateContract<ConstructorParams> {
  constructor(
    protected evm: PentePrivacyGroup,
    protected abi: ReadonlyArray<ethers.JsonFragment>,
    public readonly address: string
  ) {}

  abstract using(
    paladin: PaladinClient
  ): PentePrivateContract<ConstructorParams>;

  sendTransaction(
    transaction: PenteContractTransactionInput,
    txOptions?: Partial<IPrivacyGroupEVMTXInput>
  ) {
    const method = this.abi.find(
      (entry) => entry.name === transaction.function
    );
    if (method === undefined) {
      throw new Error(`Method '${transaction.function}' not found`);
    }
    return this.evm.sendTransaction(
      {
        from: transaction.from,
        to: this.address,
        methodAbi: method,
        data: transaction.data ?? [],
      },
      txOptions
    );
  }

  call(
    transaction: PenteContractTransactionInput,
    txOptions?: Partial<IPrivacyGroupEVMCall>
  ) {
    const method = this.abi.find(
      (entry) => entry.name === transaction.function
    );
    if (method === undefined) {
      throw new Error(`Method '${transaction.function}' not found`);
    }
    return this.evm.call(
      {
        from: transaction.from,
        to: this.address,
        methodAbi: method,
        data: transaction.data ?? [],
      },
      txOptions
    );
  }
}
