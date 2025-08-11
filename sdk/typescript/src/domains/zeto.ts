import { TransactionType } from "../interfaces";
import PaladinClient from "../paladin";
import { TransactionFuture } from "../transaction";
import { PaladinVerifier } from "../verifier";
import * as zetoPrivateJSON from "./abis/IZetoFungible.json";
import * as zetoPublicJSON from "./abis/Zeto_Anon.json";

// Algorithm/verifier types specific to Zeto
export const algorithmZetoSnarkBJJ = (domainName: string) =>
  `domain:${domainName}:snark:babyjubjub`;
export const IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X =
  "iden3_pubkey_babyjubjub_compressed_0x";

const zetoAbi = zetoPrivateJSON.abi;
const zetoPublicAbi = zetoPublicJSON.abi;

export const zetoConstructorABI = {
  type: "constructor",
  inputs: [{ name: "tokenName", type: "string" }],
};

export interface ZetoConstructorParams {
  tokenName: string;
}

export interface ZetoMintParams {
  mints: ZetoTransfer[];
}

export interface ZetoTransferParams {
  transfers: ZetoTransfer[];
}

export interface ZetoLockParams {
  amount: number;
  delegate: string;
}

export interface ZetoTransferLockedParams {
  lockedInputs: string[];
  delegate: string;
  transfers: ZetoTransfer[];
}

export interface ZetoDelegateLockParams {
  utxos: string[];
  delegate: string;
}

export interface ZetoSetERC20Params {
  erc20: string;
}

export interface ZetoTransfer {
  to: PaladinVerifier;
  amount: string | number;
  data: string;
}

export interface ZetoDepositParams {
  amount: string | number;
}

export interface ZetoWithdrawParams {
  amount: string | number;
}

export interface ZetoBalanceOfParams {
  account: string;
}

export interface ZetoBalanceOfResult {
  totalBalance: string;
  totalStates: string;
  overflow: boolean;
}

// Represents an in-flight Zeto deployment
export class ZetoFuture extends TransactionFuture {
  async waitForDeploy(waitMs?: number) {
    const receipt = await this.waitForReceipt(waitMs);
    return receipt?.contractAddress
      ? new ZetoInstance(this.paladin, receipt.contractAddress)
      : undefined;
  }
}

export class ZetoFactory {
  constructor(private paladin: PaladinClient, public readonly domain: string) {}

  using(paladin: PaladinClient) {
    return new ZetoFactory(paladin, this.domain);
  }

  newZeto(from: PaladinVerifier, data: ZetoConstructorParams) {
    return new ZetoFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        domain: this.domain,
        abi: [zetoConstructorABI],
        function: "",
        from: from.lookup,
        data,
      })
    );
  }
}

export class ZetoInstance {
  private erc20?: string;

  constructor(
    private paladin: PaladinClient,
    public readonly address: string
  ) {}

  using(paladin: PaladinClient) {
    const zeto = new ZetoInstance(paladin, this.address);
    zeto.erc20 = this.erc20;
    return zeto;
  }

  mint(from: PaladinVerifier, data: ZetoMintParams) {
    const params = {
      mints: data.mints.map((t) => ({ ...t, to: t.to.lookup })),
    };
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "mint",
        to: this.address,
        from: from.lookup,
        data: params,
      })
    );
  }

  transfer(from: PaladinVerifier, data: ZetoTransferParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transfer",
        to: this.address,
        from: from.lookup,
        data: {
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  transferLocked(from: PaladinVerifier, data: ZetoTransferLockedParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transferLocked",
        to: this.address,
        from: from.lookup,
        data: {
          lockedInputs: data.lockedInputs,
          delegate: data.delegate,
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  prepareTransferLocked(from: PaladinVerifier, data: ZetoTransferLockedParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.prepareTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "transferLocked",
        to: this.address,
        from: from.lookup,
        data: {
          lockedInputs: data.lockedInputs,
          delegate: data.delegate,
          transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
        },
      })
    );
  }

  lock(from: PaladinVerifier, data: ZetoLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "lock",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  delegateLock(from: PaladinVerifier, data: ZetoDelegateLockParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.Public,
        abi: zetoPublicAbi,
        function: "delegateLock",
        to: this.address,
        from: from.lookup,
        data: {
          data: "0x",
          utxos: data.utxos,
          delegate: data.delegate,
        },
      })
    );
  }

  setERC20(from: PaladinVerifier, data: ZetoSetERC20Params) {
    this.erc20 = data.erc20;
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PUBLIC,
        abi: zetoAbi,
        function: "setERC20",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  deposit(from: PaladinVerifier, data: ZetoDepositParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "deposit",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  withdraw(from: PaladinVerifier, data: ZetoWithdrawParams) {
    return new TransactionFuture(
      this.paladin,
      this.paladin.sendTransaction({
        type: TransactionType.PRIVATE,
        abi: zetoAbi,
        function: "withdraw",
        to: this.address,
        from: from.lookup,
        data,
      })
    );
  }

  async balanceOf(
    from: PaladinVerifier,
    data: ZetoBalanceOfParams
  ): Promise<ZetoBalanceOfResult> {
    return await this.paladin.call({
      type: TransactionType.PRIVATE,
      domain: "zeto",
      abi: zetoPrivateJSON.abi,
      function: "balanceOf",
      to: this.address,
      from: from.lookup,
      data,
    });
  }
}
