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

import { IPaladinTransaction } from "@/interfaces/transactions";
import { t } from "i18next";
import { HashChip } from "../Chips/HashChip";
import { TimeText } from "../TimeText";
import { Card, CardContent } from "../ui/card";
import { Skeleton } from "../ui/skeleton";

type Props = {
  paladinTransaction: IPaladinTransaction | undefined;
  isLoading?: boolean;
  onClick: () => void;
  isSelected?: boolean;
};

export const PaladinTransactionListItem: React.FC<Props> = ({
  paladinTransaction,
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
          <HashChip
            hash={paladinTransaction?.id ?? ""}
            preText={t("submissionId")}
          />
          <HashChip
            hash={paladinTransaction?.from ?? ""}
            isAddress
            truncate={false}
            preText={t("from")}
          />
          {paladinTransaction?.domain && (
            <HashChip
              hash={paladinTransaction.domain ?? ""}
              preText={t("domain")}
            />
          )}
        </div>
        <div className="flex justify-between items-center pt-2">
          <p className="text-muted-foreground text-base font-normal">
            {isLoading ? (
              <Skeleton className="h-6 w-20" />
            ) : (
              t(paladinTransaction?.type ?? "")
            )}
          </p>
          {isLoading ? (
            <Skeleton className="h-4 w-14" />
          ) : (
            paladinTransaction?.created && (
              <TimeText time={paladinTransaction?.created ?? ""} />
            )
          )}
        </div>
      </CardContent>
    </Card>
  );
};
