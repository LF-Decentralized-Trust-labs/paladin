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

import { Alert, Box, Button, Fade, Grid2, MenuItem, TextField, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useContext, useEffect, useState } from "react";
import { Registry } from "../components/Registry";
import { ApplicationContext } from "../contexts/ApplicationContext";
import { fetchRegistries } from "../queries/registry";
import { useTranslation } from "react-i18next";
import PersonSearchIcon from '@mui/icons-material/PersonSearch';
import { ResolveVerifierDialog } from "../dialogs/ResolveVerifier";

export const Registries: React.FC = () => {

  const { lastBlockWithTransactions, autoRefreshEnabled } = useContext(ApplicationContext);
  const [resolveVerifierDialogOpen, setResolveVerifierDialogOpen] = useState(false);
  const [selectedRegistry, setSelectedRegistry] = useState<string>();
  const { t } = useTranslation();

  const { data: registries, error, isFetching } = useQuery({
    queryKey: ["registries", autoRefreshEnabled, lastBlockWithTransactions],
    queryFn: () => fetchRegistries()
  });

  useEffect(() => {
    if (registries?.length === 1) {
      setSelectedRegistry(registries[0]);
    }
  }, [registries]);

  if (isFetching) {
    return <></>;
  }

  if (error) {
    return <Alert sx={{ margin: '30px' }} severity="error" variant="filled">{error.message}</Alert>
  }

  return (
    <>
      <Fade timeout={600} in={true}>
        <Box
          sx={{
            padding: "20px",
            maxWidth: "1300px",
            marginLeft: "auto",
            marginRight: "auto",
          }}
        >
          <Grid2 container alignItems="center" spacing={2}>
            <Grid2 sx={{ display: { xs: 'none', sm: 'none', md: 'block' } }} size={{ md: 4 }} >
              <TextField
                size="small"
                color="secondary"
                slotProps={{ input: { sx: { color: theme => theme.palette.text.secondary, borderRadius: '30px' } } }}
                select
                value={selectedRegistry ?? ''}
                onChange={event => setSelectedRegistry(event.target.value)}
              >
                {registries?.map(registry =>
                  <MenuItem key={registry} value={registry}>{registry}</MenuItem>
                )}
              </TextField>
            </Grid2>
            <Grid2 size={{ xs: 12, md: 4 }}>
              <Typography align="center" variant="h5">
                {t("entries")}
              </Typography>
            </Grid2>
            <Grid2 size={{ xs: 12, md: 4 }} container justifyContent={{ xs: 'center', sm: 'center', md: 'right' }}>
              <Grid2>
                <Button
                  size="large"
                  variant="outlined"
                  startIcon={<PersonSearchIcon />}
                  sx={{ borderRadius: '20px', marginRight: '14px' }}
                  onClick={() => setResolveVerifierDialogOpen(true)}
                >
                  {t('resolveVerifier')}
                </Button>
              </Grid2>
            </Grid2>
          </Grid2>
          {selectedRegistry !== undefined &&
            <Registry registryName={selectedRegistry} />
          }
        </Box>
      </Fade>
      <ResolveVerifierDialog
        dialogOpen={resolveVerifierDialogOpen}
        setDialogOpen={setResolveVerifierDialogOpen}
      />
    </>
  );
};
