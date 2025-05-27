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

import { CssBaseline } from "@mui/material";
import { createTheme, PaletteMode, ThemeProvider } from "@mui/material/styles";
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { constants } from "./components/config";
import { Header } from "./components/Header";
import { ApplicationContextProvider } from "./contexts/ApplicationContext";
import { AppRoutes } from "./routes";
import { darkThemeOptions, lightThemeOptions } from "./themes/default";
import { getBasePath } from "./utils";
import { Activity } from "./views/Activity";
import { Domains } from "./views/Domains";
import { Keys } from "./views/Keys";
import { Nodes } from "./views/Peers";
import { Registries } from "./views/Registries";
import { Submissions } from "./views/Submissions";

const queryClient = new QueryClient({
  queryCache: new QueryCache({}),
  mutationCache: new MutationCache({}),
});

function App() {

  const [systemTheme, setSystemTheme] = useState(
    window.matchMedia &&
      window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  );

  const [storedTheme, setStoredTheme] = useState<PaletteMode>();

  useEffect(() => {
    window
      .matchMedia('(prefers-color-scheme: dark)')
      .addEventListener('change', (event) => {
        setSystemTheme(event.matches ? 'dark' : 'light');
      });
  }, []);


  const theme = useMemo(() => {
    const modeFromStorage = localStorage.getItem(constants.COLOR_MODE_STORAGE_KEY);
    if (modeFromStorage === null) {
      // If color mode not previously set
      return createTheme(
        systemTheme === 'dark' ? darkThemeOptions : lightThemeOptions
      );
    } else {
      // Create color mode based on local storage
      return createTheme(
        modeFromStorage === 'dark' ? darkThemeOptions : lightThemeOptions
      );
    }
  }, [systemTheme, storedTheme]);

  const colorMode = useMemo(
    () => ({
      toggleColorMode: () => {
        const currentMode =
          localStorage.getItem(constants.COLOR_MODE_STORAGE_KEY) ?? systemTheme;
        const newMode = currentMode === 'light' ? 'dark' : 'light';
        localStorage.setItem(constants.COLOR_MODE_STORAGE_KEY, newMode);
        setStoredTheme(newMode);
      },
    }),
    []
  );

  const basePath = getBasePath();

  return (
    <>
      <QueryClientProvider client={queryClient}>
        <ApplicationContextProvider colorMode={colorMode}>
          <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter basename={basePath}>
              <Header />
              <Routes>
                <Route path={AppRoutes.Activity} element={<Activity />} />
                <Route path={AppRoutes.Submissions} element={<Submissions />} />
                <Route path={AppRoutes.Peers} element={<Nodes />} />
                <Route path={AppRoutes.Keys} element={<Keys />} />
                <Route path={AppRoutes.Registry} element={<Registries />} />
                <Route path={AppRoutes.Domains} element={<Domains />} />
                <Route path="*" element={<Navigate to={AppRoutes.Activity} replace />} />
              </Routes>
            </BrowserRouter>
          </ThemeProvider>
        </ApplicationContextProvider>
      </QueryClientProvider>
    </>
  );
}

export default App;
