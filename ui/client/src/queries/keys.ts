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

import { IFilter, IKeyEntry, IKeyMappingAndVerifier } from "../interfaces";
import { translateFilters } from "../utils";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import i18next from "i18next";

export const fetchKeys = async (parent: string | undefined, limit: number, sortByPathFirst: boolean, sortOrder: 'asc' | 'desc', filters: IFilter[], refEntry?: IKeyEntry): Promise<IKeyEntry[]> => {

  let translatedFilters = translateFilters(filters);

  if (parent !== undefined) {
    if (translatedFilters.equal === undefined) {
      translatedFilters.equal = [];
    }
    translatedFilters.equal.push({
      field: 'parent',
      value: parent
    });
  }

  let requestPayload: any = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.keymgr_queryKeys,
    params: [{
      ...translatedFilters,
      sort: [`${sortByPathFirst? 'path' : 'index'} ${sortOrder}`, `${sortByPathFirst? 'index' : 'path'} ${sortOrder}`],
      limit
    }]
  };

  if (refEntry !== undefined) {
    const comparison = sortOrder === 'asc' ? 'greaterThan' : 'lessThan';
    const firstSortField = sortByPathFirst? 'path' : 'index';
    const secondSortField = sortByPathFirst? 'index' : 'path';
    requestPayload.params[0].or = [
      {
        [comparison]: [{
          field: firstSortField,
          value: refEntry[firstSortField]
        }]
      },
      {
        equal: [{
          field: firstSortField,
          value: refEntry[firstSortField]
        }],
        [comparison]: [{
          field: secondSortField,
          value: refEntry[secondSortField]
        }],
      }
    ];
  }

  return <Promise<IKeyEntry[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingKeys")
    )
  );
};

export const reverseKeyLookup = async (algorithm: string, verifierType: string, verifier: string): Promise<IKeyMappingAndVerifier> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.keymgr_reverseKeyLookup,
    params: [algorithm, verifierType, verifier]
  };

  return <Promise<IKeyMappingAndVerifier>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingReverseKeyLookup"), []
    )
  );
};
