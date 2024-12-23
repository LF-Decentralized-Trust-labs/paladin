import { IStateBase } from "./states";
import { ethers } from "ethers";

export interface IBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export enum TransactionType {
  PUBLIC = "public",
  PRIVATE = "private",
}

export interface ITransactionBase {
  type: TransactionType;
  domain?: string;
  function: string;
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
  success: boolean;
  transactionHash: string;
  source: string;
  contractAddress?: string;
  states?: ITransactionStates;
  domainReceipt?: IPenteDomainReceipt;
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

export interface IDecodedEvent {
  signature: string;
  definition: ethers.JsonFragment;
  data: any;
  summary: string; // errors only
}

export interface IEventWithData {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  transactionHash: string;
  signature: string;
  soliditySignature: string;
  address: string;
  data: any;
}
