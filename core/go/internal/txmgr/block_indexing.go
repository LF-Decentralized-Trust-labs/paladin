/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package txmgr

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/pkg/blockindexer"
	"github.com/LFDT-Paladin/paladin/core/pkg/persistence"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func (tm *txManager) blockIndexerPreCommit(
	ctx context.Context,
	dbTX persistence.DBTX,
	blocks []*pldapi.IndexedBlock,
	transactions []*blockindexer.IndexedTransactionNotify,
) error {

	// Log all the transactions we've been passed
	for _, tx := range transactions {
		log.L(ctx).Infof("Received block %d transaction %s hash=%s type=%+v. Are we going to put it in the receipts table?",
			tx.BlockNumber, tx.ContractAddress, tx.Hash, tx.Nonce)
	}

	// Pass the list of transactions to the public transaction manager, who will pass us back an
	// ORDERED list of matches to transaction IDs based on the bindings.
	txMatches, err := tm.publicTxMgr.MatchUpdateConfirmedTransactions(ctx, dbTX, transactions)
	if err != nil {
		return err
	}

	// Ok now we have an ordered list of completions that match Paladin transactions
	// - If they are public paladin transactions - just finalize the receipts on this routine
	// - If they are private paladin transactions - the private TX manager only needs to be
	// notified if it was a failure. Because success cases are processed as events on the
	// separate ordering context of the block listener of that domain (we do not promise
	// order of confirmation delivery between public and private transactions)
	finalizeInfo := make([]*components.ReceiptInput, 0, len(txMatches))
	failedForPrivateTx := make([]*components.PublicTxMatch, 0)

	// First finalize all public transactions, and record all failed public submissions for private transactions
	for _, match := range txMatches {
		log.L(ctx).Infof("blockIndexerPreCommit: processing next TX match %+v", match)
		switch match.TransactionType.V() {
		case pldapi.TransactionTypePublic:
			log.L(ctx).Infof("Writing receipt for transaction %s hash=%s block=%d result=%s",
				match.TransactionID, match.Hash, match.BlockNumber, match.Result)
			// Map to the common format for finalizing transactions whether the make it on chain or not
			finalizeInfo = append(finalizeInfo, tm.mapBlockchainReceipt(match))
		case pldapi.TransactionTypePrivate:
			if match.Result.V() != pldapi.TXResult_SUCCESS {
				log.L(ctx).Infof("Base ledger transaction for private transaction %s FAILED hash=%s block=%d result=%s",
					match.TransactionID, match.Hash, match.BlockNumber, match.Result)
				failedForPrivateTx = append(failedForPrivateTx, match)
			}
		}
	}

	// It's technically possible that more than 1 public transaction result is received for the same
	// private transaction, even within the same block. If any one public TX is successful the private
	// TX is considered successful so we need to double check if any failures can be ignored because of
	// a success that overrides it.
	for _, match := range txMatches {
		log.L(ctx).Infof("blockIndexerPreCommit: re-processing next TX match %+v", match)
		switch match.TransactionType.V() {
		case pldapi.TransactionTypePrivate:
			if match.Result.V() == pldapi.TXResult_SUCCESS {
				log.L(ctx).Infof("Base ledger transaction for private transaction %s SUCCESS hash=%s block=%d result=%s",
					match.TransactionID, match.Hash, match.BlockNumber, match.Result)

				// If this private TX also had a failed public submission in this block, ignore it
				for i, failedTx := range failedForPrivateTx {
					if failedTx.TransactionID == match.TransactionID {
						log.L(ctx).Infof("Base ledger transaction for private transaction %s SUCCESS overrides failed public submission %s hash=%s block=%d result=%s",
							match.TransactionID, failedTx.TransactionID, failedTx.Hash, failedTx.BlockNumber, failedTx.Result)
						// Remove the failed transaction from the list
						failedForPrivateTx = append(failedForPrivateTx[:i], failedForPrivateTx[i+1:]...)
					}
				}
			}
		}
	}

	// Write the receipts themselves - only way of duplicates should be a rewind of
	// the block explorer, so we simply OnConflict ignore
	err = tm.FinalizeTransactions(ctx, dbTX, finalizeInfo)
	if err != nil {
		return err
	}

	// Deliver the failures to the distributed sequencer
	if len(failedForPrivateTx) > 0 {
		err = tm.sequencerMgr.HandleTransactionFailed(ctx, dbTX, failedForPrivateTx)
		if err != nil {
			return err
		}
	}

	dbTX.AddPostCommit(func(ctx context.Context) {
		// We need to notify the public TX manager when the DB transaction for these has completed,
		// so it can remove any in-memory processing (this is regardless of they were matched to
		// a public or private transaction)
		if len(txMatches) > 0 {
			tm.publicTxMgr.NotifyConfirmPersisted(ctx, txMatches)
		}
	})
	return nil
}

func (tm *txManager) mapBlockchainReceipt(pubTx *components.PublicTxMatch) *components.ReceiptInput {
	receipt := &components.ReceiptInput{
		TransactionID: pubTx.TransactionID,
		OnChain: pldtypes.OnChainLocation{
			Type:             pldtypes.OnChainTransaction,
			TransactionHash:  pubTx.Hash,
			BlockNumber:      pubTx.BlockNumber,
			TransactionIndex: pubTx.TransactionIndex,
		},
		ContractAddress: pubTx.ContractAddress,
		RevertData:      pubTx.RevertReason,
	}
	if pubTx.Result.V() == pldapi.TXResult_SUCCESS {
		receipt.ReceiptType = components.RT_Success
	} else {
		receipt.ReceiptType = components.RT_FailedOnChainWithRevertData
		receipt.RevertData = pubTx.RevertReason
	}
	return receipt
}
