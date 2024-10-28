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

import { IRegistryEntryWithProperties } from "@/interfaces/registry";
import { t } from "i18next";
import { HashChip } from "../Chips/HashChip";
import { Badge } from "../ui/badge";
import { Card, CardContent } from "../ui/card";
import { Skeleton } from "../ui/skeleton";

type Props = {
  registryEntry: IRegistryEntryWithProperties | undefined;
  isLoading?: boolean;
  onClick: () => void;
  isSelected?: boolean;
};

export const RegistryListItem: React.FC<Props> = ({
  registryEntry,
  isLoading,
  onClick,
  isSelected,
}) => {
  return (
    <Card
      className={`cursor-pointer hover:bg-muted rounded-none ${
        isSelected ? "bg-muted" : ""
      }`}
      onClick={onClick}
    >
      <CardContent className="py-4">
        <div className="flex justify-start space-y-2 items-start flex-col">
          <div className="flex justify-between w-full">
            <div>
              <p className="text-muted-foreground text-lg ">
                {isLoading ? (
                  <Skeleton className="h-6 w-20" />
                ) : (
                  t(registryEntry?.name ?? "")
                )}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-base font-normal">
                {isLoading ? (
                  <Skeleton className="h-6 w-20" />
                ) : (
                  <Badge
                    variant={registryEntry?.active ? "default" : "outline"}
                  >
                    {t(registryEntry?.active ? "active" : "inactive")}
                  </Badge>
                )}
              </p>
            </div>
          </div>
          <HashChip
            hash={registryEntry?.registry ?? ""}
            truncate={false}
            preText={t("registry")}
          />
          <HashChip hash={registryEntry?.id ?? ""} preText={t("id")} />
        </div>
      </CardContent>
    </Card>
  );
};
