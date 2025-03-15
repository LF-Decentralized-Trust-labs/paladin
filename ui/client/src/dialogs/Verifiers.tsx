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
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { IVerifier } from '../interfaces';
import { SingleValue } from '../components/SingleValue';

type Props = {
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
  verifiers: IVerifier[]
}

export const VerifiersDialog: React.FC<Props> = ({
  dialogOpen,
  setDialogOpen,
  verifiers
}) => {

  const { t } = useTranslation();

  return (
    <Dialog
      fullWidth
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      maxWidth="md"
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {t('verifiers')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', gap: '30px', flexDirection: 'column' }}>
          {verifiers.map(verifier =>
            <Box key={verifier.verifier} sx={{ display: 'flex', gap: '10px', flexDirection: 'column' }}>
              <SingleValue label={t('type')} value={verifier.type} />
              <SingleValue label={t('algorithm')} value={verifier.algorithm} />
              <SingleValue label={t('verifier')} value={verifier.verifier} />
            </Box>)}
        </Box>
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
