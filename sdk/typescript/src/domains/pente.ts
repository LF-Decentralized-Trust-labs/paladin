import { randomBytes } from "crypto";
import { ethers } from "ethers";
import {
  IGroupInfo,
  IGroupInfoUnresolved,
  TransactionType,
} from "../interfaces";
import PaladinClient from "../paladin";
import * as penteJSON from "./abis/PentePrivacyGroup.json";
import { PaladinVerifier } from "../verifier";

const DEFAULT_POLL_TIMEOUT = 10000;

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

export const penteConstructorABI = {
  type: "constructor",
  inputs: [
    penteGroupABI,
    { name: "evmVersion", type: "string" },
    { name: "endorsementType", type: "string" },
    { name: "externalCallsEnabled", type: "bool" },
  ],
};

export const privateDeployABI = (
  inputComponents: ReadonlyArray<ethers.JsonFragmentType>
): ethers.JsonFragment => ({
  name: "deploy",
  type: "function",
  inputs: [
    penteGroupABI,
    { name: "bytecode", type: "bytes" },
    { name: "inputs", type: "tuple", components: inputComponents },
  ],
});

const privateInvokeABI = (
  name: string,
  inputComponents: ReadonlyArray<ethers.JsonFragmentType>
): ethers.JsonFragment => ({
  name,
  type: "function",
  inputs: [
    penteGroupABI,
    { name: "to", type: "address" },
    { name: "inputs", type: "tuple", components: inputComponents },
  ],
});

const privateCallABI = (
  name: string,
  inputComponents: ReadonlyArray<ethers.JsonFragmentType>,
  outputComponents: ReadonlyArray<ethers.JsonFragmentType>
): ethers.JsonFragment => ({
  name,
  type: "function",
  inputs: [
    penteGroupABI,
    { name: "to", type: "address" },
    { name: "inputs", type: "tuple", components: inputComponents },
  ],
  outputs: outputComponents,
});

export interface PentePrivacyGroupParams {
  group: IGroupInfo | IGroupInfoUnresolved;
  evmVersion: string;
  endorsementType: string;
  externalCallsEnabled: boolean;
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

  async newPrivacyGroup(from: PaladinVerifier, data: PentePrivacyGroupParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      domain: this.domain,
      abi: [penteConstructorABI],
      function: "",
      from: from.lookup,
      data: {
        ...data,
        group: resolveGroup(data.group),
      },
    });
    const receipt = await this.paladin.pollForReceipt(
      txID,
      this.options.pollTimeout
    );
    return receipt?.contractAddress === undefined
      ? undefined
      : new PentePrivacyGroup(
          this.paladin,
          resolveGroup(data.group),
          receipt.contractAddress,
          this.options
        );
  }
}

export class PentePrivacyGroup {
  private options: Required<PenteOptions>;

  constructor(
    private paladin: PaladinClient,
    public readonly group: IGroupInfo,
    public readonly address: string,
    options?: PenteOptions
  ) {
    this.options = {
      pollTimeout: DEFAULT_POLL_TIMEOUT,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new PentePrivacyGroup(
      paladin,
      this.group,
      this.address,
      this.options
    );
  }

  // deploy a contract
  async deploy<ConstructorParams>(
    abi: ReadonlyArray<ethers.JsonFragment>,
    bytecode: string,
    from: PaladinVerifier,
    inputs?: ConstructorParams
  ) {

      // Find the constructor in the ABI
    const constructor = abi.find((entry) => entry.type === "constructor");

    // Handle the absence of a constructor
    const constructorInputs = constructor?.inputs ?? [];

    // Prepare the data object
    const data: Record<string, any> = {
      group: this.group,
      bytecode,
      inputs: inputs ?? [], // Ensure `inputs` is always included, defaulting to an empty array
    };

    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: [privateDeployABI(constructorInputs ?? [])],
      function: "deploy",
      to: this.address,
      from: from.lookup,
      data: data
    });
    const receipt = await this.paladin.pollForReceipt(
      txID,
      this.options.pollTimeout,
      true
    );
    return receipt?.domainReceipt?.receipt.contractAddress;
  }

  // invoke functions in the contract
  async invoke<Params>(
    from: PaladinVerifier,
    to: string,
    methodAbi: ethers.JsonFragment,
    inputs: Params
  ) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: [privateInvokeABI(methodAbi.name ?? "", methodAbi.inputs ?? [])],
      function: "",
      to: this.address,
      from: from.lookup,
      data: { group: this.group, to, inputs },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  // call functions in the contract (read-only)
  async call<Params>(
    from: PaladinVerifier,
    to: string,
    methodAbi: ethers.JsonFragment,
    inputs: Params
  ) {
    return this.paladin.call({
      type: TransactionType.PRIVATE,
      abi: [
        privateCallABI(
          methodAbi.name ?? "",
          methodAbi.inputs ?? [],
          methodAbi.outputs ?? []
        ),
      ],
      function: "",
      to: this.address,
      from: from.lookup,
      data: { group: this.group, to, inputs },
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

  async invoke<Params>(
    from: PaladinVerifier,
    methodName: string,
    params: Params
  ) {
    const method = this.abi.find((entry) => entry.name === methodName);
    if (method === undefined) {
      throw new Error(`Method '${methodName}' not found`);
    }
    return this.evm.invoke(from, this.address, method, params);
  }

  async call<Params>(
    from: PaladinVerifier,
    methodName: string,
    params: Params
  ) {
    const method = this.abi.find((entry) => entry.name === methodName);
    if (method === undefined) {
      throw new Error(`Method '${methodName}' not found`);
    }
    return this.evm.call(from, this.address, method, params);
  }
}
