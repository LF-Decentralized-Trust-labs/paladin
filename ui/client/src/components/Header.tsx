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

import { AppBar, Box, Button, Grid2, IconButton, Tab, Tabs, ToggleButton, ToggleButtonGroup, Toolbar, Tooltip, useMediaQuery, useTheme } from "@mui/material";
import { useContext, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import Brightness4Icon from '@mui/icons-material/Brightness4';
import { ApplicationContext } from "../contexts/ApplicationContext";
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import PauseIcon from '@mui/icons-material/Pause';
import RefreshIcon from '@mui/icons-material/Refresh';
import UploadIcon from '@mui/icons-material/Upload';
import { ABIUploadDialog } from "../dialogs/ABIUpload";

export const Header: React.FC = () => {

  const { colorMode, autoRefreshEnabled, setAutoRefreshEnabled, refreshRequired, refresh } = useContext(ApplicationContext);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const pathname = useLocation().pathname.toLowerCase();
  const theme = useTheme();
  const lessThanMedium = useMediaQuery(theme.breakpoints.down("md"));
  const [abiUploadDialogOpen, setAbiUploadDialogOpen] = useState(false);

  const getTabFromPath = (path: string) => {
    if (path.startsWith('/ui/indexer')) {
      return 0;
    } else if (path.startsWith('/ui/submissions')) {
      return 1;
    } else if (path.startsWith('/ui/registry')) {
      return 2;
    }
    return 0;
  };

  const [tab, setTab] = useState(getTabFromPath(pathname));

  const handleNavigation = (tab: number) => {
    setTab(tab);
    switch (tab) {
      case 0: navigate('/ui/indexer'); break;
      case 1: navigate('/ui/submissions'); break;
      case 2: navigate('/ui/registry'); break;
    }
  };

  const handleAutoRefreshChange = (value: 'play' | 'pause') => {
    switch (value) {
      case 'play': setAutoRefreshEnabled(true); break;
      case 'pause': setAutoRefreshEnabled(false); break;
    }
  };

  return (
    <>
      <AppBar>
        <Toolbar sx={{ backgroundColor: theme => theme.palette.background.paper }}>
          <Box sx={{ width: '100%', maxWidth: '1270px', marginLeft: 'auto', marginRight: 'auto' }}>
            <Grid2 container alignItems="center">
              <Grid2 size={{ xs: 12, sm: 12, md: 4 }} textAlign={lessThanMedium ? 'center' : 'left'}>
                <img src={theme.palette.mode === 'dark' ?
                  '/ui/paladin-title-dark.svg' : '/ui/paladin-title-light.svg'
                } style={{ marginTop: '7px' }} />
              </Grid2>
              <Grid2 size={{ xs: 12, sm: 12, md: 4 }} alignContent="center">
                <Tabs
                  TabIndicatorProps={{ style: { height: '4px' } }}
                  value={tab} onChange={(_event, value) => handleNavigation(value)} centered>
                  <Tab sx={{ textTransform: 'none' }} label={t('indexer')} />
                  <Tab sx={{ textTransform: 'none' }} label={t('submissions')} />
                  <Tab sx={{ textTransform: 'none' }} label={t('registry')} />
                </Tabs>
              </Grid2>
              <Grid2 size={{ xs: 12, sm: 12, md: 4 }}>
                <Grid2 container justifyContent={lessThanMedium ? 'center' : 'right'} spacing={1} alignItems="center"
                  sx={{ padding: lessThanMedium ? '20px' : undefined }}>
                  {refreshRequired &&
                    <Grid2>
                      <Button size="small" startIcon={<RefreshIcon />} variant="outlined" sx={{ textTransform: 'none', borderRadius: '20px' }}
                        onClick={() => refresh()}>
                        {t('newData')}
                      </Button>
                    </Grid2>}
                  <Grid2>
                    <ToggleButtonGroup exclusive onChange={(_event, value) => handleAutoRefreshChange(value)} value={autoRefreshEnabled ? 'play' : 'pause'}>
                      <Tooltip arrow title={t('autoRefreshOn')}
                        slotProps={{ popper: { modifiers: [{ name: 'offset', options: { offset: [0, -6] }, }] } }}
                      >
                        <ToggleButton color="primary" value="play">
                          <PlayArrowIcon fontSize="small" />
                        </ToggleButton>
                      </Tooltip>
                      <Tooltip arrow title={t('autoRefreshOff')}
                        slotProps={{ popper: { modifiers: [{ name: 'offset', options: { offset: [0, -6] }, }] } }}
                      >
                        <ToggleButton color="primary" value="pause">
                          <PauseIcon fontSize="small" />
                        </ToggleButton>
                      </Tooltip>
                    </ToggleButtonGroup>
                  </Grid2>
                  <Grid2>
                    <Tooltip arrow title={t('uploadABI')}
                      slotProps={{ popper: { modifiers: [{ name: 'offset', options: { offset: [0, -4] }, }] } }}
                    >
                      <IconButton onClick={() => setAbiUploadDialogOpen(true)}>
                        <UploadIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip arrow title={t('switchThemeMode')}
                      slotProps={{ popper: { modifiers: [{ name: 'offset', options: { offset: [0, -4] }, }] } }}
                    >
                      <IconButton onClick={() => colorMode.toggleColorMode()}>
                        <Brightness4Icon />
                      </IconButton>
                    </Tooltip>
                  </Grid2>
                </Grid2>
              </Grid2>
            </Grid2>
          </Box>
        </Toolbar>
      </AppBar>
      <Box sx={{
        height: theme => lessThanMedium ? '190px' :
          theme.mixins.toolbar
      }} />
    <ABIUploadDialog
      dialogOpen={abiUploadDialogOpen}
      setDialogOpen={setAbiUploadDialogOpen}
    />
    </>
  );

};