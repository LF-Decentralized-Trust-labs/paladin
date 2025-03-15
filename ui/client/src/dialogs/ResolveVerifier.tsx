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
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  Radio,
  RadioGroup,
  TextField,
  Tooltip,
  Typography
} from '@mui/material';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useQuery } from '@tanstack/react-query';
import { resolveVerifier } from '../queries/states';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { constants } from '../components/config';

type Props = {
  dialogOpen: boolean
  setDialogOpen: Dispatch<SetStateAction<boolean>>
}

export const ResolveVerifierDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen
}) => {

  const [keyIdentifier, setKeyIdentifier] = useState('');
  const [isLocalNode, setIsLocalNode] = useState(true);
  const [remoteNodeName, setRemoteNodeName] = useState('');
  const [algorithm, setAlgorithm] = useState(constants.KEY_ETHEREUM_ALGORITHM);
  const [verifierType, setVerifierType] = useState(constants.KEY_ETHEREUM_TYPE);
  const [isError, setIsError] = useState(false);
  const [result, setResult] = useState<string>();
  const [copyLabel, setCopyLabel] = useState('copyToClipboard');
  const { t } = useTranslation();

  const { refetch } = useQuery({
    queryKey: ["reverseKeyLookup"],
    queryFn: () => resolveVerifier(isLocalNode ? keyIdentifier : `${keyIdentifier}@${remoteNodeName}`, algorithm, verifierType),
    enabled: false,
    refetchOnMount: false,
    retry: false
  });

  useEffect(() => {
    if (!dialogOpen) {
      setTimeout(() => {
        setKeyIdentifier('');
        setIsLocalNode(true);
        setRemoteNodeName('');
        setAlgorithm(constants.KEY_ETHEREUM_ALGORITHM);
        setVerifierType(constants.KEY_ETHEREUM_TYPE);
        setIsError(false);
        setResult(undefined);
      }, 200);
    }
  }, [dialogOpen]);

  const handleSubmit = () => {
    refetch().then(result => {
      setIsError(result.status === 'error');
      setResult(result.data);
    });
  };

  const canSubmit = keyIdentifier.length > 0 && (isLocalNode || remoteNodeName.length > 0);

  return (
    <Dialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      fullWidth
      maxWidth="sm"
    >
      <form onSubmit={(event) => {
        event.preventDefault();
        handleSubmit();
      }}>
        <DialogTitle sx={{ textAlign: 'center' }}>
          {t('resolveVerifier')}
          {isError &&
            <Alert variant="filled" severity="error">{t('failedToResolveVerifier')}</Alert>
          }
          {!isError && result !== undefined &&
            <Alert variant="filled" severity="success" sx={{ alignItems: 'center' }} action={
              <Tooltip title={t(copyLabel)} arrow placement="bottom" onMouseLeave={() => setTimeout(() => setCopyLabel('copyToClipboard'), 200)}>
              <IconButton size="small" onClick={() => { navigator.clipboard.writeText(result); setCopyLabel('copied') }}>
                <ContentCopyIcon />
              </IconButton>
            </Tooltip>
            }>{result}</Alert>
          }
        </DialogTitle>
        <DialogContent>
          <Box sx={{ marginTop: '5px' }}>
            <TextField
              fullWidth
              label={t('keyIdentifier')}
              autoComplete="off"
              value={keyIdentifier}
              onChange={event => setKeyIdentifier(event.target.value)}
            />
          </Box>
          <RadioGroup
            sx={{ marginTop: '10px' }}
            value={isLocalNode ? 'local' : 'remote'}
            onChange={event => setIsLocalNode(event.target.value === 'local')}
          >
            <Box>
              <Box>
                <FormControlLabel value="local" control={<Radio />} label={t('localNode')} />
              </Box>
              <Box sx={{ display: 'flex' }}>
                <FormControlLabel sx={{ whiteSpace: 'nowrap' }} value="remote" control={<Radio />} label={t('remoteNode')} />
                <TextField
                  fullWidth
                  disabled={isLocalNode}
                  label={t('nodeName')}
                  autoComplete="off"
                  value={remoteNodeName}
                  onChange={event => setRemoteNodeName(event.target.value)}
                />
              </Box>
            </Box>
          </RadioGroup>
          <Accordion elevation={1} sx={{ marginTop: '20px' }}>
            <AccordionSummary
              expandIcon={<ExpandMoreIcon />}
              aria-controls="panel1-content"
              id="panel1-header"
            >
              <Typography component="span">{t('options')}</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <TextField
                fullWidth
                label={t('algorithm')}
                autoComplete="off"
                value={algorithm}
                onChange={event => setAlgorithm(event.target.value)}
              />
              <TextField
                sx={{ marginTop: '20px' }}
                fullWidth
                label={t('verifierType')}
                autoComplete="off"
                value={verifierType}
                onChange={event => setVerifierType(event.target.value)}
              />
            </AccordionDetails>
          </Accordion>
        </DialogContent>
        <DialogActions sx={{ justifyContent: 'center', paddingBottom: '20px' }}>
          <Button
            sx={{ minWidth: '100px' }}
            size="large"
            variant="contained"
            disableElevation
            disabled={!canSubmit}
            type="submit">
            {t('resolve')}
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
