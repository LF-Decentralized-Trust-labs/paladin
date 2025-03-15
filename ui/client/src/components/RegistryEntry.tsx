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

import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import { Box, Button, Collapse, Grid2, TextField, Typography, useTheme } from "@mui/material";
import { Hash } from "./Hash";
import { useState } from "react";
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import { IRegistryEntry } from "../interfaces";
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import { ViewDetailsDialog } from "../dialogs/ViewDetails";
import { Captions, Tag } from 'lucide-react';
import { useTranslation } from "react-i18next";

type Props = {
  registryEntry: IRegistryEntry;
};

export const RegistryEntry: React.FC<Props> = ({ registryEntry }) => {

  const [isExpanded, setIsExpanded] = useState(false);
  const [viewDetailsDialogOpen, setViewDetailsDialogOpen] = useState(false);
  const theme = useTheme();
  const { t } = useTranslation();

  const processValue = (value: string) => {
    try {
      const parsedValue = JSON.parse(value);
      return JSON.stringify(parsedValue, null, 8);
    } catch (err) {
      console.error(err);
    }
    return value;
  };

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
            <Grid2 size={{ xs: 4 }}>
              <Hash Icon={<Tag size="18px" />} title={t("id")} hash={registryEntry.id} />
            </Grid2>
            <Grid2 size={{ xs: 4 }}>
              <Hash
                Icon={<Captions size="18px" />}
                title={t("owner")}
                hash={registryEntry.properties.$owner}
              />
            </Grid2>
            <Grid2 size={{ xs: 2 }}>
              <Typography align="center" variant="h6" color="textPrimary">
                {registryEntry.name}
              </Typography>
              <Typography
                align="center"
                variant="body2"
                color="textSecondary"
              >
                {t("name")}
              </Typography>
            </Grid2>
            <Grid2 sx={{ textAlign: "center" }} alignContent="center" size={{ xs: 2 }}>
              {registryEntry.active !== false ? (
                <CheckCircleOutlineIcon color="primary" />
              ) : (
                <ErrorOutlineIcon color="error" />
              )}
              <Typography
                align="center"
                variant="body2"
                color="textSecondary"
              >
                {t("active")}
              </Typography>
            </Grid2>
          </Grid2>
        </Box>
        <Box sx={{ padding: '10px'}}>
        <Box sx={{ display: 'flex', justifyContent: 'right' }}>
          <Button size="small" startIcon={<VisibilityIcon />} sx={{ marginRight: '40px', fontWeight: '400' }}
            onClick={() => setViewDetailsDialogOpen(true)}>{t('viewDetails')}</Button>
          <Button sx={{ fontWeight: '400', minWidth: '140px' }} size="small" endIcon={isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            onClick={() => setIsExpanded(!isExpanded)}>
            {t(isExpanded ? 'hideProperties' : 'showProperties')}
          </Button>
        </Box>
        <Collapse in={isExpanded}>

          {Object.keys(registryEntry.properties)
            .filter((property) => property !== "$owner")
            .map((property) => (
              <TextField
                key={property}
                label={property}
                maxRows={8}
                multiline
                fullWidth
                size="small"
                sx={{ marginTop: '10px' }}
                slotProps={{ htmlInput: { style: { fontSize: '12px', color: `${theme.palette.text.secondary}`}  }}}
                value={processValue(registryEntry.properties[property])}
              />
            ))}

        </Collapse>
        </Box>
      </Box>
      <ViewDetailsDialog
        title={t('registryEntry')}
        details={registryEntry}
        dialogOpen={viewDetailsDialogOpen}
        setDialogOpen={setViewDetailsDialogOpen}
      />
    </>
  );
};
