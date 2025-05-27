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

import {
  Alert,
  Box,
  Fade,
  Grid2,
  MenuItem,
  TextField,
  Typography
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { t } from 'i18next';
import { Captions } from 'lucide-react';
import { useEffect, useState } from 'react';
import { DomainDeploy } from '../components/DomainDeploy';
import { Hash } from '../components/Hash';
import { SmartContractsTable } from '../components/SmartContractsTable';
import { getDomainByName, listDomains } from '../queries/domains';

export const Domains: React.FC = () => {
  const [selectedDomain, setSelectedDomain] = useState<string>();

  const {
    data: domains,
    error,
    isFetching,
  } = useQuery({
    queryKey: ['domains'],
    queryFn: () => listDomains(),
  });

  const { data: domain } = useQuery({
    queryKey: ['domain', selectedDomain],
    queryFn: () => getDomainByName(selectedDomain ?? ''),
    enabled: !!selectedDomain,
  });

  useEffect(() => {
    if (selectedDomain === undefined && domains?.length) {
      setSelectedDomain(domains[0]);
    }
  }, [selectedDomain, domains]);

  if (isFetching) {
    return <></>;
  }

  if (error) {
    return (
      <Alert sx={{ margin: '30px' }} severity="error" variant="filled">
        {error.message}
      </Alert>
    );
  }

  return (
    <>
      <Fade timeout={600} in={true}>
        <Box
          sx={{
            padding: '20px',
            maxWidth: '1300px',
            marginLeft: 'auto',
            marginRight: 'auto',
          }}
        >
          <Grid2 container alignItems="center" spacing={2}>
            <Grid2 size={{ md: 4 }}>
              <TextField
                size="small"
                color="secondary"
                slotProps={{
                  input: {
                    sx: {
                      color: (theme) => theme.palette.text.secondary,
                      borderRadius: '30px',
                    },
                  },
                }}
                select
                value={selectedDomain ?? ''}
                onChange={(event) => setSelectedDomain(event.target.value)}
              >
                {domains?.map((domain) => (
                  <MenuItem key={domain} value={domain}>
                    {domain}
                  </MenuItem>
                ))}
              </TextField>
            </Grid2>
            <Grid2 size={{ xs: 12, md: 4 }}>
              <Typography align="center" variant="h5">
                {t('domainDetails')}
              </Typography>
            </Grid2>
            <Grid2
              size={{ xs: 12, md: 4 }}
              container
              justifyContent={{ xs: 'center', sm: 'center', md: 'right' }}
            >
              <DomainDeploy domainName={selectedDomain ?? ''} />
            </Grid2>
          </Grid2>
          <Box sx={{ height: '10px' }} />
          <Box
            sx={{
              backgroundColor: (theme) => theme.palette.background.paper,
              marginBottom: '20px',
              borderRadius: '4px',
              padding: '20px',
            }}
          >
            <Grid2
              container
              justifyContent="space-between"
              alignItems="center"
              spacing={2}
            >
              <Grid2 size={{ md: 4 }}>
                <Typography align="center" variant="h6" color="textPrimary">
                  {selectedDomain ?? '--'}
                </Typography>
                <Typography
                  align="center"
                  variant="body2"
                  color="textSecondary"
                >
                  {t('domain')}
                </Typography>
              </Grid2>
              <Grid2 size={{ md: 4 }} textAlign="center">
                <Hash
                  Icon={<Captions size="18px" />}
                  hash={domain?.registryAddress ?? '--'}
                  title={t('registry')}
                />
              </Grid2>
            </Grid2>
          </Box>
          <Grid2>
            <Typography align="center" variant="h5">
              {t('smartContracts')}
            </Typography>
          </Grid2>
          <Box sx={{ height: '10px' }} />
          {domain?.registryAddress && (
            <SmartContractsTable domainAddress={domain.registryAddress} />
          )}
        </Box>
      </Fade>
    </>
  );
};
