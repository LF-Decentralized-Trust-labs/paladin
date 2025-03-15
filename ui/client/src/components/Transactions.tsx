// Copyright © 2025 Kaleido, Inc.
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

import { Alert, Box, LinearProgress, Typography, useTheme } from "@mui/material";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useContext } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import {
  fetchIndexedTransactions,
} from "../queries/transactions";
import { Transaction } from "./Transaction";
import { getAltModeScrollBarStyle } from "../themes/default";
import InfiniteScroll from "react-infinite-scroll-component";
import { ITransaction } from "../interfaces";
import { useTranslation } from "react-i18next";


export const Transactions: React.FC = () => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);

  const theme = useTheme();
  const { t } = useTranslation();

  const { data: transactions, fetchNextPage, hasNextPage, error } = useInfiniteQuery({
    queryKey: ["transactions", lastBlockWithTransactions],
    queryFn: ({ pageParam }) => fetchIndexedTransactions(pageParam),
    initialPageParam: undefined as ITransaction | undefined,
    getNextPageParam: (lastPage) => { return lastPage[lastPage.length - 1] },
  });

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  if (transactions?.pages === undefined) {
    return <></>;
  }

  return (
    <>
      <Typography align="center" variant="h5" sx={{ marginBottom: '20px' }}>
        {t("receipts")}
      </Typography>
      <Box
        id="scrollableDivEventsTransactions"
        sx={{
          height: "calc(100vh - 170px)",
          paddingRight: "15px",
          ...getAltModeScrollBarStyle(theme.palette.mode)
        }}
      >
        <InfiniteScroll
          scrollableTarget="scrollableDivEventsTransactions"
          dataLength={transactions.pages.length}
          next={() => fetchNextPage()}
          hasMore={hasNextPage}
          loader={<LinearProgress />}
        >
          {transactions.pages.map(transactionArray => transactionArray.map((transaction) => (
            <Transaction
              key={transaction.hash}
              transaction={transaction}
              transactionReceipts={transaction.receipts}
              paladinTransactions={transaction.paladinTransactions}
            />
          )))}
        </InfiniteScroll>
      </Box>
    </>
  );
};
