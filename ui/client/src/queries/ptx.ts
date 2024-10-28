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
import { IStoredABI } from "@/interfaces/abi";
import {
  IPaladinTransaction,
  IPublicTxWithBinding,
  ITransaction,
  ITransactionDependencies,
  ITransactionReceipt,
} from "@/interfaces/transactions";
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

export const PtxMethods = {
  sendTransaction: "ptx_sendTransaction",
  sendTransactions: "ptx_sendTransactions",
  call: "ptx_call",
  getTransaction: "ptx_getTransaction",
  getTransactionFull: "ptx_getTransactionFull",
  getTransactionByIdempotencyKey: "ptx_getTransactionByIdempotencyKey",
  queryTransactions: "ptx_queryTransactions",
  queryTransactionsFull: "ptx_queryTransactionsFull",
  queryPendingTransactions: "ptx_queryPendingTransactions",
  getTransactionReceipt: "ptx_getTransactionReceipt",
  queryTransactionReceipts: "ptx_queryTransactionReceipts",
  getTransactionDependencies: "ptx_getTransactionDependencies",
  queryPublicTransactions: "ptx_queryPublicTransactions",
  queryPendingPublicTransactions: "ptx_queryPendingPublicTransactions",
  getPublicTransactionByNonce: "ptx_getPublicTransactionByNonce",
  getPublicTransactionByHash: "ptx_getPublicTransactionByHash",
  storeABI: "ptx_storeABI",
  getStoredABI: "ptx_getStoredABI",
  queryStoredABIs: "ptx_queryStoredABIs",
  resolveVerifier: "ptx_resolveVerifier",
};

export const PtxQueries = {
  sendTransaction: async (tx: object): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.sendTransaction,
      params: [tx],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorSendingTransaction")
    );
  },

  sendTransactions: async (txs: object[]): Promise<string[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.sendTransactions,
      params: [txs],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorSendingTransactions")
    );
  },

  call: async (tx: object): Promise<unknown> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.call,
      params: [tx],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorCallingTransaction")
    );
  },

  getTransaction: async (txID: string): Promise<ITransaction> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getTransaction,
      params: [txID],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransaction", { txID })
    );
  },

  getTransactionFull: async (txID: string): Promise<IPaladinTransaction> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getTransactionFull,
      params: [txID],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionFull", { txID })
    );
  },

  getTransactionByIdempotencyKey: async (
    idempotencyKey: string
  ): Promise<ITransaction> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getTransactionByIdempotencyKey,
      params: [idempotencyKey],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionByIdempotencyKey", { idempotencyKey })
    );
  },

  queryTransactions: async (query: object): Promise<ITransaction[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryTransactions,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingTransactions")
    );
  },

  queryTransactionsFull: async (
    query: PaladinQueryOpts
  ): Promise<IPaladinTransaction[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryTransactionsFull,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingTransactionsFull")
    );
  },

  queryPendingTransactions: async (
    query: PaladinQueryOpts,
    full: boolean
  ): Promise<IPaladinTransaction[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryPendingTransactions,
      params: [query, full],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingPendingTransactions")
    );
  },

  getTransactionReceipt: async (txID: string): Promise<ITransactionReceipt> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getTransactionReceipt,
      params: [txID],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionReceipt", { txID })
    );
  },

  queryTransactionReceipts: async (
    query: PaladinQueryOpts
  ): Promise<ITransactionReceipt[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryTransactionReceipts,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingTransactionReceipts")
    );
  },

  getTransactionDependencies: async (
    txID: string
  ): Promise<ITransactionDependencies> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getTransactionDependencies,
      params: [txID],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionDependencies", { txID })
    );
  },

  queryPublicTransactions: async (
    query: object
  ): Promise<IPublicTxWithBinding[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryPublicTransactions,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingPublicTransactions")
    );
  },

  queryPendingPublicTransactions: async (
    query: object
  ): Promise<IPublicTxWithBinding[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryPendingPublicTransactions,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingPendingPublicTransactions")
    );
  },

  getPublicTransactionByNonce: async (
    from: string,
    nonce: number
  ): Promise<IPublicTxWithBinding> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getPublicTransactionByNonce,
      params: [from, nonce],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingPublicTransactionByNonce", { from, nonce })
    );
  },

  getPublicTransactionByHash: async (
    hash: string
  ): Promise<IPublicTxWithBinding> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getPublicTransactionByHash,
      params: [hash],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingPublicTransactionByHash", { hash })
    );
  },

  storeABI: async (abi: object): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.storeABI,
      params: [abi],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorStoringABI")
    );
  },

  getStoredABI: async (hash: string): Promise<IStoredABI> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.getStoredABI,
      params: [hash],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingStoredABI", { hash })
    );
  },

  queryStoredABIs: async (query: object): Promise<IStoredABI[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.queryStoredABIs,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingStoredABIs")
    );
  },

  resolveVerifier: async (
    lookup: string,
    algorithm: string,
    verifierType: string
  ): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: PtxMethods.resolveVerifier,
      params: [lookup, algorithm, verifierType],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorResolvingVerifier")
    );
  },
};

export const usePtxQueries = (QueryKey = "ptx") => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);

  const useGetTransaction = (txID: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getTransaction, txID],
      queryFn: () => PtxQueries.getTransaction(txID),
    });
  };

  const useGetTransactionFull = (txID: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getTransactionFull, txID],
      queryFn: () => PtxQueries.getTransactionFull(txID),
    });
  };

  const useGetTransactionByIdempotencyKey = (idempotencyKey: string) => {
    return useQuery({
      queryKey: [
        QueryKey,
        PtxMethods.getTransactionByIdempotencyKey,
        idempotencyKey,
      ],
      queryFn: () => PtxQueries.getTransactionByIdempotencyKey(idempotencyKey),
    });
  };

  const useQueryTransactions = (query: object) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.queryTransactions, query],
      queryFn: () => PtxQueries.queryTransactions(query),
    });
  };

  const useQueryTransactionsFull = (query: PaladinQueryOpts) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.queryTransactionsFull, query],
      queryFn: () => PtxQueries.queryTransactionsFull(query),
      enabled: !!query.in?.[0]?.values?.length,
    });
  };

  const useQueryTransactionReceipts = (query: PaladinQueryOpts) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.queryTransactionReceipts, query],
      queryFn: () => PtxQueries.queryTransactionReceipts(query),
      enabled: !!query.in?.[0]?.values?.length,
    });
  };

  const useGetTransactionReceipt = (txID: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getTransactionReceipt, txID],
      queryFn: () => PtxQueries.getTransactionReceipt(txID),
    });
  };

  const useGetTransactionDependencies = (txID: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getTransactionDependencies, txID],
      queryFn: () => PtxQueries.getTransactionDependencies(txID),
    });
  };

  const useQueryPublicTransactions = (query: object) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.queryPublicTransactions, query],
      queryFn: () => PtxQueries.queryPublicTransactions(query),
    });
  };

  const useGetPublicTransactionByNonce = (from: string, nonce: number) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getPublicTransactionByNonce, from, nonce],
      queryFn: () => PtxQueries.getPublicTransactionByNonce(from, nonce),
    });
  };

  const useGetPublicTransactionByHash = (hash: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getPublicTransactionByHash, hash],
      queryFn: () => PtxQueries.getPublicTransactionByHash(hash),
    });
  };

  const useGetStoredABI = (hash: string) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.getStoredABI, hash],
      queryFn: () => PtxQueries.getStoredABI(hash),
    });
  };

  const useQueryStoredABIs = (query: object) => {
    return useQuery({
      queryKey: [QueryKey, PtxMethods.queryStoredABIs, query],
      queryFn: () => PtxQueries.queryStoredABIs(query),
    });
  };

  const useFetchSubmissions = (
    type: "all" | "pending",
    query: PaladinQueryOpts
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        PtxMethods.queryPendingTransactions,
        query,
        type,
        lastBlockWithTransactions,
      ],
      queryFn: () =>
        type === "all"
          ? PtxQueries.queryTransactionsFull(query)
          : PtxQueries.queryPendingTransactions(query, true),
    });
  };

  return {
    useGetTransaction,
    useGetTransactionFull,
    useGetTransactionByIdempotencyKey,
    useQueryTransactions,
    useQueryTransactionsFull,
    useGetTransactionReceipt,
    useGetTransactionDependencies,
    useQueryPublicTransactions,
    useGetPublicTransactionByNonce,
    useQueryTransactionReceipts,
    useGetPublicTransactionByHash,
    useGetStoredABI,
    useQueryStoredABIs,
    useFetchSubmissions,
  };
};
