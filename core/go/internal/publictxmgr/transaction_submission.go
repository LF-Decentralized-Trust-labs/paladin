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

package publictxmgr

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"golang.org/x/crypto/sha3"
)

func calculateTransactionHash(rawTxnData []byte) *tktypes.Bytes32 {
	if rawTxnData == nil {
		return nil
	}
	msgHash := sha3.NewLegacyKeccak256()
	msgHash.Write(rawTxnData)
	hashBytes := tktypes.MustParseBytes32(hex.EncodeToString(msgHash.Sum(nil)))
	return &hashBytes
}

func (it *InFlightTransactionStageController) submitTX(ctx context.Context, mtx InMemoryTxStateReadOnly, signedMessage []byte) (*tktypes.Bytes32, *tktypes.Timestamp, ethclient.ErrorReason, SubmissionOutcome, error) {
	var txHash *tktypes.Bytes32
	sendStart := time.Now()
	calculatedTxHash := mtx.GetTransactionHash() // must have been persisted in previous stage
	log.L(ctx).Debugf("Sending raw transaction %s (lastSubmit=%s), Hash=%s", mtx.GetSignerNonce(), mtx.GetLastSubmitTime(), txHash)

	submissionTime := confutil.P(tktypes.TimestampNow())
	var submissionErrorReason ethclient.ErrorReason // TODO: fix reason parsing
	var submissionOutcome SubmissionOutcome
	var submissionError error

	retryError := it.transactionSubmissionRetry.Do(ctx, func(attempt int) ( /*retry*/ bool, error) {
		txHash, submissionError = it.ethClient.SendRawTransaction(ctx, tktypes.HexBytes(signedMessage))
		if submissionError == nil {
			submissionOutcome = SubmissionOutcomeFailedRequiresRetry
			it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationTransactionSend), string(GenericStatusSuccess), time.Since(sendStart).Seconds())
			if txHash != nil {
				if txHash != nil && calculatedTxHash != nil && txHash.String() != calculatedTxHash.String() {
					// TODO: Investigate why under high concurrency load with Besu we are triggering this, and the returned hash is for
					//       a DIFFERENT NONCE that is submitted at an extremely close time.
					log.L(ctx).Warnf("Received response for transaction %s, but calculated transaction hash %s is different from the response %s.", mtx.GetSignerNonce(), calculatedTxHash, txHash)
					submissionError = i18n.NewError(ctx, msgs.MsgSubmitFailedWrongHashReturned, calculatedTxHash, txHash)
					txHash = nil // clear the transaction hash as we are not certain it's correct
					return true, submissionError
				} else {
					log.L(ctx).Debugf("Submitted %s successfully with hash=%s", mtx.GetSignerNonce(), txHash)
				}
			} else {
				txHash = calculatedTxHash
				log.L(ctx).Warnf("Received response for transaction %s, no transaction hash from the response, using the calculated transaction hash %s instead.", mtx.GetSignerNonce(), txHash)
			}
			log.L(ctx).Infof("Transaction %s submitted. Hash: %s", mtx.GetSignerNonce(), calculatedTxHash)
			submissionOutcome = SubmissionOutcomeSubmittedNew
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
				it.gasPriceClient.DeleteCache(ctx)
				log.L(ctx).Debug("Underpriced, removed gas price cache")
				submissionOutcome = SubmissionOutcomeFailedRequiresRetry
			case ethclient.ErrorReasonTransactionReverted:
				// transaction could be reverted due to gas estimate too low, clear the cache before try again
				it.gasPriceClient.DeleteCache(ctx)
				log.L(ctx).Debug("Transaction reverted, removed gas price cache")
				submissionOutcome = SubmissionOutcomeFailedRequiresRetry
			case ethclient.ErrorKnownTransaction:
				// check mined transaction also returns this error code
				// KnownTransaction means it's in the mempool
				log.L(ctx).Debugf("Transaction %s known with hash: %s (previous=%s)", mtx.GetSignerNonce(), txHash, submissionError)
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = SubmissionOutcomeAlreadyKnown
			case ethclient.ErrorReasonNonceTooLow:
				// NonceTooLow means a transaction with same nonce is already mined, this could mean:
				//   1. we have a nonce conflict
				//   2. our transaction is completed and we are waiting for the confirmation
				log.L(ctx).Debugf("Nonce too low for transaction ID: %s. new transaction hash: %s, recorded transaction hash: %s", mtx.GetSignerNonce(), txHash, mtx.GetTransactionHash())
				// otherwise, we revert back to track the old hash
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = SubmissionOutcomeNonceTooLow
			default:
				submissionOutcome = SubmissionOutcomeFailedRequiresRetry
				if attempt <= it.transactionSubmissionRetryCount {
					return true, submissionError
				}
			}
			return false, nil
		}
	})

	if retryError != nil {
		return nil, nil, ethclient.ErrorReason(retryError.Error()), SubmissionOutcomeFailedRequiresRetry, retryError
	}

	return txHash, submissionTime, submissionErrorReason, submissionOutcome, submissionError
}
