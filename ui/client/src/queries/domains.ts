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

import i18next from 'i18next';
import { generatePostReq, returnResponse } from './common';
import { RpcEndpoint, RpcMethods } from './rpcMethods';

export const listDomains = async (): Promise<string[]> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_listDomains,
    params: [],
  };
  return <Promise<string[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomains')
    )
  );
};

export const getDomainByName = async (name: string): Promise<any> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_getDomain,
    params: [name],
  };

  return <Promise<any>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomain')
    )
  );
};

export const querySmartContractsByDomain = async (
  domainAddress: string
): Promise<any> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.domain_querySmartContracts,
    params: [
      {
        limit: 100, // TODO: pagination
        equal: [{ field: 'domainAddress', value: domainAddress }],
      },
    ],
  };
  return <Promise<any>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingSmartContracts')
    )
  );
};

export const fetchDomainReceipt = async (
  domain: string,
  transactionId: string
): Promise<any> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_getDomainReceipt,
    params: [domain, transactionId],
  };

  return <Promise<any>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingDomainReceipt')
    )
  );
};
