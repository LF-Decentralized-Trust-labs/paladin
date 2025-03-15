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

import { Box, Button, Grid2, Menu, ToggleButton, ToggleButtonGroup, Typography, useTheme } from "@mui/material";
import { useContext, useState } from "react";
import { ApplicationContext } from "../contexts/ApplicationContext";
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import PauseIcon from '@mui/icons-material/Pause';
import LightModeIcon from '@mui/icons-material/LightMode';
import DarkModeIcon from '@mui/icons-material/DarkMode';
import { ABIUploadDialog } from "../dialogs/ABIUpload";
import { useTranslation } from "react-i18next";

export type Props = {
  anchorEl: HTMLElement | null;
  setAnchorEl: React.Dispatch<React.SetStateAction<HTMLElement | null>>
}

export const SettingsMenu: React.FC<Props> = ({
  anchorEl,
  setAnchorEl
}) => {

  const { colorMode, autoRefreshEnabled, setAutoRefreshEnabled } = useContext(ApplicationContext);
  const [abiUploadDialogOpen, setAbiUploadDialogOpen] = useState(false);

  const theme = useTheme();
  const { t } = useTranslation();

  const handleAutoRefreshChange = (value: 'play' | 'pause' | null) => {
    switch (value) {
      case 'play': setAutoRefreshEnabled(true); break;
      case 'pause': setAutoRefreshEnabled(false); break;
    }
  };

  const handleColorChange = (mode: 'light' | 'dark' | null) => {
    if (mode !== null && mode !== theme.palette.mode) {
      colorMode.toggleColorMode();
    }
  };

  return (
    <>
      <Menu
        anchorEl={anchorEl}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        transformOrigin={{ vertical: 'top', horizontal: 'center' }}
        open={anchorEl !== null}
        keepMounted
        onClose={() => setAnchorEl(null)}
      >
        <Grid2 container>
          <Grid2>
            <Box sx={{ borderBottom: `solid 1px ${theme.palette.divider}`, padding: '8px 12px 8px 20px', display: 'flex', alignItems: 'center' }}>
              <Typography sx={{ minWidth: '150px', whiteSpace: 'nowrap', marginRight: '8px' }}>{t('autoRefreshValue', { value: t(autoRefreshEnabled ? 'on' : 'off') })}</Typography>
              <ToggleButtonGroup exclusive onChange={(_event, value) => handleAutoRefreshChange(value)} value={autoRefreshEnabled ? 'play' : 'pause'}>
                <ToggleButton color="primary" value="play">
                  <PlayArrowIcon fontSize="small" />
                </ToggleButton>
                <ToggleButton color="primary" value="pause">
                  <PauseIcon fontSize="small" />
                </ToggleButton>
              </ToggleButtonGroup>
            </Box>
            <Box sx={{ borderBottom: `solid 1px ${theme.palette.divider}`, padding: '8px 12px 8px 20px', display: 'flex', alignItems: 'center' }}>
              <Typography sx={{ minWidth: '150px', whiteSpace: 'nowrap', marginRight: '8px' }}>{t('themeValue', { value: t(theme.palette.mode === 'light' ? 'light' : 'dark') })}</Typography>
              <ToggleButtonGroup exclusive onChange={(_event, value) => handleColorChange(value)} value={theme.palette.mode}>
                <ToggleButton color="primary" value="light">
                  <LightModeIcon fontSize="small" />
                </ToggleButton>
                <ToggleButton color="primary" value="dark">
                  <DarkModeIcon fontSize="small" />
                </ToggleButton>
              </ToggleButtonGroup>
            </Box>
            <Box sx={{ padding: '8px 12px 8px 20px', display: 'flex', alignItems: 'center' }}>
              <Typography sx={{ minWidth: '150px', whiteSpace: 'nowrap', marginRight: '8px' }}>{t('contractAbis')}</Typography>
              <Button
                sx={{ minHeight: '40px', minWidth: '87px' }}
                variant="outlined"
                onClick={() => { setAnchorEl(null); setAbiUploadDialogOpen(true) }}
              >{t('upload')}</Button>
            </Box>
          </Grid2>
        </Grid2>
      </Menu>
      <ABIUploadDialog
        dialogOpen={abiUploadDialogOpen}
        setDialogOpen={setAbiUploadDialogOpen}
      />
    </>
  );

};