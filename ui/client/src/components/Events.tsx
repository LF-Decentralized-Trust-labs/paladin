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
import { fetchEvents } from "../queries/events";
import { Event } from "./Event";
import { useContext } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { getAltModeScrollBarStyle } from "../themes/default";
import InfiniteScroll from "react-infinite-scroll-component";
import { IEvent } from "../interfaces";
import { useTranslation } from "react-i18next";

export const Events: React.FC = () => {

  const { lastBlockWithTransactions } = useContext(ApplicationContext);

  const { data: events, fetchNextPage, hasNextPage, error } = useInfiniteQuery({
    queryKey: ["events", lastBlockWithTransactions],
    queryFn: ({ pageParam }) => fetchEvents(pageParam),
    initialPageParam: undefined as IEvent | undefined,
    getNextPageParam: (lastPage) => { return lastPage.length > 0? lastPage[lastPage.length - 1] : undefined },
  });

  const theme = useTheme();
  const { t } = useTranslation();

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  if (events?.pages === undefined) {
    return <></>;
  }

  return (
    <>
      <Typography align="center" variant="h5" sx={{ marginBottom: '20px' }}>
        {t("events")}
      </Typography>
      <Box
        id="scrollableDivEvents"
        sx={{
          height: "calc(100vh - 170px)",
          paddingRight: "15px",
          ...getAltModeScrollBarStyle(theme.palette.mode)
        }}
      >
        <InfiniteScroll
          scrollableTarget="scrollableDivEvents"
          dataLength={events.pages.length}
          next={() => fetchNextPage()}
          hasMore={hasNextPage}
          loader={<LinearProgress />}
        >
          {
            events.pages.map(eventArray => eventArray.map(
              (event) => (
                <Event key={`${event.blockNumber}-${event.logIndex}`} event={event} />
              )
            ))
          }
        </InfiniteScroll>
      </Box>
    </>
  );
};
