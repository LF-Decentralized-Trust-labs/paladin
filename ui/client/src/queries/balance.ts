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

import i18next from 'i18next';
import { generatePostReq, returnResponse } from './common';
import { RpcEndpoint, RpcMethods } from './rpcMethods';

export interface BalanceOfParams {
  account: string;
}

export interface BalanceOfResult {
  totalBalance: string;
  totalStates: string;
  overflow: boolean;
}

export const callBalanceOf = async (
  domain: string,
  contractAddress: string,
  from: string,
  params: BalanceOfParams
): Promise<BalanceOfResult> => {
  const payload = {
    jsonrpc: '2.0',
    id: Date.now(),
    method: RpcMethods.ptx_call,
    params: [
      {
        type: 'private',
        domain,
        function: 'balanceOf',
        from,
        to: contractAddress,
        data: params,
        abi: [
          {
            inputs: [
              {
                internalType: 'string',
                name: 'account',
                type: 'string',
              },
            ],
            name: 'balanceOf',
            outputs: [
              {
                internalType: 'uint256',
                name: 'totalStates',
                type: 'uint256',
              },
              {
                internalType: 'uint256',
                name: 'totalBalance',
                type: 'uint256',
              },
              {
                internalType: 'bool',
                name: 'overflow',
                type: 'bool',
              },
            ],
            stateMutability: 'view',
            type: 'function',
          },
        ],
      },
    ],
  };

  return <Promise<BalanceOfResult>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t('errorFetchingBalance')
    )
  );
};
