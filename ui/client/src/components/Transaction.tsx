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

import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import { Box, Button, Grid2, Typography } from "@mui/material";
import daysjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { Captions, HashIcon } from 'lucide-react';
import { useState } from "react";
import { useTranslation } from 'react-i18next';
import iconLight from '../../public/paladin-icon-light.svg';
import { PaladinTransactionsReceiptDetailsDialog } from "../dialogs/TransactionReceiptDetails";
import { ViewDetailsDialog } from "../dialogs/ViewDetails";
import { IPaladinTransaction, ITransaction, ITransactionReceipt } from "../interfaces";
import { EllapsedTime } from "./EllapsedTime";
import { Hash } from "./Hash";

type Props = {
  transaction: ITransaction
  transactionReceipts?: ITransactionReceipt[]
  paladinTransactions?: IPaladinTransaction[]
}

daysjs.extend(relativeTime);

export const Transaction: React.FC<Props> = ({
  transaction,
  paladinTransactions,
  transactionReceipts,
}) => {

  const [viewDetailsDialogOpen, setViewDetailsDialogOpen] = useState(false);
  const receiptCount = (transactionReceipts && transactionReceipts.length) ? transactionReceipts.length : 0;
  const receiptIsPrivate = (transactionReceipts && transactionReceipts.length && transactionReceipts[0].domain !== undefined);
  const typeKey =
    receiptCount > 1 ? 'atomicNumber' :
      receiptIsPrivate ? 'private' :
        'public';
  const { t } = useTranslation();

  return (
    <>
      <Box sx={{
        position: 'relative',
        backgroundColor: theme => theme.palette.background.paper,
        marginBottom: '20px', borderRadius: '4px'
      }}>
        {receiptCount > 0 &&
          <img src={iconLight} width="40" style={{ position: 'absolute', left: '20px', bottom: '0px' }} />
        }
        <Box sx={{ padding: '10px', paddingLeft: '20px', paddingRight: '20px', borderBottom: theme => `solid 1px ${theme.palette.divider}` }}>
          <Grid2 container direction="column" spacing={2}>
            <Grid2 container justifyContent="space-between">
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{transaction.blockNumber.toLocaleString()}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('block')}</Typography>
              </Grid2>
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{transaction.transactionIndex}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('transactionIndex')}</Typography>
              </Grid2>
              <Grid2>
                <Typography align="center" variant="h6" color="textPrimary">{transaction.nonce}</Typography>
                <Typography align="center" variant="body2" color="textSecondary">{t('nonce')}</Typography>
              </Grid2>
              {receiptCount > 0 ?
                <Grid2>
                  <Typography align="center" variant="h6" color="textPrimary">{t(typeKey, { number: receiptCount })}</Typography>
                  <Typography align="center" variant="body2" color="textSecondary">{t('type')}</Typography>
                </Grid2>
                : receiptCount ?
                  <Grid2>
                    <Typography align="center" variant="h6" color="textPrimary">{t('receipt')}</Typography>
                    <Typography align="center" variant="body2" color="textSecondary">{t('type')}</Typography>
                  </Grid2>
                  : undefined
              }
              <Grid2 sx={{ textAlign: 'center' }} alignContent="center">
                {transaction.result === 'success' ? <CheckCircleOutlineIcon color="primary" /> : <ErrorOutlineIcon color="error" />}
                <Typography align="center" variant="body2" color="textSecondary">{t('result')}</Typography>
              </Grid2>
            </Grid2>
          </Grid2>
        </Box>
        <Box sx={{ padding: '10px', paddingBottom: '20px' }}>
          <Grid2 container justifyContent="space-evenly" wrap="nowrap" spacing={2}>
            {transaction.contractAddress ?
              <>
                <Grid2 size={{ xs: 12, sm: 4 }}>
                  <Hash Icon={<Captions size="18px" />} title={t('hash')} hash={transaction.hash} />
                </Grid2>
                <Grid2 size={{ xs: 12, sm: 4 }}>
                  <Hash Icon={<Captions size="18px"/>} title={t('from')} hash={transaction.from} />
                </Grid2>
                <Grid2 size={{ xs: 12, sm: 4 }}>
                  <Hash Icon={<Captions size="18px"/>} title={t('contract')} hash={transaction.contractAddress} />
                </Grid2>
              </>
              :
              <>
                <Grid2 size={{ xs: 12, sm: 6 }}>
                  <Hash Icon={<HashIcon size="18px" />} title={t('hash')} hash={transaction.hash} />
                </Grid2>
                <Grid2 size={{ xs: 12, sm: 6 }}>
                  <Hash Icon={<Captions size="18px"/>} title={t('from')} hash={transaction.from} />
                </Grid2>
              </>
            }
          </Grid2>
        </Box>
        <Box sx={{ display: 'flex', padding: '10px', justifyContent: 'end' }}>
          <EllapsedTime timestamp={transaction.block.timestamp} />
          <Button sx={{ marginLeft: '20px', fontWeight: '400' }} size="small" startIcon={<VisibilityIcon />}
            onClick={() => setViewDetailsDialogOpen(true)}>{t('viewDetails')}</Button>
        </Box>
      </Box>
      {transactionReceipts && transactionReceipts.length > 0 ?
        <PaladinTransactionsReceiptDetailsDialog
          paladinReceipts={transactionReceipts}
          paladinTransactions={paladinTransactions}
          dialogOpen={viewDetailsDialogOpen}
          setDialogOpen={setViewDetailsDialogOpen}
        />
        :
        <ViewDetailsDialog
          title={t('transaction')}
          details={transaction}
          dialogOpen={viewDetailsDialogOpen}
          setDialogOpen={setViewDetailsDialogOpen}
        />}
    </>
  );

}