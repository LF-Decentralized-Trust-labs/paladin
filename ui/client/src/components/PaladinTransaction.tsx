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

import { Box, Button, Collapse, Grid2, TextField, Typography, useTheme } from "@mui/material";
import { useState } from "react";
import { IPaladinTransaction } from "../interfaces";
import { Hash } from "./Hash";
import daysjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import { EllapsedTime } from "./EllapsedTime";
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import { PaladinTransactionsDetailsDialog } from "../dialogs/TransactionDetails";
import { Captions, Tag } from 'lucide-react';
import { formatJSONWhenApplicable } from "../utils";
import { useTranslation } from "react-i18next";

daysjs.extend(relativeTime);

type Props = {
  paladinTransaction: IPaladinTransaction;
};

export const PaladinTransaction: React.FC<Props> = ({ paladinTransaction }) => {

  const [viewDetailsDialogOpen, setViewDetailsDialogOpen] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const theme = useTheme();
  const { t } = useTranslation();

  if (paladinTransaction === undefined) {
    return <></>;
  }

  return (
    <>
      <Box
        sx={{
          backgroundColor: (theme) => theme.palette.background.paper,
          marginBottom: "20px",
          borderRadius: "4px"
        }}
      >
        <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px', borderBottom: theme => `solid 1px ${theme.palette.divider}` }}>
          <Grid2 container justifyContent="space-between" alignItems="center" spacing={2}>
            <Grid2 textAlign="center" size={{ md: 2.5 }}>
              <Hash Icon={<Tag size="18px" />} title={t("id")} hash={paladinTransaction.id} />
            </Grid2>
            <Grid2 textAlign="center" size={{ md: 2.5 }}>
              <Hash Icon={<Captions size="18px" />} hash={paladinTransaction.from} title={t('from')} />
            </Grid2>
            <Grid2 textAlign="center" size={{ md: 2.5 }}>
              <Hash Icon={<Captions size="18px" />} hash={paladinTransaction.to ?? '--'} title={t('to')} />
            </Grid2>
            <Grid2 size={{ md: 2.25 }}>
              <Typography align="center" variant="h6">
                {t(paladinTransaction.type)}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("type")}
              </Typography>
            </Grid2>
            <Grid2 size={{ md: 2.25 }}>
              <Typography align="center" variant="h6" color="textPrimary">
                {paladinTransaction.domain ?? '--'}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("domain")}
              </Typography>
            </Grid2>
          </Grid2>
        </Box>
        <Box sx={{ padding: '10px' }}>
          <Grid2 container justifyContent="space-between" spacing={2}>
            <Grid2>
              <EllapsedTime timestamp={paladinTransaction?.created} />
            </Grid2>
            <Grid2 container spacing={3} size="grow" justifyContent="end">
              <Grid2>
                <Button size="small" startIcon={<VisibilityIcon />} sx={{ minWidth: '120px', fontWeight: '400' }}
                  onClick={() => setViewDetailsDialogOpen(true)}>{t('viewDetails')}</Button>
              </Grid2>
              <Grid2>
                <Button size="small" endIcon={isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                  onClick={() => setIsExpanded(!isExpanded)} sx={{ minWidth: '140px', fontWeight: '400' }}>
                  {t(isExpanded ? 'hideProperties' : 'showProperties')}
                </Button>
              </Grid2>
            </Grid2>
          </Grid2>
          <Collapse in={isExpanded}>
            {Object.keys(paladinTransaction.data)
              .filter((property) => property !== "$owner")
              .map((property) => (
                <TextField
                  key={property}
                  label={t(property)}
                  maxRows={8}
                  multiline
                  fullWidth
                  size="small"
                  sx={{ marginTop: '12px' }}
                  slotProps={{ htmlInput: { style: { fontSize: '12px', color: `${theme.palette.text.secondary}` } } }}
                  value={formatJSONWhenApplicable(paladinTransaction.data[property])}
                />
              ))}
            {Object.keys(paladinTransaction.data).length === 0 &&
              <Typography align="center">{t('noProperties')}</Typography>}
          </Collapse>
        </Box>
      </Box>
      <PaladinTransactionsDetailsDialog
        paladinTransactions={[paladinTransaction]}
        dialogOpen={viewDetailsDialogOpen}
        setDialogOpen={setViewDetailsDialogOpen}
      />
    </>
  );
};
