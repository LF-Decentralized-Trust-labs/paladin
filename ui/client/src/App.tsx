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

import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  createBrowserRouter,
  Navigate,
  RouterProvider,
} from "react-router-dom";
import { Config } from "./config";
import { ApplicationContextProvider } from "./contexts/ApplicationContext";
import { ThemeContextProvider } from "./contexts/ThemeContext";
import { AppLinks } from "./navigation/AppLinks";
import AppRoot from "./navigation/AppRoot";
import { IndexersRoute } from "./pages/Indexers/Routes";
import { QueryBuilderRoute } from "./pages/Queries/Routes";
import { RegistriesRoute } from "./pages/Registries/Routes";
import { SubmissionsRoute } from "./pages/Submissions/Routes";

const queryClient = new QueryClient({
  queryCache: new QueryCache({}),
  mutationCache: new MutationCache({}),
});

function App() {
  const router = createBrowserRouter([
    {
      path: "/",
      element: <AppRoot />,
      children: [
        IndexersRoute,
        RegistriesRoute,
        SubmissionsRoute,
        QueryBuilderRoute,
        {
          path: "*",
          element: <Navigate to={AppLinks.Indexers} />,
        },
      ],
    },
  ]);

  return (
    <>
      <QueryClientProvider client={queryClient}>
        <ThemeContextProvider
          defaultTheme="light"
          storageKey={Config.COLOR_MODE_STORAGE_KEY}
        >
          <ApplicationContextProvider>
            <RouterProvider {...{ router }} />
          </ApplicationContextProvider>
        </ThemeContextProvider>
      </QueryClientProvider>
    </>
  );
}

export default App;
