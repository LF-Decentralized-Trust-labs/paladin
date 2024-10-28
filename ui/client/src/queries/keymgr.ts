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

import { KeyMappingAndVerifier, WalletInfo } from "@/interfaces/keymgr";
import { useQuery } from "@tanstack/react-query";
import i18next from "i18next";
import {
  DefaultRpcPayload,
  generatePostReq,
  returnResponse,
  RpcEndpoint,
} from "./common";

export const KeyMgrMethods = {
  wallets: "keymgr_wallets",
  resolveKey: "keymgr_resolveKey",
  resolveEthAddress: "keymgr_resolveEthAddress",
  reverseKeyLookup: "keymgr_reverseKeyLookup",
};

export const KeyMgrQueries = {
  wallets: async (): Promise<WalletInfo[]> => {
    const payload = {
      ...DefaultRpcPayload,
      method: KeyMgrMethods.wallets,
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorListingWallets")
    );
  },
  resolveKey: async (
    identifier: string,
    algorithm: string,
    verifierType: string
  ): Promise<KeyMappingAndVerifier> => {
    const payload = {
      ...DefaultRpcPayload,
      method: KeyMgrMethods.resolveKey,
      params: [identifier, algorithm, verifierType],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorResolvingKey")
    );
  },
  resolveEthAddress: async (identifier: string): Promise<string> => {
    const payload = {
      ...DefaultRpcPayload,
      method: KeyMgrMethods.resolveEthAddress,
      params: [identifier],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorResolvingEthAddress")
    );
  },
  reverseKeyLookup: async (
    algorithm: string,
    verifierType: string,
    verifier: string
  ): Promise<KeyMappingAndVerifier> => {
    const payload = {
      ...DefaultRpcPayload,
      method: KeyMgrMethods.reverseKeyLookup,
      params: [algorithm, verifierType, verifier],
    };
    return returnResponse(
      await fetch(RpcEndpoint, generatePostReq(JSON.stringify(payload))),
      i18next.t("errorReverseKeyLookup")
    );
  },
};

export const useKeyMgrQueries = (QueryKey = "keymgr") => {
  const useListWallets = () => {
    return useQuery({
      queryKey: [QueryKey, KeyMgrMethods.wallets],
      queryFn: () => KeyMgrQueries.wallets(),
    });
  };

  const useResolveKey = (
    identifier: string,
    algorithm: string,
    verifierType: string
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        KeyMgrMethods.resolveKey,
        identifier,
        algorithm,
        verifierType,
      ],
      queryFn: () =>
        KeyMgrQueries.resolveKey(identifier, algorithm, verifierType),
    });
  };

  const useResolveEthAddress = (identifier: string) => {
    return useQuery({
      queryKey: [QueryKey, KeyMgrMethods.resolveEthAddress, identifier],
      queryFn: () => KeyMgrQueries.resolveEthAddress(identifier),
    });
  };

  const useReverseKeyLookup = (
    algorithm: string,
    verifierType: string,
    verifier: string
  ) => {
    return useQuery({
      queryKey: [
        QueryKey,
        KeyMgrMethods.reverseKeyLookup,
        algorithm,
        verifierType,
        verifier,
      ],
      queryFn: () =>
        KeyMgrQueries.reverseKeyLookup(algorithm, verifierType, verifier),
    });
  };

  return {
    useListWallets,
    useResolveKey,
    useResolveEthAddress,
    useReverseKeyLookup,
  };
};
