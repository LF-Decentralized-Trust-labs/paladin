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

import { useContext } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { useQuery } from "@tanstack/react-query";
import { fetchTransportNodes } from "../queries/nodes";
import { Alert, Box, Fade, Typography, useTheme } from "@mui/material";
import { getAltModeScrollBarStyle } from "../themes/default";
import { TransportPeer } from "../components/TransportPeer";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { t } from "i18next";

export const Nodes: React.FC = () => {

  const { lastBlockWithTransactions, autoRefreshEnabled } = useContext(ApplicationContext);
  const theme = useTheme();

  const { data: transportPeers, error, isFetching } = useQuery({
    queryKey: ["registries", autoRefreshEnabled, lastBlockWithTransactions],
    queryFn: () => fetchTransportNodes()
  });

  if (isFetching) {
    return <></>;
  }

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  return (
    <Fade timeout={600} in={true}>
      <Box
        sx={{
          padding: "30px",
          maxWidth: "1300px",
          marginLeft: "auto",
          marginRight: "auto",
        }}
      >
        <Typography align="center" variant="h5" sx={{ marginBottom: '20px' }}>
          {t("activePeers")}
        </Typography>
        <Box
          sx={{
            paddingRight: '15px',
            height: "calc(100vh - 170px)",
            ...getAltModeScrollBarStyle(theme.palette.mode)
          }}
        >
          {transportPeers?.map(transportPeer => (
            <TransportPeer key={transportPeer.name} transportPeer={transportPeer} />
          ))}
          {transportPeers?.length === 0 &&
            <Box sx={{ marginTop: '60px', textAlign: 'center', color: theme => theme.palette.text.secondary }}>
              <InfoOutlinedIcon sx={{ fontSize: '50px'}} />
              <Typography>{t('noActivePeers')}</Typography>
            </Box>
          }
        </Box>
      </Box>
    </Fade>
  );

};
