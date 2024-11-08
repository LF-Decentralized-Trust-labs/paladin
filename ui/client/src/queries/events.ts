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
import { IEvent } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";

export const fetchEvents = async (pageParam?: IEvent): Promise<IEvent[]> => {
  let requestPayload: any = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.bidx_QueryIndexedEvents,
    params: [
      {
        limit: constants.EVENT_QUERY_LIMIT,
        sort: ["blockNumber DESC", "transactionIndex DESC", "logIndex DESC"],
      }
    ]
  };

  if (pageParam !== undefined) {
    requestPayload.params[0].or = [
      {
        "lessThan": [
          {
            "field": "blockNumber",
            "value": pageParam.blockNumber
          }
        ]
      },
      {
        "equal": [{
          "field": "blockNumber",
          "value": pageParam.blockNumber
        }],
        "or": [
          {
            "lessThan": [{
              "field": "transactionIndex",
              "value": pageParam.transactionIndex
            }]
          },
          {
            "equal": [{
              "field": "transactionIndex",
              "value": pageParam.transactionIndex
            }],
            "lessThan": [{
              "field": "logIndex",
              "value": pageParam.logIndex
            }]
          }
        ]
      }
    ];
  }

  return <Promise<IEvent[]>>(
    returnResponse(
      () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
      i18next.t("errorFetchingLatestEvents")
    )
  );
};
