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

import { Box, Fade, Grid2, useMediaQuery, useTheme } from "@mui/material";
import { Transactions } from "../components/Transactions";
import { Events } from "../components/Events";

export const Activity: React.FC = () => {

  const theme = useTheme();
  const displayLine = useMediaQuery(theme.breakpoints.up('md'));

  return (
    <Fade timeout={600} in={true}>
      <Box sx={{ padding: '10px', paddingTop: '30px', maxWidth: '1300px', marginLeft: 'auto', marginRight: 'auto' }}>
        <Grid2 container spacing={8}>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }} alignSelf="center">
            <Transactions />
          </Grid2>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }}>
            <Events />
          </Grid2>
        </Grid2>
        {displayLine &&
          <Box sx={{
            left: 'calc(50% - 1px)',
            top: 0,
            position: 'fixed',
            height: '100vh',
            width: '1px',
            backgroundColor: theme => theme.palette.primary.main
          }} />}
      </Box>
    </Fade>
  );
};
