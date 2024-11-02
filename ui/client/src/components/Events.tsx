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

import { Box, Typography, useTheme } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { t } from "i18next";
import { fetchEvents } from "../queries/events";
import { Event } from "./Event";
import { useContext } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { altDarkModeScrollbarStyle, altLightModeScrollbarStyle } from "../themes/default";

export const Events: React.FC = () => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  const { data: events } = useQuery({
    queryKey: ["events", lastBlockWithTransactions],
    queryFn: () => fetchEvents(),
  });

  const theme = useTheme();
  const addedStyle = theme.palette.mode === 'light'? altLightModeScrollbarStyle : altDarkModeScrollbarStyle;

  return (
    <>
      <Typography align="center" variant="h5" sx={{ marginBottom: '20px' }}>
        {t("events")}
      </Typography>
      <Box
        sx={{
          height: "calc(100vh - 170px)",
          paddingRight: "15px",
          ...addedStyle
        }}
      >
        {events?.map((event) => (
          <Event key={`${event.blockNumber}-${event.logIndex}`} event={event} />
        ))}
      </Box>
    </>
  );
};
