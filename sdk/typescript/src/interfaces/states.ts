export interface ISchema {
  id: string;
  created: string;
  domain: string;
  type: SchemaType;
  signature: string;
  definition: object;
  labels: string[];
}

export type SchemaType = "abi";

export interface IStateBase {
  id: string;
  created: string;
  domain: string;
  schema: string;
  contractAddress: string;
  data: object;
}

export interface IStateEncoded {
  id: string;
  domain: string;
  schema: string;
  contractAddress: string;
  data: string;
}

export interface IState extends IStateBase {
  confirmed?: IStateConfirm;
  spent?: IStateSpend;
  locks?: IStateLock[];
  nullifier?: IStateNullifier;
}

export interface IStateLabel {
  domainName: string;
  state: string;
  label: string;
  value: string;
}

export interface IStateInt64Label {
  domainName: string;
  state: string;
  label: string;
  value: number;
}

export interface IStateConfirm {
  transaction: string;
}

export interface IStateSpend {
  transaction: string;
}

type StateLockType = "create" | "read" | "spend";

export interface IStateLock {
  transaction: string;
  type: StateLockType;
}

export interface IStateNullifier {
  domain: string;
  id: string;
  spent?: IStateSpend;
}

export type StateStatus =
  | "available"
  | "confirmed"
  | "unconfirmed"
  | "spent"
  | "all";
