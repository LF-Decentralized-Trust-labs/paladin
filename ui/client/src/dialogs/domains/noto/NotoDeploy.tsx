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
  TextField,
} from '@mui/material';
import { useMutation } from '@tanstack/react-query';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TransactionType } from '../../../interfaces';
import { sendTransaction } from '../../../queries/transactions';

const notoConstructorABI = {
  type: 'constructor',
  inputs: [
    { name: 'notary', type: 'string' },
    { name: 'notaryMode', type: 'string' },
  ],
};

interface NotoConstructorParams {
  notary: string;
  notaryMode: 'basic' | 'hooks';
}

type Props = {
  dialogOpen: boolean;
  setDialogOpen: Dispatch<SetStateAction<boolean>>;
  domain: string;
};

export const NotoDeployDialog: React.FC<Props> = ({
  domain,
  dialogOpen,
  setDialogOpen,
}) => {
  const { t } = useTranslation();
  const [sender, setSender] = useState<string>('');
  const [form, setForm] = useState<NotoConstructorParams>({
    notary: '',
    notaryMode: 'basic',
  });
  const [errorMessage, setErrorMessage] = useState<string>();

  const { mutate, error } = useMutation({
    mutationFn: () =>
      sendTransaction({
        type: TransactionType.PRIVATE,
        from: sender,
        domain,
        abi: [notoConstructorABI],
        data: form,
      }),
    onSuccess: () => setDialogOpen(false),
  });

  useEffect(() => {
    if (error !== null) {
      setErrorMessage(t('mintFailed'));
    }
  }, [error]);

  const canSubmit = sender.length > 0 && form.notary.length > 0;

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
          mutate();
        }}
      >
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('deployNew')}
          <Box sx={{ marginTop: '10px' }}>
            {errorMessage !== undefined && (
              <Alert variant="filled" severity="error">
                {errorMessage}
              </Alert>
            )}
          </Box>
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <TextField
              fullWidth
              disabled
              label={t('domain')}
              autoComplete="off"
              value={domain}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('deployer')}
              autoComplete="off"
              value={sender}
              onChange={(event) => setSender(event.target.value)}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('notary')}
              autoComplete="off"
              value={form.notary}
              onChange={(event) =>
                setForm({ ...form, notary: event.target.value })
              }
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('notaryMode')}
              autoComplete="off"
              disabled
              value={form.notaryMode}
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
            {t('deploy')}
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
