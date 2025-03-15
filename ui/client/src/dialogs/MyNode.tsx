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
  MenuItem,
  TextField,
  Typography
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { fetchTransportLocalDetails } from '../queries/transport';
import { JSONBox } from '../components/JSONBox';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';

type Props = {
  nodeName: string
  dialogOpen: boolean
  setDialogOpen: React.Dispatch<React.SetStateAction<boolean>>
}

export const MyNodeDialog: React.FC<Props> = ({
  nodeName,
  dialogOpen,
  setDialogOpen
}) => {

  const { t } = useTranslation();

  const { data: transportLocalDetails, error, isLoading } = useQuery({
    enabled: dialogOpen,
    queryKey: ["transportLocalDetails"],
    queryFn: () => fetchTransportLocalDetails('grpc')
  });

  return (
    <Dialog
      open={dialogOpen}
      maxWidth="sm"
      fullWidth
      onClose={() => setDialogOpen(false)}
    >
      <DialogTitle sx={{ textAlign: 'center' }}>
        {nodeName}
        <Typography color="textSecondary" variant="body2">{t('thisNode')}</Typography>
        {error &&
          <Alert severity="error" variant="filled">{error.message}</Alert>}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ paddingTop: '5px' }}>
          <TextField
            label={t('transport')}
            select
            size="small"
            fullWidth
            value={"grpc"}
          >
            <MenuItem value="grpc">GRPC</MenuItem>
          </TextField>
          <Accordion defaultExpanded elevation={0} disableGutters sx={{ marginTop: '20px' }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              {t('details')}
            </AccordionSummary>
            <AccordionDetails >
              {!isLoading &&
                <JSONBox data={transportLocalDetails} />
              }
            </AccordionDetails>
          </Accordion>
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
