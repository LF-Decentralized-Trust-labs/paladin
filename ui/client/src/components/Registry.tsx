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

import { Config } from "@/config";
import { IRegistryEntryWithProperties } from "@/interfaces/registry";
import { useRegQueries } from "@/queries/reg";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { RegistryListItem } from "./Lists/RegistryListItem";

type Props = {
  registryName: string;
  isLoading?: boolean;
  onClick: (registry: IRegistryEntryWithProperties) => void;
};

export const Registry: React.FC<Props> = ({
  registryName,
  isLoading,
  onClick,
}) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedRegistry, setSelectedRegistry] = useState<string | undefined>(
    undefined
  );

  const { useQueryEntriesWithProps } = useRegQueries();

  const { data: registryEntries } = useQueryEntriesWithProps(
    registryName,
    { limit: Config.REGISTRY_ENTRIES_QUERY_LIMIT },
    "any"
  );

  useEffect(() => {
    const registryName = searchParams.get("registryName");
    const registry = registryEntries?.find((reg) => reg.name === registryName);
    setSelectedRegistry(registry?.name);
  }, [searchParams, registryEntries]);

  return (
    <div>
      {registryEntries
        ?.filter((registryEntry) => registryEntry.name !== "root")
        .map((registryEntry) => (
          <RegistryListItem
            key={registryEntry.id}
            registryEntry={registryEntry}
            isLoading={isLoading}
            onClick={() => {
              setSearchParams({ registryName: registryEntry.name });
              onClick(registryEntry);
            }}
            isSelected={selectedRegistry === registryEntry.name}
          />
        ))}
    </div>
  );
};
