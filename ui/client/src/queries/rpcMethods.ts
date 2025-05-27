// Copyright © 2025 Kaleido, Inc.
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

import { getBasePath } from '../utils';

export const RpcEndpoint = getBasePath();

export const RpcMethods = {
  bidx_QueryIndexedEvents: 'bidx_queryIndexedEvents',
  bidx_QueryIndexedTransactions: 'bidx_queryIndexedTransactions',
  domain_listDomains: 'domain_listDomains',
  domain_getDomain: 'domain_getDomain',
  domain_querySmartContracts: 'domain_querySmartContracts',
  ptx_QueryPendingTransactions: 'ptx_queryPendingTransactions',
  ptx_QueryTransactionReceipts: 'ptx_queryTransactionReceipts',
  ptx_getTransactionReceipt: 'ptx_getTransactionReceipt',
  ptx_QueryTransactions: 'ptx_queryTransactions',
  ptx_QueryTransactionsFull: 'ptx_queryTransactionsFull',
  ptx_getStateReceipt: 'ptx_getStateReceipt',
  ptx_getDomainReceipt: 'ptx_getDomainReceipt',
  ptx_decodeCall: 'ptx_decodeCall',
  ptx_decodeEvent: 'ptx_decodeEvent',
  ptx_storeABI: 'ptx_storeABI',
  ptx_resolveVerifier: 'ptx_resolveVerifier',
  ptx_sendTransaction: 'ptx_sendTransaction',
  reg_QueryEntriesWithProps: 'reg_queryEntriesWithProps',
  reg_Registries: 'reg_registries',
  transport_nodeName: 'transport_nodeName',
  transport_localTransportDetails: 'transport_localTransportDetails',
  transport_peers: 'transport_peers',
  keymgr_queryKeys: 'keymgr_queryKeys',
  keymgr_reverseKeyLookup: 'keymgr_reverseKeyLookup',
};
