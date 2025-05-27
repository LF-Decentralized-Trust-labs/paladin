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
import { encodeHex } from '../../../utils';

type Props = {
  contractAddress: string;
  dialogOpen: boolean;
  setDialogOpen: Dispatch<SetStateAction<boolean>>;
};

const mintAbi = {
  inputs: [
    {
      components: [
        {
          internalType: 'string',
          name: 'to',
          type: 'string',
        },
        {
          internalType: 'uint256',
          name: 'amount',
          type: 'uint256',
        },
        {
          internalType: 'bytes',
          name: 'data',
          type: 'bytes',
        },
      ],
      internalType: 'struct IZetoFungible.TransferParam[]',
      name: 'mints',
      type: 'tuple[]',
    },
  ],
  name: 'mint',
  outputs: [],
  stateMutability: 'nonpayable',
  type: 'function',
};

export const ZetoMintDialog: React.FC<Props> = ({
  contractAddress,
  dialogOpen,
  setDialogOpen,
}) => {
  const { t } = useTranslation();
  const [sender, setSender] = useState('');
  const [recipient, setRecipient] = useState('');
  const [amount, setAmount] = useState('');
  const [data, setData] = useState('');
  const [errorMessage, setErrorMessage] = useState<string>();

  useEffect(() => {
    if (!dialogOpen) {
      setTimeout(() => {
        setRecipient('');
        setAmount('');
        setData('');
      }, 200);
    }
  }, [dialogOpen]);

  const { mutate, error } = useMutation({
    mutationFn: () =>
      sendTransaction({
        type: TransactionType.PRIVATE,
        from: sender,
        to: contractAddress,
        abi: [mintAbi],
        function: 'mint',
        data: {
          mints: [
            {
              to: recipient,
              amount,
              data: encodeHex(data),
            },
          ],
        },
      }),
    onSuccess: () => setDialogOpen(false),
  });

  useEffect(() => {
    if (error !== null) {
      setErrorMessage(t('mintFailed'));
    }
  }, [error]);

  const canSubmit =
    recipient.length > 0 && amount.length > 0 && !isNaN(parseInt(amount));

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
          {t('mint')}
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
              label={t('contractAddress')}
              autoComplete="off"
              value={contractAddress}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('from')}
              autoComplete="off"
              value={sender}
              onChange={(event) => setSender(event.target.value)}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('to')}
              autoComplete="off"
              value={recipient}
              onChange={(event) => setRecipient(event.target.value)}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('amount')}
              autoComplete="off"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
          </Box>
          <Box sx={{ marginTop: '20px' }}>
            <TextField
              fullWidth
              label={t('data')}
              autoComplete="off"
              value={data}
              onChange={(event) => setData(event.target.value)}
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
            {t('mint')}
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
