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

import { Box, Grid2, Typography } from "@mui/material";
import { ITransportPeer } from "../interfaces";
import byteSize from "pretty-bytes";
import UploadIcon from '@mui/icons-material/Upload';
import DownloadIcon from '@mui/icons-material/Download';
import { EllapsedTime } from "./EllapsedTime";
import { useTranslation } from "react-i18next";


type Props = {
  transportPeer: ITransportPeer
}

export const TransportPeer: React.FC<Props> = ({ transportPeer }) => {

  const { t } = useTranslation();

  return (
    <Box
      sx={{
        backgroundColor: (theme) => theme.palette.background.paper,
        marginBottom: "20px",
        borderRadius: "4px"
      }}
    >
      <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px', borderBottom: theme => `solid 1px ${theme.palette.divider}` }}>
        <Grid2>
          <Typography align="center" sx={{ fontWeight: 500 }} variant="h5" color="textPrimary">{transportPeer.name}</Typography>
          <Typography align="center" variant="body2" color="textSecondary">{t('transportSubtitle', { transport: transportPeer.outboundTransport, detail: transportPeer.outbound.endpoint })}</Typography>
        </Grid2>
      </Box>
      <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px', borderBottom: theme => `solid 1px ${theme.palette.divider}` }}>
        <Grid2 container justifyContent="space-evenly" spacing={5}>

          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{transportPeer.stats.sentMsgs.toLocaleString()}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('messagesSent')}</Typography>
          </Grid2>

          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{transportPeer.stats.receivedMsgs.toLocaleString()}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('messagesReceived')}</Typography>
          </Grid2>

          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{byteSize(transportPeer.stats.sentBytes)}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('dataSent')}</Typography>
          </Grid2>


          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{byteSize(transportPeer.stats.receivedBytes)}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('dataReceived')}</Typography>
          </Grid2>


          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{transportPeer.stats.reliableHighestSent.toLocaleString()}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('reliableHighestSent')}</Typography>
          </Grid2>


          <Grid2>
            <Typography align="center" variant="h6" color="textPrimary">{transportPeer.stats.reliableAckBase.toLocaleString()}</Typography>
            <Typography align="center" variant="body2" color="textSecondary">{t('reliableHighestAck')}</Typography>
          </Grid2>

        </Grid2>
      </Box>
      <Box sx={{ padding: '10px' }}>
        <Box sx={{ justifyContent: 'right', display: 'flex', gap: '50px' }}>
          <EllapsedTime
            timestamp={transportPeer.activated}
            prefix={t('activated')}
          />

          <EllapsedTime
            timestamp={transportPeer.stats.lastSend}
            icon={<UploadIcon />}
            prefix={t('lastSend')}
          />


          <EllapsedTime
            timestamp={transportPeer.stats.lastReceive}
            icon={<DownloadIcon />}

            prefix={t('lastReceive')}
          />

        </Box>



      </Box>


    </Box>
  );

};