// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

export interface IBlock {
  number: number;
  hash: string;
  timestamp: string;
}

export interface ITransaction {
  hash: string;
  blockNumber: number;
  transactionIndex: number;
  from: string;
  nonce: number;
  contractAddress?: string;
  result: string;
  block: IBlock;
}

export interface IEnrichedTransaction extends ITransaction {
  receipts: ITransactionReceipt[]
  paladinTransactions: IPaladinTransaction[]
}

export interface IEvent {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  transactionHash: string;
  signature: string;
  block: IBlock;
}

export interface IRegistryEntry {
  registry: string;
  id: string;
  name: string;
  active: boolean;
  properties: {
    [key: string]: string;
  };
}

export interface IPaladinTransaction {
  id: string;
  created: string;
  type: string;
  domain: string;
  function: string;
  to?: string;
  from: string;
  abiReference: string;
  data: {
    [key: string]: string;
  };
}

export interface ITransactionReceipt {
  blockNumber: number;
  domain: string;
  id: string;
  success: boolean;
  transactionHash: string;
}

export interface IStateReceipt {
  none?: boolean;
  [key: string]: any;
}

export interface IPrivateEVMTransaction {
  from?: string;
  to?: string;
  nonce?: string;
  gas?: string;
  data?: string;
}

export interface IPrivateEVMReceipt {
  from?: string;
  to?: string;
  gasUsed?: string;
  contractAddress?: string;
  logs?: IPrivateEVMLog[];
}

export interface IPrivateEVMLog {
  address?: string;
  topics?: string[];
  data?: string;
}

export interface IABIParameter {
  type: string;
  name?: string;
  indexed?: boolean;
  components?: IABIParameter[] | null;
}

export interface IABIEntry {
  type: string;
  name: string;
  inputs?: IABIParameter[] | null;
  outputs?: IABIParameter[] | null;
}

export interface IABIDecodedEntry {
  data: any;
  definition: IABIEntry;
  signature: string;
  summary?: string; // errors only
}