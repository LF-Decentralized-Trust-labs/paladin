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

import { ABIUploadResponse } from "../interfaces";
import { generatePostReq, returnResponse } from "./common";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";

export const uploadABI = async (
  abi: Object
): Promise<ABIUploadResponse> => {
  const payload = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: RpcMethods.ptx_storeABI,
    params: [abi],
  };

  return <Promise<ABIUploadResponse>>(
    returnResponse(
      () =>  fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))), "", []
    )
  );

};
