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

import { PaletteMode, ThemeOptions } from '@mui/material';

const commonThemeOptions: ThemeOptions = {
  components: {
    MuiTextField: {
      defaultProps: {
        slotProps: {
          inputLabel: {
            shrink: true
          }
        }
      }
    },
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none'
        }
      }
    },
    MuiTab: {
      styleOverrides: {
        root: {
          textTransform: 'none'
        }
      }
    },
    MuiToggleButton: {
      styleOverrides: {
        root: {
          textTransform: 'none'
        }
      }
    }
  }
};

export const darkThemeOptions: ThemeOptions = {
  ...commonThemeOptions,
  palette: {
    mode: 'dark',
    primary: {
      main: '#20dfdf',
      dark: '#6D6D6D'
    },
    secondary: {
      main: '#bbbbbb',
    },
    background: {
      default: 'black',
      paper: '#1E242B',
    }
  },
  components: {
    ...commonThemeOptions.components,
    MuiAlert: {
      styleOverrides: {
        filledSuccess: {
          color: 'white'
        },
        filledError: {
          color: 'white'
        }
      }
    }
  }
};

export const lightThemeOptions: ThemeOptions = {
  ...commonThemeOptions,
  palette: {
    mode: 'light',
    primary: {
      main: '#107070',
      dark: '#6D6D6D'
    },
    secondary: {
      main: '#BBEDED'
    },
    background: {
      default: '#F0F0F0',
      paper: '#FFFFFF',
    }
  }
};

export const altDarkModeScrollbarStyle = {
  overflow: 'overlay',
  scrollbarColor: '#1e242a',
  '&::-webkit-scrollbar, & *::-webkit-scrollbar': {
    backgroundColor: 'rgba(60, 60, 60, .40)',
  },
  '&::-webkit-scrollbar-thumb, & *::-webkit-scrollbar-thumb': {
    borderRadius: 8,
    backgroundColor: '#12171d',
    border: '3px solid #1e242a',
  },
};

export const altLightModeScrollbarStyle = {
  overflow: 'overlay',
  scrollbarColor: '#FFFFFF',
  '&::-webkit-scrollbar, & *::-webkit-scrollbar': {
    backgroundColor: 'rgba(255, 255, 255, .40)',
  },
  '&::-webkit-scrollbar-thumb, & *::-webkit-scrollbar-thumb': {
    borderRadius: 8,
    backgroundColor: '#F0F0F0',
    border: '3px solid #FFFFFF',
  },
};

export const getAltModeScrollBarStyle = (paletteMode: PaletteMode) => (
  paletteMode === 'light' ? altLightModeScrollbarStyle : altDarkModeScrollbarStyle
);
