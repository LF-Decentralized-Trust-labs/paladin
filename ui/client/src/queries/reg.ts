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
import {
  ActiveFilter,
  IRegistryEntry,
  IRegistryEntryWithProperties,
} from "@/interfaces/registry";
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

export const RegMethods = {
  registries: "reg_registries",
  queryEntries: "reg_queryEntries",
  queryEntriesWithProps: "reg_queryEntriesWithProps",
  getEntryProperties: "reg_getEntryProperties",
};

export const RegQueries = {
  listRegistries: async (): Promise<string[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: RegMethods.registries,
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorListingRegistries")
    );
  },

  queryEntries: async (
    registryName: string,
    query: object,
    activeFilter: ActiveFilter
  ): Promise<IRegistryEntry[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: RegMethods.queryEntries,
      params: [registryName, query, activeFilter],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingEntries", { registryName })
    );
  },

  queryEntriesWithProps: async (
    registryName: string,
    query: PaladinQueryOpts,
    activeFilter: ActiveFilter
  ): Promise<IRegistryEntryWithProperties[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: RegMethods.queryEntriesWithProps,
      params: [registryName, query, activeFilter],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorQueryingEntriesWithProps", { registryName })
    );
  },

  getEntryProperties: async (
    registryName: string,
    entryID: string,
    activeFilter: ActiveFilter
  ): Promise<IRegistryEntryWithProperties[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: RegMethods.getEntryProperties,
      params: [registryName, entryID, activeFilter],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorGettingEntryProperties", { registryName, entryID })
    );
  },
};

export const useRegQueries = (QueryKey = "reg") => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);

  const useListRegistries = () => {
    return useQuery({
      queryKey: [QueryKey, RegMethods.registries, lastBlockWithTransactions],
      queryFn: () =>
        RegQueries.listRegistries().then((registries) => registries.sort()),
    });
  };

  const useQueryEntries = (
    registryName: string,
    query: object,
    activeFilter: ActiveFilter
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        RegMethods.queryEntries,
        registryName,
        query,
        activeFilter,
      ],
      queryFn: () =>
        RegQueries.queryEntries(registryName, query, activeFilter).then(
          (registries) => registries.sort((a, b) => (a.name < b.name ? -1 : 0))
        ),
      enabled: !!registryName,
    });
  };

  const useQueryEntriesWithProps = (
    registryName: string,
    query: PaladinQueryOpts,
    activeFilter: ActiveFilter
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        RegMethods.queryEntriesWithProps,
        registryName,
        query,
        activeFilter,
      ],
      queryFn: () =>
        RegQueries.queryEntriesWithProps(registryName, query, activeFilter),
      enabled: !!registryName,
    });
  };

  const useGetEntryProperties = (
    registryName: string,
    entryID: string,
    activeFilter: ActiveFilter
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        RegMethods.getEntryProperties,
        registryName,
        entryID,
        activeFilter,
      ],
      queryFn: () =>
        RegQueries.getEntryProperties(registryName, entryID, activeFilter),
      enabled: !!registryName && !!entryID,
    });
  };

  return {
    useListRegistries,
    useQueryEntries,
    useQueryEntriesWithProps,
    useGetEntryProperties,
  };
};
