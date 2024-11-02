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

import { Box, Button, Grid2, Typography } from "@mui/material";
import { IEvent } from "../interfaces";
import { t } from "i18next";
import { Hash } from "./Hash";
import { EllapsedTime } from "./EllapsedTime";
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import { useState } from "react";
import { ViewDetailsDialog } from "../dialogs/ViewDetails";

type Props = {
  event: IEvent
}

export const Event: React.FC<Props> = ({ event }) => {

  const [viewDetailsDialogOpen, setViewDetailsDialogOpen] = useState(false);

  return (
    <>
      <Box sx={{
        backgroundColor: theme => theme.palette.background.paper,
        marginBottom: '20px', borderRadius: '4px'
      }}>
        <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px', borderBottom: theme => `solid 1px ${theme.palette.divider}` }}>
          <Grid2 container direction="column" spacing={2}>
            <Grid2 container justifyContent="space-between">
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{event.blockNumber.toLocaleString()}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('block')}</Typography>
              </Grid2>
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{event.transactionIndex}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('transactionIndex')}</Typography>
              </Grid2>
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{event.logIndex}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('logIndex')}</Typography>
              </Grid2>
            </Grid2>
          </Grid2>
        </Box>
        <Box sx={{ padding: '10px', paddingBottom: '20px' }}>
          <Grid2 container justifyContent="space-evenly" spacing={2}>
            <Grid2 size={{ xs: 6 }}>
              <Hash title={t('transactionHash')} hash={event.transactionHash} />
            </Grid2>
            <Grid2 size={{ xs: 6 }}>
              <Hash title={t('signature')} hash={event.signature} />
            </Grid2>
          </Grid2>
        </Box>
        <Box sx={{ display: 'flex', padding: '10px', justifyContent: 'end' }}>
          <EllapsedTime timestamp={event.block.timestamp} />
          <Button sx={{ marginLeft: '20px', textTransform: 'none', fontWeight: '400' }} size="small" startIcon={<VisibilityIcon />}
            onClick={() => setViewDetailsDialogOpen(true)}>{t('viewDetails')}</Button>
        </Box>
      </Box>
      <ViewDetailsDialog
        title={t('event')}
        details={event}
        dialogOpen={viewDetailsDialogOpen}
        setDialogOpen={setViewDetailsDialogOpen}
      />
    </>
  );

};