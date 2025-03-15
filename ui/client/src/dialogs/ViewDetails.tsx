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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  useTheme} from '@mui/material';
import { useTranslation } from 'react-i18next';
import JSONPretty from 'react-json-pretty';

type Props = {
  title: string
  details: Object
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const ViewDetailsDialog: React.FC<Props> = ({
  title,
  details,
  dialogOpen,
  setDialogOpen
}) => {

  const { t } = useTranslation();
  const theme = useTheme();

  const colors = theme.palette.mode === 'dark'?
  {
    main: 'line-height:1.3;color:#white;overflow:auto;',
    key: 'color:white;',
    string: 'color:#20dfdf;',
    value: 'color:#20dfdf;',
    boolean: 'color:#20dfdf;'
  }:
  {
    main: 'line-height:1.3;color:#107070;overflow:auto;',
    key: 'color:#464646;',
    string: 'color:#107070;',
    value: 'color:#107070;',
    boolean: 'color:#107070;'
  };

  return (
    <Dialog
      onClose={() => setDialogOpen(false)}
      open={dialogOpen}
      maxWidth="lg"
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {title}
      </DialogTitle>
      <DialogContent>
        <JSONPretty style={{ fontSize: '12px'}}  data={details} theme={colors} />
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
