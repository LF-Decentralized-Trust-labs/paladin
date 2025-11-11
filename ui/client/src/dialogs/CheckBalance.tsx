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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  TextField,
  Tooltip,
  Typography,
  Grid2,
} from '@mui/material';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { callBalanceOf, BalanceOfResult } from '../queries/balance';

type Props = {
  dialogOpen: boolean;
  setDialogOpen: Dispatch<SetStateAction<boolean>>;
  domain: string;
  contractAddress: string;
};

export const CheckBalanceDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
  domain,
  contractAddress,
}) => {
  const [account, setAccount] = useState('');
  const [from, setFrom] = useState('');
  const [isError, setIsError] = useState(false);
  const [result, setResult] = useState<BalanceOfResult>();
  const [copyLabel, setCopyLabel] = useState('copyToClipboard');
  const { t } = useTranslation();

  const { refetch } = useQuery({
    queryKey: ['balanceOf', domain, contractAddress, account, from],
    queryFn: () => callBalanceOf(domain, contractAddress, from, { account }),
    enabled: false,
    refetchOnMount: false,
    retry: false,
  });

  useEffect(() => {
    if (!dialogOpen) {
      setTimeout(() => {
        setAccount('');
        setFrom('');
        setIsError(false);
        setResult(undefined);
      }, 200);
    }
  }, [dialogOpen]);

  const handleSubmit = () => {
    refetch().then((result) => {
      setIsError(result.status === 'error');
      setResult(result.data);
    });
  };

  const canSubmit = account.length > 0 && from.length > 0;

  const formatBalance = (hexValue: string) => {
    try {
      return BigInt(hexValue).toString();
    } catch {
      return hexValue;
    }
  };

  return (
    <Dialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      fullWidth
      maxWidth="sm"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          handleSubmit();
        }}
      >
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('checkBalance')}
          {isError && (
            <Alert variant="filled" severity="error">
              {t('failedToCheckBalance')}
            </Alert>
          )}
          {!isError && result !== undefined && (
            <Alert
              variant="filled"
              severity="success"
              sx={{ alignItems: 'center', marginTop: '10px' }}
            >
              <Box>
                <Grid2 container spacing={2} sx={{ textAlign: 'left' }}>
                  <Grid2 size={4}>
                    <Typography variant="body2">
                      <strong>{t('totalBalance')}:</strong>
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}
                    >
                      {formatBalance(result.totalBalance)}
                    </Typography>
                  </Grid2>
                  <Grid2 size={4}>
                    <Typography variant="body2">
                      <strong>{t('totalStates')}:</strong>
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}
                    >
                      {formatBalance(result.totalStates)}
                    </Typography>
                  </Grid2>
                  <Grid2 size={4}>
                    <Typography variant="body2">
                      <strong>{t('overflow')}:</strong>
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}
                    >
                      {result.overflow ? t('true') : t('false')}
                    </Typography>
                  </Grid2>
                </Grid2>
              </Box>
            </Alert>
          )}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <TextField
              fullWidth
              label={t('from')}
              autoComplete="off"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
              sx={{ marginBottom: '20px' }}
            />
          </Box>
          <Box sx={{ marginTop: '5px' }}>
            <TextField
              fullWidth
              label={t('account')}
              autoComplete="off"
              value={account}
              onChange={(event) => setAccount(event.target.value)}
            />
          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit"
          >
            {t('checkBalance')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t('close')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
