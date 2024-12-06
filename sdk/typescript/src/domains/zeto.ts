import { TransactionType } from "../interfaces";
import PaladinClient from "../paladin";

const POLL_TIMEOUT_MS = 10000;

export interface ZetoOptions {
  pollTimeout?: number;
}

const zetoPrivateAbi = [{
  "name": "mint",
  "type": "function",
  "inputs": [
    {
      "name": "mints",
      "type": "tuple[]",
      "components": [
        {
          "name": "to",
          "type": "string",
          "internalType": "string"
        },
        {
          "name": "amount",
          "type": "uint256",
          "internalType": "uint256"
        }
      ]
    }
  ],
  "outputs": [],
}, {
  "type": "function",
  "name": "transfer",
  "inputs": [
    {
      "name": "transfers",
      "type": "tuple[]",
      "components": [
        {
          "name": "to",
          "type": "string",
          "internalType": "string"
        },
        {
          "name": "amount",
          "type": "uint256",
          "internalType": "uint256"
        }
      ]
    }
  ],
  "outputs": []
}, {
  "type": "function",
  "name": "deposit",
  "inputs": [
    {
      "internalType": "uint256",
      "name": "amount",
      "type": "uint256"
    }
  ],
  "outputs": []
}, {
  "type": "function",
  "name": "withdraw",
  "inputs": [
    {
      "internalType": "uint256",
      "name": "amount",
      "type": "uint256"
    }
  ],
  "outputs": []
}];

const zetoPublicAbi = [{
  "inputs": [
    {
      "internalType": "contract IERC20",
      "name": "_erc20",
      "type": "address"
    }
  ],
  "name": "setERC20",
  "outputs": [],
  "stateMutability": "nonpayable",
  "type": "function"
}, {
  "inputs": [
    {
      "internalType": "uint256",
      "name": "amount",
      "type": "uint256"
    },
    {
      "internalType": "uint256[]",
      "name": "outputs",
      "type": "uint256[]"
    },
    {
      "components": [
        {
          "internalType": "uint256[2]",
          "name": "pA",
          "type": "uint256[2]"
        },
        {
          "internalType": "uint256[2][2]",
          "name": "pB",
          "type": "uint256[2][2]"
        },
        {
          "internalType": "uint256[2]",
          "name": "pC",
          "type": "uint256[2]"
        }
      ],
      "internalType": "struct Commonlib.Proof",
      "name": "proof",
      "type": "tuple"
    },
    {
      "internalType": "bytes",
      "name": "data",
      "type": "bytes"
    }
  ],
  "name": "deposit",
  "outputs": [],
  "stateMutability": "nonpayable",
  "type": "function"
}, {
  "inputs": [
    {
      "internalType": "uint256",
      "name": "amount",
      "type": "uint256"
    },
    {
      "internalType": "uint256[]",
      "name": "nullifiers",
      "type": "uint256[]"
    },
    {
      "internalType": "uint256",
      "name": "output",
      "type": "uint256"
    },
    {
      "internalType": "uint256",
      "name": "root",
      "type": "uint256"
    },
    {
      "components": [
        {
          "internalType": "uint256[2]",
          "name": "pA",
          "type": "uint256[2]"
        },
        {
          "internalType": "uint256[2][2]",
          "name": "pB",
          "type": "uint256[2][2]"
        },
        {
          "internalType": "uint256[2]",
          "name": "pC",
          "type": "uint256[2]"
        }
      ],
      "internalType": "struct Commonlib.Proof",
      "name": "proof",
      "type": "tuple"
    },
    {
      "internalType": "bytes",
      "name": "data",
      "type": "bytes"
    }
  ],
  "name": "withdraw",
  "outputs": [],
  "stateMutability": "nonpayable",
  "type": "function"
}];

const erc20Abi = [{
  "type": "function",
  "name": "mint",
  "inputs": [
    {
      "internalType": "address",
      "name": "to",
      "type": "address"
    },
    {
      "internalType": "uint256",
      "name": "amount",
      "type": "uint256"
    }
  ],
  "outputs": []
}, {
  "type": "function",
  "name": "approve",
  "inputs": [
    {
      "internalType": "address",
      "name": "spender",
      "type": "address"
    },
    {
      "internalType": "uint256",
      "name": "value",
      "type": "uint256"
    }
  ],
  "outputs": []
}];

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

export interface ZetoSetERC20Params {
  _erc20: string;
}

export interface ZetoTransfer {
  to: string;
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

  async newZeto(from: string, data: ZetoConstructorParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      domain: this.domain,
      abi: [zetoConstructorABI],
      function: "",
      from,
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

  async mint(from: string, data: ZetoMintParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoPrivateAbi,
      function: "mint",
      to: this.address,
      from,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async transfer(from: string, data: ZetoTransferParams) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoPrivateAbi,
      function: "transfer",
      to: this.address,
      from,
      data,
    });
    return this.paladin.pollForReceipt(txID, this.options.pollTimeout);
  }

  async setERC20(from: string, data: ZetoSetERC20Params) {
    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: zetoPublicAbi,
      function: "setERC20",
      to: this.address,
      from,
      data,
    });
    const result = await this.paladin.pollForReceipt(txID, POLL_TIMEOUT_MS);
    if (result === undefined) {
      throw new Error("Failed to set ERC20");
    }
    this.erc20 = data._erc20;
    return result;
  }

  async deposit(from: string, data: ZetoDepositParams) {
    // first approve the Zeto contract to draw the amount from our balance in the ERC20
    const txID1 = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: erc20Abi,
      function: "approve",
      to: this.erc20,
      from,
      data: { value: data.amount, spender: this.address },
    });
    const result1 = await this.paladin.pollForReceipt(txID1, POLL_TIMEOUT_MS);
    if (result1 === undefined) {
      throw new Error("Failed to approve transfer");
    }

    const depositID = await this.paladin.prepareTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoPrivateAbi,
      function: "deposit",
      to: this.address,
      from,
      data,
    });
    const result2 = await this.paladin.pollForPreparedTransaction(depositID, this.options.pollTimeout);
    if (result2 === undefined) {
      throw new Error("Failed to prepare deposit");
    }

    const txID2 = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: zetoPublicAbi,
      function: "deposit",
      to: this.address,
      from,
      data: result2!.transaction.data,
    });
    return this.paladin.pollForReceipt(txID2, POLL_TIMEOUT_MS);
  }

  async withdraw(from: string, data: ZetoWithdrawParams) {
    const withdrawID = await this.paladin.prepareTransaction({
      type: TransactionType.PRIVATE,
      abi: zetoPrivateAbi,
      function: "withdraw",
      to: this.address,
      from,
      data,
    });
    const result1 = await this.paladin.pollForPreparedTransaction(withdrawID, this.options.pollTimeout);
    if (result1 === undefined) {
      throw new Error("Failed to prepare withdraw");
    }

    const txID = await this.paladin.sendTransaction({
      type: TransactionType.PUBLIC,
      abi: zetoPublicAbi,
      function: "withdraw",
      to: this.address,
      from,
      data: result1!.transaction.data,
    });
    return this.paladin.pollForReceipt(txID, POLL_TIMEOUT_MS);
  }
}
