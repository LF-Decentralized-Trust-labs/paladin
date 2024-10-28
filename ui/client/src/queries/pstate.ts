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

import { ApplicationContext } from "@/contexts/ApplicationContext";
import { ISchema, IState } from "@/interfaces/pstate";
import { useQuery } from "@tanstack/react-query";
import i18next from "i18next";
import { useContext } from "react";
import {
  DefaultRpcPayload,
  generatePostReq,
  PaladinQueryOpts,
  returnResponse,
  RpcEndpoint,
} from "./common";

export const PStateMethods = {
  listSchemas: "pstate_listSchemas",
  storeState: "pstate_storeState",
  queryStates: "pstate_queryStates",
  queryContractStates: "pstate_queryContractStates",
};

export const PStateQueries = {
  listSchemas: async (): Promise<ISchema[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PStateMethods.listSchemas,
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorListingSchemas")
    );
  },
  storeState: async (
    contractAddress: string,
    schema: string,
    data: object
  ): Promise<IState> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PStateMethods.storeState,
      params: [contractAddress, schema, data],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorStoringState")
    );
  },

  queryStates: async (
    schema: string,
    query: PaladinQueryOpts,
    status: string
  ): Promise<IState[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PStateMethods.queryStates,
      params: [schema, query, status],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingStates")
    );
  },

  queryContractStates: async (
    contractAddress: string,
    schema: string,
    query: PaladinQueryOpts,
    status: string
  ): Promise<IState[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PStateMethods.queryContractStates,
      params: [contractAddress, schema, query, status],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingContractStates")
    );
  },
};

export const usePStateQueries = (QueryKey = "pstate") => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);

  const useListSchemas = () => {
    return useQuery({
      queryKey: [
        QueryKey,
        PStateMethods.listSchemas,
        lastBlockWithTransactions,
      ],
      queryFn: () => PStateQueries.listSchemas(),
    });
  };

  const useStoreState = (
    contractAddress: string,
    schema: string,
    data: object
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        PStateMethods.storeState,
        contractAddress,
        schema,
        data,
      ],
      queryFn: () => PStateQueries.storeState(contractAddress, schema, data),
    });
  };

  const useQueryStates = (
    schema: string,
    query: PaladinQueryOpts,
    status: string
  ) => {
    return useQuery({
      queryKey: [QueryKey, PStateMethods.queryStates, schema, query, status],
      queryFn: () => PStateQueries.queryStates(schema, query, status),
    });
  };

  const useQueryContractStates = (
    contractAddress: string,
    schema: string,
    query: PaladinQueryOpts,
    status: string
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        PStateMethods.queryContractStates,
        contractAddress,
        schema,
        query,
        status,
      ],
      queryFn: () =>
        PStateQueries.queryContractStates(
          contractAddress,
          schema,
          query,
          status
        ),
    });
  };

  return {
    useListSchemas,
    useStoreState,
    useQueryStates,
    useQueryContractStates,
  };
};
