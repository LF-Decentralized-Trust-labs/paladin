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

import { useQuery } from "@tanstack/react-query";
import i18next from "i18next";
import {
  DefaultRpcPayload,
  generatePostReq,
  returnResponse,
  RpcEndpoint,
} from "./common";

export const TransportMethods = {
  nodeName: "transport_nodeName",
  localTransports: "transport_localTransports",
  localTransportDetails: "transport_localTransportDetails",
};

export const TransportQueries = {
  nodeName: async (): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: TransportMethods.nodeName,
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorListingSchemas")
    );
  },

  localTransports: async (): Promise<string[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: TransportMethods.localTransports,
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorListingLocalTransports")
    );
  },

  localTransportDetails: async (transportName: string): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: TransportMethods.localTransportDetails,
      params: [transportName],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorGettingTransportDetails", { transportName })
    );
  },
};

export const useTransportQueries = (QueryKey = "transport") => {
  const useNodeName = () => {
    return useQuery({
      queryKey: [QueryKey, TransportMethods.nodeName],
      queryFn: () => TransportQueries.nodeName(),
    });
  };

  const useLocalTransports = () => {
    return useQuery({
      queryKey: [QueryKey, TransportMethods.localTransports],
      queryFn: () => TransportQueries.localTransports(),
    });
  };

  const useLocalTransportDetails = (transportName: string) => {
    return useQuery({
      queryKey: [
        QueryKey,
        TransportMethods.localTransportDetails,
        transportName,
      ],
      queryFn: () => TransportQueries.localTransportDetails(transportName),
    });
  };

  return {
    useNodeName,
    useLocalTransports,
    useLocalTransportDetails,
  };
};
