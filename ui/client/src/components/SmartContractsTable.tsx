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
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  useTheme,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { querySmartContractsByDomain } from '../queries/domains';
import { getAltModeScrollBarStyle } from '../themes/default';
import { DomainButtons } from './DomainButtons';
import { Hash } from './Hash';

type Props = {
  domainAddress: string;
};

export const SmartContractsTable: React.FC<Props> = ({ domainAddress }) => {
  const { t } = useTranslation();
  const theme = useTheme();

  const {
    data: contracts,
    error,
    isFetching,
  } = useQuery({
    queryKey: ['contracts', domainAddress],
    queryFn: () => querySmartContractsByDomain(domainAddress),
  });

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
    <TableContainer
      component={Paper}
      sx={{
        height: 'calc(100vh - 320px)',
        ...getAltModeScrollBarStyle(theme.palette.mode),
      }}
    >
      <Table stickyHeader>
        <TableHead>
          <TableRow>
            <TableCell
              sx={{
                backgroundColor: (theme) => theme.palette.background.paper,
              }}
            >
              {t('contractAddress')}
            </TableCell>
            <TableCell
              sx={{
                backgroundColor: (theme) => theme.palette.background.paper,
              }}
            >
              {t('actions')}
            </TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {contracts?.map((contract: any) => (
            <TableRow key={contract.address} sx={{ height: '70px' }}>
              <TableCell>
                <Hash title={t('address')} hash={contract.address} />
              </TableCell>
              <TableCell>
                <DomainButtons
                  domainName={contract.domainName}
                  contractAddress={contract.address}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
