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

package baseledgertx

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"golang.org/x/crypto/sha3"
)

func calculateTransactionHash(rawTxnData []byte) *types.Bytes32 {
	if rawTxnData == nil {
		return nil
	}
	msgHash := sha3.NewLegacyKeccak256()
	msgHash.Write(rawTxnData)
	hashBytes := types.MustParseBytes32(hex.EncodeToString(msgHash.Sum(nil)))
	return &hashBytes
}

func (it *InFlightTransactionStageController) submitTX(ctx context.Context, mtx *baseTypes.ManagedTX, signedMessage []byte) (string, *fftypes.FFTime, ethclient.ErrorReason, baseTypes.SubmissionOutcome, error) {
	var txHash *types.Bytes32
	sendStart := time.Now()
	calculatedTxHash := calculateTransactionHash(signedMessage)
	log.L(ctx).Debugf("Sending raw transaction %s at nonce %s / %d (lastSubmit=%s), Hash= %s, Data=%s", mtx.ID, mtx.From, mtx.Nonce.Int64(), mtx.LastSubmit, txHash, mtx.Data)

	submissionTime := fftypes.Now()
	var submissionErrorReason ethclient.ErrorReason // TODO: fix reason parsing
	var submissionOutcome baseTypes.SubmissionOutcome
	var submissionError error

	retryError := it.transactionSubmissionRetry.Do(ctx, fmt.Sprintf("tx submission  %s/%s", mtx.ID, calculatedTxHash), func(attempt int) ( /*retry*/ bool, error) {
		txHash, submissionError = it.ethClient.SendRawTransaction(ctx, types.HexBytes(types.MustParseHexBytes(string(signedMessage)).HexString0xPrefix()))
		if submissionError == nil {
			it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationTransactionSend), string(GenericStatusSuccess), time.Since(sendStart).Seconds())
			if txHash != nil {
				if txHash != nil && calculatedTxHash != nil && txHash.String() != calculatedTxHash.String() {
					// TODO: Investigate why under high concurrency load with Besu we are triggering this, and the returned hash is for
					//       a DIFFERENT NONCE that is submitted at an extremely close time.
					log.L(ctx).Warnf("Received response for transaction %s, but calculated transaction hash %s is different from the response %s.", mtx.ID, calculatedTxHash, txHash)
					submissionError = i18n.NewError(ctx, msgs.MsgSubmitFailedWrongHashReturned, calculatedTxHash, txHash)
					txHash = nil // clear the transaction hash as we are not certain it's correct
					if attempt <= it.transactionSubmissionRetryCount {
						return true, submissionError
					} else {
						submissionOutcome = baseTypes.SubmissionOutcomeFailedRequiresRetry
						return false, nil
					}
				} else {
					log.L(ctx).Debugf("Submitted %s successfully with hash=%s", mtx.ID, txHash)
				}
			} else {
				txHash = calculatedTxHash
				log.L(ctx).Warnf("Received response for transaction %s, no transaction hash from the response, using the calculated transaction hash %s instead.", mtx.ID, txHash)
			}
			log.L(ctx).Infof("Transaction %s at nonce %s / %d submitted. Hash: %s", mtx.ID, mtx.From, mtx.Nonce.Int64(), mtx.TransactionHash)
			submissionOutcome = baseTypes.SubmissionOutcomeSubmittedNew
			return false, nil
		} else {
			if calculatedTxHash != nil {
				txHash = calculatedTxHash
			}
			submissionErrorReason = ethclient.MapError(submissionError)
			it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationTransactionSend), string(GenericStatusFail), time.Since(sendStart).Seconds())
			// We have some simple rules for handling reasons from the connector, which could be enhanced by extending the connector.
			switch submissionErrorReason {
			case ethclient.ErrorReasonTransactionUnderpriced:
				// if this is not already a retry
				// retry the request without using the oracle immediately as the oracle sometimes set the price too low for the node to accept
				// this is because each node can set the gas price limit in the config which is independent from other nodes
				// but a gas oracle typically come up the value based on the data collected from all nodes
				_ = it.gasPriceClient.DeleteCache(ctx)
				log.L(ctx).Debug("Underpriced, removed gas price cache")
				submissionOutcome = baseTypes.SubmissionOutcomeFailedRequiresRetry
			case ethclient.ErrorReasonTransactionReverted:
				// transaction could be reverted due to gas estimate too low, clear the cache before try again
				_ = it.gasPriceClient.DeleteCache(ctx)
				log.L(ctx).Debug("Transaction reverted, removed gas price cache")
				submissionOutcome = baseTypes.SubmissionOutcomeFailedRequiresRetry
			case ethclient.ErrorKnownTransaction:
				// check mined transaction also returns this error code
				// KnownTransaction means it's in the mempool
				log.L(ctx).Debugf("Transaction %s at nonce %s / %d known with hash: %s (previous=%s)", mtx.ID, mtx.From, mtx.Nonce.Int64(), txHash, submissionError)
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = baseTypes.SubmissionOutcomeAlreadyKnown
			case ethclient.ErrorReasonNonceTooLow:
				// NonceTooLow means a transaction with same nonce is already mined, this could mean:
				//   1. we have a nonce conflict
				//   2. our transaction is completed and we are waiting for the receipt
				// TODO: handle nonce conflict
				log.L(ctx).Debugf("Nonce too low for transaction ID: %s. new transaction hash: %s, recorded transaction hash: %s", mtx.ID, txHash, mtx.TransactionHash)
				// otherwise, we revert back to track the old hash
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = baseTypes.SubmissionOutcomeNonceTooLow
			default:
				submissionOutcome = baseTypes.SubmissionOutcomeFailedRequiresRetry
				if attempt <= it.transactionSubmissionRetryCount {
					return true, submissionError
				}
			}
			return false, nil
		}
	})

	if retryError != nil {
		return "", nil, ethclient.ErrorReason(retryError.Error()), baseTypes.SubmissionOutcomeFailedRequiresRetry, retryError
	}

	txHashString := ""
	if txHash != nil {
		txHashString = txHash.String()
	}

	return txHashString, submissionTime, submissionErrorReason, submissionOutcome, submissionError
}
