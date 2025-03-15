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

import { Alert, Box, LinearProgress, ToggleButton, ToggleButtonGroup, Typography, useTheme } from "@mui/material";
import { useInfiniteQuery } from "@tanstack/react-query";
import { fetchRegistryEntries } from "../queries/registry";
import { RegistryEntry } from "./RegistryEntry";
import { IFilter, IRegistryEntry } from "../interfaces";
import { getAltModeScrollBarStyle } from "../themes/default";
import InfiniteScroll from "react-infinite-scroll-component";
import { useTranslation } from "react-i18next";
import { Filters } from "./Filters";
import { useEffect, useState } from "react";
import { constants } from "./config";

type Props = {
  registryName: string;
};

export const Registry: React.FC<Props> = ({ registryName }) => {

  const getFiltersFromStorage = () => {
    const value = window.localStorage.getItem(constants.REGISTRY_FILTERS);
    if (value !== null) {
      try {
        return JSON.parse(value);
      } catch (_err) { }
    }
    return [];
  };

  const [filters, setFilters] = useState<IFilter[]>(getFiltersFromStorage());
  const [tab, setTab] = useState<'active' | 'inactive' | 'any'>('any');
  const theme = useTheme();
  const { t } = useTranslation();

  const { data: registryEntries, fetchNextPage, hasNextPage, error } = useInfiniteQuery({
    queryKey: ["registryEntries", tab, filters],
    queryFn: ({ pageParam }) => fetchRegistryEntries(registryName, filters, tab, pageParam),
    initialPageParam: undefined as IRegistryEntry | undefined,
    getNextPageParam: (lastPage) => { return lastPage.length > 0 ? lastPage[lastPage.length - 1] : undefined },
  });

  useEffect(() => {
    window.localStorage.setItem(constants.REGISTRY_FILTERS, JSON.stringify(filters));
  }, [filters]);

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  if (registryEntries?.pages === undefined) {
    return <></>;
  }

  return (
    <>
      <Box sx={{ margin: '10px', textAlign: 'center' }}>
        <ToggleButtonGroup exclusive onChange={(_event, value) => setTab(value)} value={tab}>
          <ToggleButton color="primary" value="any" sx={{ width: '130px', height: '45px' }}>{t('all')}</ToggleButton>
          <ToggleButton color="primary" value="active" sx={{ width: '130px', height: '45px' }}>{t('active')}</ToggleButton>
          <ToggleButton color="primary" value="inactive" sx={{ width: '130px', height: '45px' }}>{t('inactive')}</ToggleButton>
        </ToggleButtonGroup>
      </Box>
      <Box sx={{ margin: '15px' }}>
        <Filters
          filterFields={[
            {
              label: t('id'),
              name: '.id',
              type: 'string'
            },
            {
              label: t('owner'),
              name: '$owner',
              type: 'string'
            },
            {
              label: t('name'),
              name: '.name',
              type: 'string'
            }
          ]}
          filters={filters}
          setFilters={setFilters}
        />
      </Box>
      <Box
        id="scrollableDivRegistryEntries"
        sx={{
          paddingRight: "15px",
          height: "calc(100vh - 300px)",
          ...getAltModeScrollBarStyle(theme.palette.mode)
        }}
      >
        <InfiniteScroll
          scrollableTarget="scrollableDivRegistryEntries"
          dataLength={registryEntries.pages.length}
          next={() => fetchNextPage()}
          hasMore={hasNextPage}
          loader={<LinearProgress />}
        >
          {registryEntries.pages.map(registryEntriesArray =>
            registryEntriesArray.map(registryEntry => (
              <RegistryEntry
                key={registryEntry.name}
                registryEntry={registryEntry}
              />
            ))
          )}
        </InfiniteScroll>
        {registryEntries.pages.length === 1 && registryEntries.pages[0].length === 0 &&
          <Typography color="textSecondary" align="center" variant="h6" sx={{ marginTop: '40px' }}>{t('noRegistryEntries')}</Typography>}
      </Box>
    </>
  );
};
