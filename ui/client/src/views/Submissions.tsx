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

import { Box, Fade, ToggleButton, ToggleButtonGroup, Typography, useTheme } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { t } from "i18next";
import { useContext, useState } from "react";
import { PaladinTransaction } from "../components/PaladinTransaction";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { fetchSubmissions } from "../queries/transactions";
import { altLightModeScrollbarStyle, altDarkModeScrollbarStyle } from "../themes/default";

export const Submissions: React.FC = () => {
  const { lastBlockWithTransactions } = useContext(ApplicationContext);
  const [tab, setTab] = useState<'all' | 'pending'>('all');

  const theme = useTheme();
  const addedStyle = theme.palette.mode === 'light' ? altLightModeScrollbarStyle : altDarkModeScrollbarStyle;

  const { data: transactions } = useQuery({
    queryKey: ["pendingTransactions", tab, lastBlockWithTransactions],
    queryFn: () => fetchSubmissions(tab),
    retry: false
  });

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
        <Box sx={{ marginBottom: '20px', textAlign: 'right' }}>
          <ToggleButtonGroup exclusive onChange={(_event, value) => setTab(value)} value={tab}>
            <ToggleButton color="primary" value="all" sx={{ textTransform: 'none', width: '130px', height: '45px' }}>{t('all')}</ToggleButton>
            <ToggleButton color="primary" value="pending" sx={{ textTransform: 'none', width: '130px', height: '45px' }}>{t('pending')}</ToggleButton>
          </ToggleButtonGroup>
        </Box>
        <Box
          sx={{
            paddingRight: "15px",
            height: "calc(100vh - 178px)",
            ...addedStyle
          }}
        >
          {transactions?.map(transaction => (
            <PaladinTransaction
              key={transaction.id}
              paladinTransaction={transaction}
            />
          ))}
          {transactions?.length === 0 &&
            <Typography color="textSecondary" align="center" variant="h6" sx={{ marginTop: '40px' }}>{t('noPendingTransactions')}</Typography>}
        </Box>

      </Box>
    </Fade>
  );
};
