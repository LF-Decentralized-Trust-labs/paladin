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

import { useContext, useState } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { useQuery } from "@tanstack/react-query";
import LocationOnOutlinedIcon from '@mui/icons-material/LocationOnOutlined';
import { Alert, Box, Button, Fade, Typography, useTheme } from "@mui/material";
import { getAltModeScrollBarStyle } from "../themes/default";
import { TransportPeer } from "../components/TransportPeer";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { fetchTransportNodeName, fetchTransportPeers } from "../queries/transport";
import { MyNodeDialog } from "../dialogs/MyNode";
import { useTranslation } from "react-i18next";

export const Nodes: React.FC = () => {

  const { lastBlockWithTransactions, autoRefreshEnabled } = useContext(ApplicationContext);
  const theme = useTheme();
  const [MyNodeDialogOpen, setMyNodeDialogOpen] = useState(false);
  const { t } = useTranslation();

  const { data: transportNodeName, error: errorFetchingTransportNodeName, isFetching: isFetchingTransportNodeName } = useQuery({
    queryKey: ["transportNodeName"],
    queryFn: () => fetchTransportNodeName()
  });

  const { data: transportPeers, error: errorFetchingTransportPeers, isFetching: isFetchingTransportPeers } = useQuery({
    queryKey: ["transportPeers", autoRefreshEnabled, lastBlockWithTransactions],
    queryFn: () => fetchTransportPeers()
  });

  if (isFetchingTransportNodeName || isFetchingTransportPeers) {
    return <></>;
  }

  if (errorFetchingTransportNodeName) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{errorFetchingTransportNodeName.message}</Alert>
  }

  if (errorFetchingTransportPeers) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{errorFetchingTransportPeers.message}</Alert>
  }

  return (
    <>
      <Fade timeout={600} in={true}>
        <Box
          sx={{
            padding: "20px",
            maxWidth: "1300px",
            marginLeft: "auto",
            marginRight: "auto",
            position: 'relative'
          }}
        >
          <Typography align="center" variant="h5" sx={{ marginBottom: '20px' }}>
            {t("activePeers")}
          </Typography>
          <Button
            size="large"
            variant="outlined"
            startIcon={<LocationOnOutlinedIcon />}
            sx={{ position: 'absolute', right: '46px', top: '23px', borderRadius: '20px' }}
            onClick={() => setMyNodeDialogOpen(true)}
          >
            {transportNodeName}
          </Button>
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
                <InfoOutlinedIcon sx={{ fontSize: '50px' }} />
                <Typography>{t('noActivePeers')}</Typography>
              </Box>
            }
          </Box>
        </Box>
      </Fade>
      {transportNodeName !== undefined &&
        <MyNodeDialog
          nodeName={transportNodeName}
          dialogOpen={MyNodeDialogOpen}
          setDialogOpen={setMyNodeDialogOpen}
        />}
    </>
  );

};
