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
import { IBlock } from "@/interfaces/block";
import { IEvent } from "@/interfaces/events";
import { ITransaction } from "@/interfaces/transactions";
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

export const BidxMethods = {
  decodeTransactionEvents: "bidx_decodeTransactionEvents",
  getBlockByNumber: "bidx_getBlockByNumber",
  getBlockTransactionsByNumber: "bidx_getBlockTransactionsByNumber",
  getConfirmedBlockHeight: "bidx_getConfirmedBlockHeight",
  getTransactionByHash: "bidx_getTransactionByHash",
  getTransactionByNonce: "bidx_getTransactionByNonce",
  getTransactionEventsByHash: "bidx_getTransactionEventsByHash",
  queryIndexedBlocks: "bidx_queryIndexedBlocks",
  queryIndexedEvents: "bidx_queryIndexedEvents",
  queryIndexedTransactions: "bidx_queryIndexedTransactions",
};

export const BidxQueries = {
  decodeTransactionEvents: async (
    hash: string,
    abi: string,
    resultFormat: string
  ): Promise<IEvent[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.decodeTransactionEvents,
      params: [hash, abi, resultFormat],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorDecodingTransactionEvents")
    );
  },
  getBlockByNumber: async (number: number): Promise<IBlock> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getBlockByNumber,
      params: [number],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingBlockByNumber", { number })
    );
  },
  getTransactionByHash: async (hash: string): Promise<ITransaction> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getTransactionByHash,
      params: [hash],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionByHash", { hash })
    );
  },

  getTransactionByNonce: async (
    from: string,
    nonce: number
  ): Promise<ITransaction> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getTransactionByNonce,
      params: [from, nonce],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionByNonce", { from, nonce })
    );
  },

  getBlockTransactionsByNumber: async (
    blockNumber: number
  ): Promise<ITransaction[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getBlockTransactionsByNumber,
      params: [blockNumber],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingBlockTransactions", { blockNumber })
    );
  },

  getTransactionEventsByHash: async (hash: string): Promise<IEvent[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getTransactionEventsByHash,
      params: [hash],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingTransactionEvents", { hash })
    );
  },

  getConfirmedBlockHeight: async (): Promise<number> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.getConfirmedBlockHeight,
      params: [],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorFetchingConfirmedBlockHeight")
    );
  },

  queryIndexedBlocks: async (query: object): Promise<IBlock[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.queryIndexedBlocks,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingIndexedBlocks")
    );
  },

  queryIndexedTransactions: async (query: object): Promise<ITransaction[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.queryIndexedTransactions,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingIndexedTransactions")
    );
  },

  queryIndexedEvents: async (query: object): Promise<IEvent[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: BidxMethods.queryIndexedEvents,
      params: [query],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingIndexedEvents")
    );
  },
};

export const useBidxQueries = (QueryKey = "bidx") => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  // Blocks
  const useGetBlockByNumber = (number: number) => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.getBlockByNumber, number],
      queryFn: () => BidxQueries.getBlockByNumber(number),
    });
  };

  const useGetBlockTransactionsByNumber = (blockNumber: number) => {
    return useQuery({
      queryKey: [
        QueryKey,
        BidxMethods.getBlockTransactionsByNumber,
        blockNumber,
      ],
      queryFn: () => BidxQueries.getBlockTransactionsByNumber(blockNumber),
    });
  };

  const useGetConfirmedBlockHeight = () => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.getConfirmedBlockHeight],
      queryFn: () => BidxQueries.getConfirmedBlockHeight(),
    });
  };

  const useQueryIndexedBlocks = (query: object) => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.queryIndexedBlocks, query],
      queryFn: () => BidxQueries.queryIndexedBlocks(query),
    });
  };

  // Transactions
  const useGetTransactionByHash = (hash: string) => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.getTransactionByHash, hash],
      queryFn: () => BidxQueries.getTransactionByHash(hash),
    });
  };

  const useGetTransactionByNonce = (from: string, nonce: number) => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.getTransactionByNonce, from, nonce],
      queryFn: () => BidxQueries.getTransactionByNonce(from, nonce),
    });
  };

  const useGetTransactionEventsByHash = (hash: string) => {
    return useQuery({
      queryKey: [QueryKey, BidxMethods.getTransactionEventsByHash, hash],
      queryFn: () => BidxQueries.getTransactionEventsByHash(hash),
    });
  };

  const useQueryIndexedTransactions = (
    query: PaladinQueryOpts,
    refetchInterval?: number
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        BidxMethods.queryIndexedTransactions,
        query,
        lastBlockWithTransactions,
      ],
      queryFn: () => BidxQueries.queryIndexedTransactions(query),
      refetchInterval: refetchInterval ?? false,
      retry: (failureCount) => {
        return failureCount < 1;
      },
    });
  };

  // Events
  const useQueryIndexedEvents = (query: PaladinQueryOpts) => {
    return useQuery({
      queryKey: [
        QueryKey,
        BidxMethods.queryIndexedEvents,
        query,
        lastBlockWithTransactions,
      ],
      queryFn: () => BidxQueries.queryIndexedEvents(query),
    });
  };

  const useDecodeTransactionEvents = (
    hash: string,
    abi: string,
    resultFormat: string
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        BidxMethods.decodeTransactionEvents,
        hash,
        abi,
        resultFormat,
      ],
      queryFn: () =>
        BidxQueries.decodeTransactionEvents(hash, abi, resultFormat),
    });
  };

  return {
    useGetBlockByNumber,
    useGetTransactionByHash,
    useGetTransactionByNonce,
    useGetBlockTransactionsByNumber,
    useGetTransactionEventsByHash,
    useGetConfirmedBlockHeight,
    useQueryIndexedBlocks,
    useQueryIndexedTransactions,
    useQueryIndexedEvents,
    useDecodeTransactionEvents,
  };
};
