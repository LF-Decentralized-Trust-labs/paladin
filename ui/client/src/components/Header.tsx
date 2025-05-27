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

import MenuIcon from '@mui/icons-material/Menu';
import RefreshIcon from '@mui/icons-material/Refresh';
import {
  AppBar,
  Box,
  Button,
  Grid2,
  IconButton,
  Tab,
  Tabs,
  Toolbar,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import { useContext, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import logoDark from '../../public/paladin-title-dark.svg';
import logoLight from '../../public/paladin-title-light.svg';
import { ApplicationContext } from '../contexts/ApplicationContext';
import { SettingsMenu } from '../menus/Settings';
import { AppRoutes } from '../routes';

enum HeaderTab {
  Activity = 0,
  Submissions = 1,
  Peers = 2,
  Keys = 3,
  Registry = 4,
  Domains = 5,
}

export const Header: React.FC = () => {
  const { refreshRequired, refresh } = useContext(ApplicationContext);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const pathname = useLocation().pathname.toLowerCase();
  const theme = useTheme();
  const lessThanMedium = useMediaQuery(theme.breakpoints.down('md'));
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  const getTabFromPath = (path: string) => {
    if (path.startsWith(AppRoutes.Activity)) {
      return HeaderTab.Activity;
    } else if (path.startsWith(AppRoutes.Submissions)) {
      return HeaderTab.Submissions;
    } else if (path.startsWith(AppRoutes.Peers)) {
      return HeaderTab.Peers;
    } else if (path.startsWith(AppRoutes.Keys)) {
      return HeaderTab.Keys;
    } else if (path.startsWith(AppRoutes.Registry)) {
      return HeaderTab.Registry;
    } else if (path.startsWith(AppRoutes.Domains)) {
      return HeaderTab.Domains;
    }
    return HeaderTab.Activity;
  };

  const [tab, setTab] = useState(getTabFromPath(pathname));

  const handleNavigation = (tab: number) => {
    setTab(tab);
    switch (tab) {
      case HeaderTab.Activity:
        navigate(AppRoutes.Activity);
        break;
      case HeaderTab.Submissions:
        navigate(AppRoutes.Submissions);
        break;
      case HeaderTab.Peers:
        navigate(AppRoutes.Peers);
        break;
      case HeaderTab.Keys:
        navigate(AppRoutes.Keys);
        break;
      case HeaderTab.Registry:
        navigate(AppRoutes.Registry);
        break;
      case HeaderTab.Domains:
        navigate(AppRoutes.Domains);
        break;
    }
  };

  return (
    <>
      <AppBar>
        <Toolbar
          sx={{ backgroundColor: (theme) => theme.palette.background.paper }}
        >
          <Box
            sx={{
              width: '100%',
              maxWidth: '1270px',
              marginLeft: 'auto',
              marginRight: 'auto',
            }}
          >
            <Grid2 container alignItems="center">
              <Grid2
                size={{ xs: 12, sm: 12, md: 3 }}
                textAlign={lessThanMedium ? 'center' : 'left'}
              >
                <img
                  src={theme.palette.mode === 'dark' ? logoDark : logoLight}
                  style={{ marginTop: '7px' }}
                />
              </Grid2>
              <Grid2 size={{ xs: 12, sm: 12, md: 6 }} alignContent="center">
                <Tabs
                  TabIndicatorProps={{ style: { height: '4px' } }}
                  value={tab}
                  onChange={(_event, value) => handleNavigation(value)}
                  centered
                >
                  <Tab label={t('activity')} />
                  <Tab label={t('submissions')} />
                  <Tab label={t('peers')} />
                  <Tab label={t('keys')} />
                  <Tab label={t('registry')} />
                  <Tab label={t('domains')} />
                </Tabs>
              </Grid2>
              <Grid2 size={{ xs: 12, sm: 12, md: 3 }}>
                <Grid2
                  container
                  justifyContent={lessThanMedium ? 'center' : 'right'}
                  spacing={1}
                  alignItems="center"
                  sx={{ padding: lessThanMedium ? '20px' : undefined }}
                >
                  {refreshRequired && (
                    <Grid2>
                      <Button
                        size="small"
                        startIcon={<RefreshIcon />}
                        variant="outlined"
                        sx={{ borderRadius: '20px' }}
                        onClick={() => refresh()}
                      >
                        {t('newData')}
                      </Button>
                    </Grid2>
                  )}
                  <Grid2>
                    <IconButton
                      onClick={(event) => setAnchorEl(event.currentTarget)}
                    >
                      <MenuIcon />
                    </IconButton>
                  </Grid2>
                </Grid2>
              </Grid2>
            </Grid2>
          </Box>
        </Toolbar>
      </AppBar>
      <Box
        sx={{
          height: (theme) => (lessThanMedium ? '190px' : theme.mixins.toolbar),
        }}
      />
      <SettingsMenu anchorEl={anchorEl} setAnchorEl={setAnchorEl} />
    </>
  );
};
