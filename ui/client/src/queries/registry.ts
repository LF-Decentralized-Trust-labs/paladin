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

import i18next from "i18next";
import { constants } from "../components/config";
import { IFilter, IRegistryEntry } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import { translateFilters } from "../utils";

export const fetchRegistries = async (): Promise<string[]> => {
  const requestPayload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.reg_Registries,
  };

  return <Promise<string[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingRegistries")
    )
  );
};

export const fetchRegistryEntries = async (
  registryName: string,
  filters: IFilter[],
  tab: 'active' | 'inactive' | 'any',
  pageParam?: IRegistryEntry
): Promise<IRegistryEntry[]> => {

  let translatedFilters = translateFilters(filters);


  

  let requestPayload: any = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.reg_QueryEntriesWithProps,
    params: [
      registryName,
      {
        ...translatedFilters,
        limit: constants.REGISTRY_ENTRIES_QUERY_LIMIT,
        sort: ['.name ASC']
      },
      tab,
    ],
  };

  if(pageParam !== undefined) {
    requestPayload.params[1].greaterThan = [
      {
        "field": ".name",
        "value": pageParam.name
      }
    ];
  }

  return <Promise<IRegistryEntry[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingRegistryEntries")
    )
  );
};
