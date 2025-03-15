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
  Alert,
  Dialog,
  DialogContent,
  DialogTitle} from '@mui/material';
import { useTranslation } from 'react-i18next';

type Props = {
  message: string
  dialogOpen: boolean
}

export const ErrorDialog: React.FC<Props> = ({
  message,
  dialogOpen
}) => {

  const { t } = useTranslation();

  return (
    <Dialog
      open={dialogOpen}
      maxWidth="lg"
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {t('errorConnectingToPaladinNode')}
      </DialogTitle>
      <DialogContent>
          <Alert variant="filled" severity="error">
            {message}
          </Alert>
      </DialogContent>
    </Dialog>
  );
};
