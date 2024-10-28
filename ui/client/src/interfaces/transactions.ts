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
  id: string;
  success: boolean;
  transactionHash: string;
}

export interface ITransactionDependencies {
  dependsOn: string[];
  prereqOf: string[];
}

export interface IPublicTxWithBinding {
  to?: string;
  data?: string;
  from: string;
  nonce: string;
  created: string;
  completedAt?: string;
  transactionHash?: string;
  success?: boolean;
  revertData?: string;
  submissions?: IPublicTxSubmissionData[];
  activity?: ITransactionActivityRecord[];
  gas?: string;
  value?: string;
  maxPriorityFeePerGas?: string;
  maxFeePerGas?: string;
  gasPrice?: string;
  transaction: string;
  transactionType: TransactionType;
}

export interface IPublicTxSubmissionData {
  time: string;
  transactionHash: string;
  maxPriorityFeePerGas?: string;
  maxFeePerGas?: string;
  gasPrice?: string;
}

export interface ITransactionActivityRecord {
  time: string;
  message: string;
}

export enum TransactionType {
  Private = "private",
  Public = "public",
}
