import { ethers } from "ethers";
import { IStateEncoded, TransactionType } from "../interfaces";
import PaladinClient from "../paladin";
import { TransactionFuture } from "../transaction";
import { PaladinVerifier } from "../verifier";
import * as notoJSON from "./abis/INoto.json";
import * as notoPrivateJSON from "./abis/INotoPrivate.json";

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
                    components: [
                      { name: "salt", type: "bytes32" },
                      { name: "members", type: "string[]" },
                    ],
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
                ],
              },
            ]),
      ],
    },
  ],
});

export interface IGroupInfo {
  salt: string;
  members: string[];
}

export interface NotoConstructorParams {
  notary: PaladinVerifier;
  notaryMode: "basic" | "hooks";
  options?: {
    basic?: {
      restrictMint: boolean;
      allowBurn: boolean;
      allowLock: boolean;
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

export interface NotoBurnParams {
  amount: string | number;
  data: string;
}

export interface NotoBurnFromParams {
  from: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface NotoTransferParams {
  to: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface NotoTransferFromParams {
  from: PaladinVerifier;
  to: PaladinVerifier;
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
  unlock: NotoUnlockPublicParams;
  delegate: string;
  data: string;
}

export interface NotoUnlockPublicParams {
  txId: string;
  lockedInputs: string[];
  lockedOutputs: string[];
  outputs: string[];
  signature: string;
  data: string;
}

export interface NotoBalanceOfParams {
  account: string;
}

export interface NotoBalanceOfResult {
  totalBalance: string;
  totalStates: string;
  overflow: boolean;
}

// Represents an in-flight Noto deployment
export class NotoFuture extends TransactionFuture {
  async waitForDeploy(waitMs?: number) {
    const receipt = await this.waitForReceipt(waitMs);
    return receipt?.contractAddress
      ? new NotoInstance(this.paladin, receipt.contractAddress)
      : undefined;
  }
}

export class NotoFactory {
  constructor(private paladin: PaladinClient, public readonly domain: string) {}

  using(paladin: PaladinClient) {
    return new NotoFactory(paladin, this.domain);
  }

  newNoto(from: PaladinVerifier, data: NotoConstructorParams) {
    return new NotoFuture(
      this.paladin,
      this.paladin.sendTransaction({
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
              ...data.options?.basic,
            },
            ...data.options,
          },
        },
      })
    );
  }
}

export class NotoInstance {
  constructor(
    private paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    return new NotoInstance(paladin, this.address);
  }

  mint(from: PaladinVerifier, data: NotoMintParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "mint",
        to: this.address,
        from: from.lookup,
        data: {
          ...data,
          to: data.to.lookup,
        },
      })
    );
  }

  transfer(from: PaladinVerifier, data: NotoTransferParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "transfer",
        to: this.address,
        from: from.lookup,
        data: {
          ...data,
          to: data.to.lookup,
        },
      })
    );
  }

  transferFrom(from: PaladinVerifier, data: NotoTransferFromParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "transferFrom",
        to: this.address,
        from: from.lookup,
        data: {
          ...data,
          from: data.from.lookup,
          to: data.to.lookup,
        },
      })
    );
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

  approveTransfer(from: PaladinVerifier, data: NotoApproveTransferParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "approveTransfer",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  burn(from: PaladinVerifier, data: NotoBurnParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "burn",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  burnFrom(from: PaladinVerifier, data: NotoBurnFromParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "burnFrom",
        to: this.address,
        from: from.lookup,
        data: {
          ...data,
          from: data.from.lookup,
        }
      })
    );
  }

  lock(from: PaladinVerifier, data: NotoLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "lock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  unlock(from: PaladinVerifier, data: NotoUnlockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
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
      })
    );
  }

  unlockAsDelegate(from: PaladinVerifier, data: NotoUnlockPublicParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PUBLIC,
        abi: notoJSON.abi,
        function: "unlock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  prepareUnlock(from: PaladinVerifier, data: NotoUnlockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
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
      })
    );
  }

  delegateLock(from: PaladinVerifier, data: NotoDelegateLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: notoPrivateJSON.abi,
        function: "delegateLock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  encodeUnlock(data: NotoUnlockPublicParams) {
    return new ethers.Interface(notoJSON.abi).encodeFunctionData("unlock", [
      data.txId,
      data.lockedInputs,
      data.lockedOutputs,
      data.outputs,
      data.signature,
      data.data,
    ]);
  }

  balanceOf(
    from: PaladinVerifier,
    data: NotoBalanceOfParams
  ): Promise<NotoBalanceOfResult> {
    return this.paladin.call({
      type: TransactionType.PRIVATE,
      domain: "noto",
      abi: notoPrivateJSON.abi,
      function: "balanceOf",
      to: this.address,
      from: from.lookup,
      data,
    });
  }
}
