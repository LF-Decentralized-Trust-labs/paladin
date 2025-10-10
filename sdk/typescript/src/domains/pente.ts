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

const DEFAULT_POLL_TIMEOUT = 10000;

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

export interface PenteOptions {
  pollTimeout?: number;
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

export class PenteFactory {
  private options: Required<PenteOptions>;

  constructor(
    private paladin: PaladinClient,
    public readonly domain: string,
    options?: PenteOptions
  ) {
    this.options = {
      pollTimeout: DEFAULT_POLL_TIMEOUT,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new PenteFactory(paladin, this.domain, this.options);
  }

  async newPrivacyGroup(input: PentePrivacyGroupParams) {
    const group = await this.paladin.createPrivacyGroup({
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
    });
    const receipt = group.genesisTransaction
      ? await this.paladin.pollForReceipt(
          group.genesisTransaction,
          this.options.pollTimeout
        )
      : undefined;
    group.contractAddress = receipt ? receipt.contractAddress : undefined;
    return group.contractAddress === undefined
      ? undefined
      : new PentePrivacyGroup(this.paladin, group, this.options);
  }

  async resumePrivacyGroup(input: IPrivacyGroupResume) {
    const existingGroup = await this.paladin.getPrivacyGroupById(
      this.domain,
      input.id
    );
    return existingGroup.contractAddress === undefined
      ? undefined
      : new PentePrivacyGroup(this.paladin, existingGroup, this.options);
  }
}

export class PentePrivacyGroup {
  private options: Required<PenteOptions>;
  public readonly address: string;
  public readonly salt: string;
  public readonly members: string[];

  constructor(
    private paladin: PaladinClient,
    public readonly group: IPrivacyGroup,
    options?: PenteOptions
  ) {
    if (group.contractAddress === undefined) {
      throw new Error(
        `Supplied group '${group.id}' is missing a contract address. Check transaction ${group.genesisTransaction}`
      );
    }
    this.address = group.contractAddress;
    this.salt = group.id; // when bypassing privacy group helper functionality, and directly building Pente private transactions
    this.members = group.members;
    this.options = {
      pollTimeout: DEFAULT_POLL_TIMEOUT,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new PentePrivacyGroup(paladin, this.group, this.options);
  }

  async deploy(
    params: PenteDeploy,
    txOptions?: Partial<IPrivacyGroupEVMTXInput>
  ) {
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

    const txID = await this.paladin.sendPrivacyGroupTransaction(transaction);
    const receipt = await this.paladin.pollForReceipt(
      txID,
      this.options.pollTimeout,
      true
    );
    return receipt?.domainReceipt !== undefined &&
      "receipt" in receipt.domainReceipt
      ? receipt.domainReceipt.receipt.contractAddress
      : undefined;
  }

  // sendTransaction functions in the contract (write)
  async sendTransaction(
    transaction: PenteGroupTransactionInput,
    txOptions?: Partial<IPrivacyGroupEVMTXInput>
  ) {
    const txID = await this.paladin.sendPrivacyGroupTransaction({
      ...txOptions,
      domain: this.group.domain,
      group: this.group.id,
      from: transaction.from,
      to: transaction.to,
      input: transaction.data,
      function: transaction.methodAbi,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
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
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: penteJSON.abi,
      function: "approveTransition",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
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

  async sendTransaction(
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

  async call(
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
