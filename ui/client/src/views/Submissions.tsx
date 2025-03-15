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

import { Alert, Box, Fade, LinearProgress, ToggleButton, ToggleButtonGroup, Typography, useTheme } from "@mui/material";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useContext, useEffect, useState } from "react";
import { PaladinTransaction } from "../components/PaladinTransaction";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { fetchSubmissions } from "../queries/transactions";
import { getAltModeScrollBarStyle } from "../themes/default";
import InfiniteScroll from "react-infinite-scroll-component";
import { IFilter, IPaladinTransaction } from "../interfaces";
import { useTranslation } from "react-i18next";
import { Filters } from "../components/Filters";
import { constants } from "../components/config";

export const Submissions: React.FC = () => {

  const getFiltersFromStorage = () => {
    const value = window.localStorage.getItem(constants.SUBMISSIONS_FILTERS_KEY);
    if (value !== null) {
      try {
        return JSON.parse(value);
      } catch (_err) { }
    }
    return [];
  };

  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  const [filters, setFilters] = useState<IFilter[]>(getFiltersFromStorage());
  const [tab, setTab] = useState<'all' | 'pending'>('all');

  const theme = useTheme();
  const { t } = useTranslation();

  const { data: transactions, fetchNextPage, hasNextPage, error } = useInfiniteQuery({
    queryKey: ["submissions", tab, lastBlockWithTransactions, filters],
    queryFn: ({ pageParam }) => fetchSubmissions(tab, filters, pageParam),
    initialPageParam: undefined as IPaladinTransaction | undefined,
    getNextPageParam: (lastPage) => { return lastPage.length > 0 ? lastPage[lastPage.length - 1] : undefined },
  });

  useEffect(() => {
    window.localStorage.setItem(constants.SUBMISSIONS_FILTERS_KEY, JSON.stringify(filters));
  }, [filters]);

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  return (
    <Fade timeout={600} in={true}>
      <Box
        sx={{
          padding: "20px",
          maxWidth: "1300px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Typography align="center" variant="h5">
          {t("transactions")}
        </Typography>
        <Box sx={{ marginTop: '15px', marginBottom: '25px', textAlign: 'center' }}>
          <ToggleButtonGroup exclusive onChange={(_event, value) => setTab(value)} value={tab}>
            <ToggleButton color="primary" value="all" sx={{ width: '130px', height: '45px' }}>{t('all')}</ToggleButton>
            <ToggleButton color="primary" value="pending" sx={{ width: '130px', height: '45px' }}>{t('pending')}</ToggleButton>
          </ToggleButtonGroup>
        </Box>
        <Box sx={{ marginBottom: '20px' }}>
          <Filters
            filterFields={[
              {
                label: t('id'),
                name: 'id',
                type: 'string',
                isUUID: true
              },
              {
                label: t('from'),
                name: 'from',
                type: 'string'
              },
              {
                label: t('to'),
                name: 'to',
                type: 'string',
                isHexValue: true
              },
              {
                label: t('type'),
                name: 'type',
                type: 'string'
              },
              {
                label: t('domain'),
                name: 'domain',
                type: 'string'
              }
            ]}
            filters={filters}
            setFilters={setFilters}
          />
        </Box>
        <Box
          id="scrollableDivSubmissions"
          sx={{
            paddingRight: "15px",
            height: "calc(100vh - 250px)",
            ...getAltModeScrollBarStyle(theme.palette.mode)
          }}
        >
          {transactions !== undefined &&
            <>
              <InfiniteScroll
                scrollableTarget="scrollableDivSubmissions"
                dataLength={transactions.pages.length}
                next={() => fetchNextPage()}
                hasMore={hasNextPage}
                loader={<LinearProgress />}
              >
                {transactions.pages.map(transactionsArray =>
                  transactionsArray.map(transaction => (
                    <PaladinTransaction
                      key={transaction.id}
                      paladinTransaction={transaction}
                    />
                  ))
                )}
              </InfiniteScroll>
              {transactions.pages.length === 1 && transactions.pages[0].length === 0 &&
                <Typography color="textSecondary" align="center" variant="h6" sx={{ marginTop: '40px' }}>{t('noPendingTransactions')}</Typography>}

            </>
          }
        </Box>
      </Box>
    </Fade>
  );
};
