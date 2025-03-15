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
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
  useTheme
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { IPaladinTransaction } from '../interfaces';
import { PaladinTransactionsDetails } from '../components/TransactionDetails';
import { useEffect, useState } from 'react';
import { getAltModeScrollBarStyle } from '../themes/default';

type Props = {
  paladinTransactions?: IPaladinTransaction[]
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const PaladinTransactionsDetailsDialog: React.FC<Props> = ({
  paladinTransactions,
  dialogOpen,
  setDialogOpen
}) => {

  const [selectedPaladinTransactionId, setSelectedPaladinTransactionId] = useState('');
  const { t } = useTranslation();

  const theme = useTheme();

  const selectedTransaction = paladinTransactions?.find(r => (r.id == selectedPaladinTransactionId));

  useEffect(() => {
    if (dialogOpen) {
      setSelectedPaladinTransactionId((paladinTransactions && paladinTransactions.length > 0) ? paladinTransactions[0].id : '');
    }
  }, [dialogOpen]);

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      fullWidth
      maxWidth="xl"
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {t('transaction')}
      </DialogTitle>
      <DialogContent sx={{ height: '80vh',  padding: '20px', ...getAltModeScrollBarStyle(theme.palette.mode)}}>
        <Box sx={{ padding: '20px', paddingTop: '5px' }}>
        <TextField select={paladinTransactions && paladinTransactions.length > 1} label={t('id')} fullWidth size="small" value={selectedPaladinTransactionId}
          onChange={event => setSelectedPaladinTransactionId(event.target.value)}>
          {paladinTransactions?.map(paladinTransaction =>
            <MenuItem key={paladinTransaction.id} value={paladinTransaction.id}>{paladinTransaction.id}</MenuItem>
          )}
        </TextField>
        </Box>
        { selectedTransaction ?
          <PaladinTransactionsDetails
            paladinTransaction={selectedTransaction}
          />
          : undefined
        }
      </DialogContent>
      <DialogActions sx={{ justifyContent: 'center', marginBottom: '15px' }}>
        <Button
          onClick={() => setDialogOpen(false)}
          variant="contained"
          disableElevation>
          {t('close')}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
