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

import { Box, Fade, Grid2, Paper } from "@mui/material";
import { Transactions } from "../components/Transactions";
import { Events } from "../components/Events";

export const Indexer: React.FC = () => {

  return (
    <Fade timeout={800} in={true}>
      <Box sx={{ padding: '20px', maxWidth: '1200px', marginLeft: 'auto', marginRight: 'auto' }}>
        <Grid2 container spacing={2}>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }}>
            <Paper sx={{ padding: '10px', paddingTop: '12px',
              backgroundColor: theme => theme.palette.mode === 'light' ?
               'rgba(255, 255, 255, .65)' : 'rgba(60, 60, 60, .65)' }}>
              <Transactions />
            </Paper>
          </Grid2>
          <Grid2 size={{ md: 6, sm: 12, xs: 12 }}>
            <Paper sx={{ padding: '10px', paddingTop: '12px', 
              backgroundColor: theme => theme.palette.mode === 'light' ?
              'rgba(255, 255, 255, .65)' : 'rgba(60, 60, 60, .65)' }}>
              <Events />
            </Paper>
          </Grid2>
        </Grid2>
      </Box>
    </Fade>
  );
};
