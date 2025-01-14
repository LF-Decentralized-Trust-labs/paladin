import { ethers } from "ethers";
import { IGroupInfo, IStateEncoded, TransactionType } from "../interfaces";
import PaladinClient from "../paladin";
import * as notoPrivateJSON from "./abis/INotoPrivate.json";
import * as notoJSON from "./abis/INoto.json";
import { penteGroupABI } from "./pente";
import { PaladinVerifier } from "../verifier";

const DEFAULT_POLL_TIMEOUT = 10000;

export interface NotoOptions {
  pollTimeout?: number;
}

export const notoConstructorABI = (
  withHooks: boolean
): ethers.JsonFragment => ({
  type: "constructor",
  inputs: [
    { name: "notary", type: "string" },
    { name: "notaryMode", type: "string" },
    {
      name: "options",
      type: "tuple",
      components: [
        ...(withHooks
          ? [
              {
                name: "hooks",
                type: "tuple",
                components: [
                  {
                    name: "privateGroup",
                    type: "tuple",
                    components: penteGroupABI.components,
                  },
                  { name: "publicAddress", type: "address" },
                  { name: "privateAddress", type: "address" },
                ],
              },
            ]
          : [
              {
                name: "basic",
                type: "tuple",
                components: [
                  { name: "restrictMint", type: "bool" },
                  { name: "allowBurn", type: "bool" },
                  { name: "allowLock", type: "bool" },
                  { name: "restrictUnlock", type: "bool" },
                ],
              },
            ]),
      ],
    },
  ],
});

export interface NotoConstructorParams {
  notary: PaladinVerifier;
  notaryMode: "basic" | "hooks";
  options?: {
    basic?: {
      restrictMint: boolean;
      allowBurn: boolean;
      allowLock: boolean;
      restrictUnlock: boolean;
    };
    hooks?: {
      publicAddress: string;
      privateGroup?: IGroupInfo;
      privateAddress?: string;
    };
  };
}

export interface NotoMintParams {
  to: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface NotoTransferParams {
  to: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface NotoBurnParams {
  amount: string | number;
  data: string;
}

export interface NotoApproveTransferParams {
  inputs: IStateEncoded[];
  outputs: IStateEncoded[];
  data: string;
  delegate: string;
}

export interface NotoLockParams {
  lockId: string;
  amount: string | number;
  data: string;
}

export interface NotoUnlockParams {
  lockId: string;
  from: PaladinVerifier;
  recipients: UnlockRecipient[];
  data: string;
}

export interface UnlockRecipient {
  to: PaladinVerifier;
  amount: string | number;
}

export interface NotoDelegateLockParams {
  lockId: string;
  delegate: string;
  data: string;
}

export interface NotoUnlockPublicParams {
  lockId: string;
  lockedInputs: string[];
  lockedOutputs: string[];
  outputs: string[];
  signature: string;
  data: string;
}

export class NotoFactory {
  private options: Required<NotoOptions>;

  constructor(
    private paladin: PaladinClient,
    public readonly domain: string,
    options?: NotoOptions
  ) {
    this.options = {
      pollTimeout: DEFAULT_POLL_TIMEOUT,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new NotoFactory(paladin, this.domain, this.options);
  }

  async newNoto(from: PaladinVerifier, data: NotoConstructorParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      domain: this.domain,
      abi: [notoConstructorABI(!!data.options?.hooks)],
      function: "",
      from: from.lookup,
      data: {
        ...data,
        notary: data.notary.lookup,
        options: {
          basic: {
            restrictMint: true,
            allowBurn: true,
            allowLock: true,
            restrictUnlock: true,
            ...data.options?.basic,
          },
          ...data.options,
        },
      },
    });
    const receipt = await this.paladin.pollForReceipt(
      txID,
      this.options.pollTimeout
    );
    return receipt?.contractAddress === undefined
      ? undefined
      : new NotoInstance(this.paladin, receipt.contractAddress, this.options);
  }
}

export class NotoInstance {
  private options: Required<NotoOptions>;

  constructor(
    private paladin: PaladinClient,
    public readonly address: string,
    options?: NotoOptions
  ) {
    this.options = {
      pollTimeout: DEFAULT_POLL_TIMEOUT,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new NotoInstance(paladin, this.address, this.options);
  }

  async mint(from: PaladinVerifier, data: NotoMintParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "mint",
      to: this.address,
      from: from.lookup,
      data: {
        ...data,
        to: data.to.lookup,
      },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async transfer(from: PaladinVerifier, data: NotoTransferParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "transfer",
      to: this.address,
      from: from.lookup,
      data: {
        ...data,
        to: data.to.lookup,
      },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  prepareTransfer(from: PaladinVerifier, data: NotoTransferParams) {
    return this.paladin.prepareTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "transfer",
      to: this.address,
      from: from.lookup,
      data: {
        ...data,
        to: data.to.lookup,
      },
    });
  }

  async approveTransfer(
    from: PaladinVerifier,
    data: NotoApproveTransferParams
  ) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "approveTransfer",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async burn(from: PaladinVerifier, data: NotoBurnParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "burn",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async lock(from: PaladinVerifier, data: NotoLockParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "lock",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async unlock(from: PaladinVerifier, data: NotoUnlockParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "unlock",
      to: this.address,
      from: from.lookup,
      data: {
        ...data,
        from: data.from.lookup,
        recipients: data.recipients.map((recipient) => ({
          to: recipient.to.lookup,
          amount: recipient.amount,
        })),
      },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async unlockAsDelegate(from: PaladinVerifier, data: NotoUnlockPublicParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: notoJSON.abi,
      function: "unlock",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async prepareUnlock(from: PaladinVerifier, data: NotoUnlockParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "prepareUnlock",
      to: this.address,
      from: from.lookup,
      data: {
        ...data,
        from: data.from.lookup,
        recipients: data.recipients.map((recipient) => ({
          to: recipient.to.lookup,
          amount: recipient.amount,
        })),
      },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async delegateLock(from: PaladinVerifier, data: NotoDelegateLockParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: notoPrivateJSON.abi,
      function: "delegateLock",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }
}
