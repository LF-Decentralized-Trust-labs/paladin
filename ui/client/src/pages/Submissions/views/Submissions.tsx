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

import { SubmissionDetails } from "@/components/Accordions/SubmissionDetails";
import { SubmissionCard } from "@/components/Cards/SubmissionCard";
import PageLayout from "@/components/Layouts/PageLayout";
import { PaladinTransactionListItem } from "@/components/Lists/PaladinTransactionListItem";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Config } from "@/config";
import { IPaladinTransaction } from "@/interfaces/transactions";
import { usePtxQueries } from "@/queries/ptx";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

export const Submissions: React.FC = () => {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentTab = (searchParams.get("tab") as "all" | "pending") || "all";
  const [selectedSubmission, setSelectedSubmission] = useState<
    IPaladinTransaction | undefined
  >(undefined);

  const { useFetchSubmissions } = usePtxQueries();
  const { data: transactions, isLoading: isLoadingTransactions } =
    useFetchSubmissions(currentTab === "pending" ? "pending" : "all", {
      limit: Config.PENDING_TRANSACTIONS_QUERY_LIMIT,
      sort: ["created DESC"],
    });

  useEffect(() => {
    let submissionId = searchParams.get("submissionId");
    if (submissionId === null) {
      submissionId = transactions?.[0]?.id ?? null;
    }
    const submission = transactions?.find((tx) => tx.id === submissionId);
    setSelectedSubmission(submission);
  }, [searchParams, transactions]);

  return (
    <PageLayout breadcrumbs={[{ title: t("submissions") }]} noPadding>
      <div className="grid grid-cols-3">
        <div className="border-r col-span-1">
          <div className="p-2 border-b border-border">
            <Tabs
              className="w-full border rounded"
              defaultValue="all"
              value={currentTab}
              onValueChange={(value) => {
                setSearchParams(
                  (prev) => {
                    prev.set("tab", value);
                    return prev;
                  },
                  { replace: true }
                );
              }}
            >
              <TabsList className="w-full">
                <TabsTrigger className="w-1/2" value="all">
                  {t("all")}
                </TabsTrigger>
                <TabsTrigger className="w-1/2" value="pending">
                  {t("pending")}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
          <ScrollArea className=" h-[calc(100vh-64px-55px)] ">
            <div className="flex flex-col">
              {isLoadingTransactions &&
                Array.from(Array(10)).map((_, idx) => {
                  return (
                    <PaladinTransactionListItem
                      key={`pending-tx-loader-${idx}`}
                      paladinTransaction={undefined}
                      isLoading={isLoadingTransactions}
                      onClick={() => {}}
                      isSelected={false}
                    />
                  );
                })}
              {transactions?.map((transaction) => (
                <PaladinTransactionListItem
                  key={transaction.id}
                  paladinTransaction={transaction}
                  isLoading={isLoadingTransactions}
                  onClick={() =>
                    setSearchParams({ submissionId: transaction.id })
                  }
                  isSelected={selectedSubmission?.id === transaction.id}
                />
              ))}
              {!isLoadingTransactions && transactions?.length === 0 && (
                <div className="p-10 flex justify-center items-center">
                  <p className="text-muted-foreground">
                    {t("noPendingSubmissions")}
                  </p>
                </div>
              )}
            </div>
          </ScrollArea>
        </div>
        {selectedSubmission && (
          <div className="p-4 space-y-4 h-[calc(100vh-64px-64px)] w-full col-span-2">
            <div className="space-y-4">
              <SubmissionCard
                paladinTransaction={selectedSubmission}
                isLoading={isLoadingTransactions}
              />
              <SubmissionDetails
                mode="properties"
                paladinTransaction={selectedSubmission}
              />
              <SubmissionDetails
                mode="details"
                paladinTransaction={selectedSubmission}
              />
            </div>
          </div>
        )}
      </div>
    </PageLayout>
  );
};
