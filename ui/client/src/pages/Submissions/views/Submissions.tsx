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

import { PendingTransaction } from "@/components/PaladinTransaction";
import { Config } from "@/config";
import { usePtxQueries } from "@/queries/ptx";
import { Box, Fade, Paper, Tab, Tabs } from "@mui/material";
import { t } from "i18next";
import { useState } from "react";

export const Submissions: React.FC = () => {
  const [tab, setTab] = useState(0);
  const { useFetchSubmissions } = usePtxQueries();

  const { data: pendingTransactions, isLoading } = useFetchSubmissions(
    tab === 0 ? "all" : "pending",
    {
      limit: Config.PENDING_TRANSACTIONS_QUERY_LIMIT,
      sort: ["created DESC"],
    }
  );

  if (isLoading) {
    return <></>;
  }

  return (
    <Fade timeout={800} in={true}>
      <Box
        sx={{
          padding: "20px",
          maxWidth: "1200px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Paper
          sx={{
            padding: "10px",
            paddingTop: "12px",
            backgroundColor: (theme) =>
              theme.palette.mode === "light"
                ? "rgba(255, 255, 255, .65)"
                : "rgba(60, 60, 60, .65)",
          }}
        >
          <Tabs
            value={tab}
            onChange={(_event, value) => setTab(value)}
            centered
          >
            <Tab label={t("all")} />
            <Tab label={t("pending")} />
          </Tabs>

          <Box
            sx={{
              padding: "20px",
              overflow: "scroll",
              height: "calc(100vh - 162px)",
            }}
          >
            {pendingTransactions?.map((pendingTransaction) => (
              <PendingTransaction
                key={pendingTransaction.id}
                paladinTransaction={pendingTransaction}
              />
            ))}
          </Box>
        </Paper>
      </Box>
    </Fade>
  );
};
