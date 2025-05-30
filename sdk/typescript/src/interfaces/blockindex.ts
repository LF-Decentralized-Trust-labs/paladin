export interface IIndexedBlock {
  number: number;
  hash: string;
  timestamp: number;
}

export interface IIndexedTransaction {
  hash: string;
  blockNumber: number;
  transactionIndex: number;
  from?: string;
  to?: string;
  nonce: number;
  contractAddress?: string;
  result?: "failure" | "success";
  block?: IIndexedBlock;
}

export interface IIndexedEvent {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  transactionHash: string;
  signature: string;
  transaction?: IIndexedTransaction;
  block?: IIndexedBlock;
}

export interface IEventWithData extends IIndexedEvent {
  soliditySignature: string;
  address: string;
  data: any;
}
