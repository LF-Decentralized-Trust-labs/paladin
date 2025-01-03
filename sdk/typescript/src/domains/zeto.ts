import { TransactionType } from "../interfaces";
import PaladinClient from "../paladin";
import { PaladinVerifier } from "../verifier";
import * as zetoPrivateJSON from "./abis/IZetoPrivate.json";

const POLL_TIMEOUT_MS = 10000;

export interface ZetoOptions {
  pollTimeout?: number;
}

const zetoAbi = zetoPrivateJSON.abi;

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
  delegate: string;
  call: string;
}

export interface ZetoSetERC20Params {
  erc20: string;
}

export interface ZetoTransfer {
  to: PaladinVerifier;
  amount: string | number;
}

export interface ZetoDepositParams {
  amount: string | number;
}

export interface ZetoWithdrawParams {
  amount: string | number;
}

export class ZetoFactory {
  private options: Required<ZetoOptions>;

  constructor(
    private paladin: PaladinClient,
    public readonly domain: string,
    options?: ZetoOptions
  ) {
    this.options = {
      pollTimeout: POLL_TIMEOUT_MS,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    return new ZetoFactory(paladin, this.domain, this.options);
  }

  async newZeto(from: PaladinVerifier, data: ZetoConstructorParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      domain: this.domain,
      abi: [zetoConstructorABI],
      function: "",
      from: from.lookup,
      data,
    });
    const receipt = await this.paladin.pollForReceipt(
      txID,
      this.options.pollTimeout
    );
    return receipt?.contractAddress === undefined
      ? undefined
      : new ZetoInstance(this.paladin, receipt.contractAddress, this.options);
  }
}

export class ZetoInstance {
  private options: Required<ZetoOptions>;
  private erc20?: string;

  constructor(
    private paladin: PaladinClient,
    public readonly address: string,
    options?: ZetoOptions
  ) {
    this.options = {
      pollTimeout: POLL_TIMEOUT_MS,
      ...options,
    };
  }

  using(paladin: PaladinClient) {
    const zeto = new ZetoInstance(paladin, this.address, this.options);
    zeto.erc20 = this.erc20;
    return zeto;
  }

  async mint(from: PaladinVerifier, data: ZetoMintParams) {
    const params = {
      mints: data.mints.map((t) => ({ ...t, to: t.to.lookup })),
    };
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoAbi,
      function: "mint",
      to: this.address,
      from: from.lookup,
      data: params,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async transfer(from: PaladinVerifier, data: ZetoTransferParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoAbi,
      function: "transfer",
      to: this.address,
      from: from.lookup,
      data: {
        transfers: data.transfers.map((t) => ({ ...t, to: t.to.lookup })),
      },
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async lock(from: PaladinVerifier, data: ZetoLockParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoAbi,
      function: "lock",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(txID, POLL_TIMEOUT_MS);
  }

  async setERC20(from: PaladinVerifier, data: ZetoSetERC20Params) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: zetoAbi,
      function: "setERC20",
      to: this.address,
      from: from.lookup,
      data,
    });
    this.erc20 = data.erc20;
    return this.paladin.pollForReceipt(txID, POLL_TIMEOUT_MS);
  }

  async deposit(from: PaladinVerifier, data: ZetoDepositParams) {
    const receipt = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoAbi,
      function: "deposit",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(receipt, POLL_TIMEOUT_MS);
  }

  async withdraw(from: PaladinVerifier, data: ZetoWithdrawParams) {
    const receipt = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoAbi,
      function: "withdraw",
      to: this.address,
      from: from.lookup,
      data,
    });
    return this.paladin.pollForReceipt(receipt, POLL_TIMEOUT_MS);
  }
}
