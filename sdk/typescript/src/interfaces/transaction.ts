import { BigNumberish, ethers } from "ethers";
import { NotoUnlockPublicParams } from "../domains/noto";
import { IStateBase } from "./states";

export interface IBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export enum TransactionType {
  PUBLIC = "public",
  PRIVATE = "private",
}

export interface PublicTxOptions {
  gas?: BigNumberish;
  value?: BigNumberish;
  maxPriorityFeePerGas?: BigNumberish;
  maxFeePerGas?: BigNumberish;
  gasPrice?: BigNumberish;
}

export interface ITransactionBase {
  type: TransactionType;
  domain?: string;
  function?: string;
  from: string;
  to?: string;
  data: {
    [key: string]: any;
  };
}

export interface ITransaction extends ITransactionBase {
  id: string;
  created: string;
  abiReference: string;
}

export interface IPreparedTransaction {
  id: string;
  domain: string;
  to: string;
  transaction: ITransactionBase & {
    abiReference: string;
  };
  states: {
    spent?: IStateBase[];
    read?: IStateBase[];
    confirmed?: IStateBase[];
    info?: IStateBase[];
  };
  metadata: any;
}

export interface ITransactionInput extends ITransactionBase {
  abiReference?: string;
  abi?: ethers.InterfaceAbi;
  bytecode?: string;
}

export interface ITransactionCall extends ITransactionInput {}

export interface ITransactionReceipt {
  blockNumber: number;
  id: string;
  sequence: number;
  success: boolean;
  transactionHash: string;
  source: string;
  domain?: string;
  contractAddress?: string;
  states?: ITransactionStates;
  domainReceipt?: IPenteDomainReceipt | INotoDomainReceipt;
  failureMessage?: string;
}

export interface IPenteDomainReceipt {
  receipt: {
    from?: string;
    to?: string;
    contractAddress?: string;
    logs?: IPenteLog[];
  };
}

export interface IPenteLog {
  address: string;
  topics: string[];
  data: string;
}

export interface INotoDomainReceipt {
  states: {
    inputs?: IReceiptState<INotoCoin>[];
    outputs?: IReceiptState<INotoCoin>[];
    readInputs?: IReceiptState<INotoCoin>[];
    preparedOutputs?: IReceiptState<INotoCoin>[];

    lockedInputs?: IReceiptState<INotoLockedCoin>[];
    lockedOutputs?: IReceiptState<INotoLockedCoin>[];
    readLockedInputs?: IReceiptState<INotoLockedCoin>[];
    preparedLockedOutputs?: IReceiptState<INotoLockedCoin>[];
  };
  transfers?: {
    from?: string;
    to?: string;
    amount: string;
  }[];
  lockInfo?: {
    lockId: string;
    delegate?: string;
    unlockParams?: NotoUnlockPublicParams;
    unlockCall?: string;
  };
  data?: string;
}

export interface IReceiptState<T> {
  id: string;
  data: T;
}

export interface INotoCoin {
  salt: string;
  owner: string;
  amount: string;
}

export interface INotoLockedCoin {
  lockId: string;
  salt: string;
  owner: string;
  amount: string;
}

export interface ITransactionStates {
  none?: boolean;
  spent?: IStateBase[];
  read?: IStateBase[];
  confirmed?: IStateBase[];
  info?: IStateBase[];
  unavailable?: {
    spent?: string[];
    read?: string[];
    confirmed?: string[];
    info?: string[];
  };
}

export enum TransactionType {
  Private = "private",
  Public = "public",
}

export interface IABIDecodedData {
  signature: string;
  definition: ethers.JsonFragment;
  data: any;
  summary: string; // errors only
}

export interface IStoredABI {
  hash: string;
  abi: ethers.InterfaceAbi;
}

export interface ITransactionReceiptListener {
  name: string;
  filters?: {
    sequenceAbove?: number;
    type?: TransactionType;
    domain?: string;
  };
  options?: {
    domainReceipts?: boolean;
    incompleteStateReceiptBehavior?: "block_contract" | "process";
  };
}
