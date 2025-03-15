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
  FormControlLabel,
  Radio,
  RadioGroup,
  TextField
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { reverseKeyLookup } from '../queries/keys';
import { constants } from '../components/config';
import { useTranslation } from 'react-i18next';
import { IFilter } from '../interfaces';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
  mode: 'explorer' | 'list'
  setParent: Dispatch<SetStateAction<string>>
  setFilters: Dispatch<SetStateAction<IFilter[]>>
}

export const ReverseKeyLookupDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
  mode,
  setParent,
  setFilters
}) => {

  const [verifier, setVerifier] = useState('');
  const [isEthereum, setIsEthereum] = useState(true);
  const [type, setType] = useState('');
  const [otherType, setOtherType] = useState('');
  const [otherAlgorithm, setOtherAlgorithm] = useState('');
  const [algorithm, setAlgorithm] = useState('');
  const [notFound, setNotFound] = useState(false);
  const { t } = useTranslation();

  const { refetch } = useQuery({
    queryKey: ["reverseKeyLookup"],
    queryFn: () => reverseKeyLookup(algorithm, type, verifier),
    enabled: false,
    refetchOnMount: false,
    retry: false
  });

  useEffect(() => {
    if (!dialogOpen) {
      setTimeout(() => {
        setVerifier('');
        setIsEthereum(true);
        setOtherType('');
        setOtherAlgorithm('');
        setNotFound(false);
      }, 200);
    }
  }, [dialogOpen]);

  useEffect(() => {
    setType(isEthereum ? constants.KEY_ETHEREUM_TYPE : otherType);
    setAlgorithm(isEthereum ? constants.KEY_ETHEREUM_ALGORITHM : otherAlgorithm);
  }, [isEthereum, otherType, otherAlgorithm]);

  const handleSubmit = () => {
    refetch().then(result => {
      if (result.status === 'success') {
        const path = result.data.path.map(segment => segment.name).join('.');
        const index = path.lastIndexOf('.');
        if(index !== -1 && mode === 'explorer') {
          setParent(path.substring(0, index));
        }
        setFilters([{
          field: {
            label: t('path'),
            name: 'path',
            type: 'string'
          },
          operator: 'equal',
          value: path
        }]);
        setDialogOpen(false);
      } else if (result.status === 'error') {
        setNotFound(true);
      }
    });
  };

  const canSubmit = verifier.length > 0 && (isEthereum ||
    (otherType.length > 0 && otherAlgorithm.length > 0));

  return (
    <Dialog
      disableRestoreFocus
      fullWidth
      open={dialogOpen}
      maxWidth="sm"
      onClose={() => setDialogOpen(false)}
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('reverseLookup')}
          {notFound &&
            <Alert variant="filled" severity="warning">{t('verifierNotFound')}</Alert>}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <TextField
              autoFocus
              autoComplete="off"
              fullWidth
              label={t('address')}
              value={verifier}
              onChange={event => setVerifier(event.target.value)} />

            <RadioGroup
              sx={{ marginTop: '10px' }}
              value={isEthereum ? 'ethereum' : 'other'}
              onChange={event => setIsEthereum(event.target.value === 'ethereum')}
            >
              <FormControlLabel value="ethereum" control={<Radio />} label={t('ethereum')} />
              <FormControlLabel value="other" control={<Radio />} label={t('other')} />
            </RadioGroup>
            <Box sx={{ marginTop: '15px', marginLeft: '30px' }}>
              <TextField
                autoComplete="off"
                fullWidth
                disabled={isEthereum}
                label={t('type')}
                value={otherType}
                onChange={event => setOtherType(event.target.value)}>
              </TextField>
              <TextField
                sx={{ marginTop: '20px' }}
                autoComplete="off"
                fullWidth
                disabled={isEthereum}
                label={t('algorithm')}
                value={otherAlgorithm}
                onChange={event => setOtherAlgorithm(event.target.value)}>
              </TextField>
            </Box>

          </Box>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('lookup')}
          </Button>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="outlined"
            disableElevation
            onClick={() => setDialogOpen(false)}
          >
            {t('cancel')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
