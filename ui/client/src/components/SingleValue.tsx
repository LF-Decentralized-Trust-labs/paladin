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

import { Box, Typography, Tooltip, IconButton } from "@mui/material";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useState } from "react";
import { useTranslation } from "react-i18next";

type Props = {
  label: string
  value: string
}

export const SingleValue: React.FC<Props> = ({ label, value }) => {

  const [copyLabel, setCopyLabel] = useState('copyToClipboard');
  const { t } = useTranslation();

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', border: theme => `solid 1px ${theme.palette.divider}`, borderRadius: '4px' }}>
      <Box sx={{ borderRight: theme => `solid 1px ${theme.palette.divider}`, padding: '10px', minWidth: '160px' }}>
        <Typography color="textSecondary">{label}</Typography>
      </Box>
      <Typography sx={{ marginLeft: '10px', flexGrow: 1 }} color="textPrimary">{value}</Typography>
      <Tooltip title={t(copyLabel)} arrow placement="bottom" onMouseLeave={() => setTimeout(() => setCopyLabel('copyToClipboard'), 200)}>
        <IconButton onClick={() => { navigator.clipboard.writeText(value); setCopyLabel('copied') }}>
          <ContentCopyIcon />
        </IconButton>
      </Tooltip>
    </Box>
  );
}