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

export interface IOnChainLocation {
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
}

export interface IActiveFlag {
  active: boolean;
}

export interface IRegistryEntry {
  registry: string;
  id: string;
  name: string;
  blockNumber: number;
  transactionIndex: number;
  logIndex: number;
  active: boolean;
}

export type ActiveFilter = "active" | "inactive" | "any";

export interface IRegistryEntryWithProperties extends IRegistryEntry {
  properties: Record<string, string>;
}

export interface IRegistryProperty {
  registry: string;
  entryId: string;
  name: string;
  value: string;
  onChainLocation?: IOnChainLocation;
  activeFlag?: IActiveFlag;
}
