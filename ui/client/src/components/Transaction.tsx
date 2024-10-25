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
  IPaladinTransaction,
  ITransaction,
  ITransactionReceipt,
} from "@/interfaces/transactions";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import { Box, ButtonBase, Grid2, Typography } from "@mui/material";
import { t } from "i18next";
import { useState } from "react";
import { PaladinTransactionDialog } from "../dialogs/PaladinTransaction";
import { Hash } from "./Hash";

type Props = {
  transaction: ITransaction;
  transactionReceipt?: ITransactionReceipt;
  paladinTransaction?: IPaladinTransaction;
};

export const Transaction: React.FC<Props> = ({
  transaction,
  transactionReceipt,
  paladinTransaction,
}) => {
  const [paladinTransactionDialogOpen, setPaladinTransactionDialogOpen] =
    useState(false);

  return (
    <>
      <Box
        sx={{
          position: "relative",
          backgroundColor: (theme) => theme.palette.background.paper,
          marginBottom: "20px",
          padding: "10px",
          borderRadius: "6px",
          boxShadow: "0px 0px 8px 3px rgba(0,0,0,0.26)",
        }}
      >
        {paladinTransaction !== undefined && (
          <img
            src="/paladin-icon-light.svg"
            width="45"
            style={{ position: "absolute", left: "4px", bottom: "0px" }}
          />
        )}
        <Grid2 container direction="column" spacing={2}>
          <Grid2 container justifyContent="space-evenly">
            {paladinTransaction !== undefined && (
              <Grid2>
                <ButtonBase
                  onClick={() => setPaladinTransactionDialogOpen(true)}
                >
                  <Typography align="center" variant="h6" color="primary">
                    {t(paladinTransaction.type)}
                  </Typography>
                </ButtonBase>
                <Typography
                  align="center"
                  variant="body2"
                  color="textSecondary"
                >
                  {t("type")}
                </Typography>
              </Grid2>
            )}
            <Grid2>
              <Typography align="center" variant="h6" color="textPrimary">
                {transaction.blockNumber.toLocaleString()}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("block")}
              </Typography>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="h6" color="textPrimary">
                {transaction.transactionIndex}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("transactionIndex")}
              </Typography>
            </Grid2>
            <Grid2>
              <Typography align="center" variant="h6" color="textPrimary">
                {transaction.nonce}
              </Typography>
              <Typography align="center" variant="body2" color="textSecondary">
                {t("nonce")}
              </Typography>
            </Grid2>
            <Grid2 sx={{ textAlign: "center" }} alignContent="center">
              {transaction.result === "success" ? (
                <CheckCircleOutlineIcon color="primary" />
              ) : (
                <ErrorOutlineIcon color="error" />
              )}
              <Typography align="center" variant="body2" color="textSecondary">
                {t("result")}
              </Typography>
            </Grid2>
          </Grid2>
          <Grid2 container justifyContent="space-evenly" wrap="nowrap">
            <Grid2>
              <Hash title={t("hash")} hash={transaction.hash} />
              <Typography align="center" variant="body2" color="textSecondary">
                {t("hash")}
              </Typography>
            </Grid2>
            <Grid2>
              <Hash title={t("from")} hash={transaction.from} />
              <Typography align="center" variant="body2" color="textSecondary">
                {t("from")}
              </Typography>
            </Grid2>
            {transaction.contractAddress && (
              <Grid2>
                <Hash
                  title={t("contract")}
                  hash={transaction.contractAddress}
                />
                <Typography
                  align="center"
                  variant="body2"
                  color="textSecondary"
                >
                  {t("contract")}
                </Typography>
              </Grid2>
            )}
          </Grid2>
        </Grid2>
      </Box>
      {transactionReceipt !== undefined && paladinTransaction !== undefined && (
        <PaladinTransactionDialog
          paladinTransaction={paladinTransaction}
          dialogOpen={paladinTransactionDialogOpen}
          setDialogOpen={setPaladinTransactionDialogOpen}
        />
      )}
    </>
  );
};
