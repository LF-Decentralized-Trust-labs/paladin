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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"golang.org/x/crypto/sha3"
)

func calculateTransactionHash(rawTxnData []byte) *pldtypes.Bytes32 {
	if rawTxnData == nil {
		return nil
	}
	msgHash := sha3.NewLegacyKeccak256()
	msgHash.Write(rawTxnData)
	hashBytes := pldtypes.MustParseBytes32(hex.EncodeToString(msgHash.Sum(nil)))
	return &hashBytes
}

func (it *inFlightTransactionStageController) submitTX(ctx context.Context, signedMessage []byte, calculatedTxHash *pldtypes.Bytes32, signerNonce string, lastSubmitTime *pldtypes.Timestamp, cancelled func(context.Context) bool) (*pldtypes.Bytes32, *pldtypes.Timestamp, ethclient.ErrorReason, SubmissionOutcome, error) {
	var txHash *pldtypes.Bytes32
	sendStart := time.Now()
	if calculatedTxHash == nil {
		return nil, nil, ethclient.ErrorReasonInvalidInputs, SubmissionOutcomeFailedRequiresRetry, i18n.NewError(ctx, msgs.MsgInvalidStateMissingTXHash)
	}
	log.L(ctx).Debugf("Sending raw transaction %s (lastSubmit=%s), Hash=%s", signerNonce, lastSubmitTime, calculatedTxHash)

	submissionTime := confutil.P(pldtypes.TimestampNow())
	var submissionErrorReason ethclient.ErrorReason // TODO: fix reason parsing
	var submissionOutcome SubmissionOutcome
	var submissionError error

	retryError := it.transactionSubmissionRetry.Do(ctx, func(attempt int) ( /*retry*/ bool, error) {
		if cancelled(ctx) {
			return false, nil
		}
		txHash, submissionError = it.ethClient.SendRawTransaction(ctx, pldtypes.HexBytes(signedMessage))
		if submissionError == nil {
			submissionOutcome = SubmissionOutcomeFailedRequiresRetry
			it.thMetrics.RecordOperationMetrics(ctx, string(InFlightTxOperationTransactionSend), string(GenericStatusSuccess), time.Since(sendStart).Seconds())
			if txHash != nil {
				if calculatedTxHash != nil && txHash.String() != calculatedTxHash.String() {
					// TODO: Investigate why under high concurrency load with Besu we are triggering this, and the returned hash is for
					//       a DIFFERENT NONCE that is submitted at an extremely close time.
					log.L(ctx).Warnf("Received response for transaction %s, but calculated transaction hash %s is different from the response %s.", signerNonce, calculatedTxHash, txHash)
					submissionError = i18n.NewError(ctx, msgs.MsgSubmitFailedWrongHashReturned, calculatedTxHash, txHash)
					txHash = nil // clear the transaction hash as we are not certain it's correct
					return true, submissionError
				} else {
					log.L(ctx).Debugf("Submitted %s successfully with hash=%s", signerNonce, txHash)
				}
			} else {
				txHash = calculatedTxHash
				log.L(ctx).Warnf("Received response for transaction %s, no transaction hash from the response, using the calculated transaction hash %s instead.", signerNonce, txHash)
			}
			log.L(ctx).Infof("Transaction %s submitted. Hash: %s", signerNonce, calculatedTxHash)
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
				log.L(ctx).Debugf("Transaction %s known with hash: %s (previous=%s)", signerNonce, txHash, submissionError)
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = SubmissionOutcomeAlreadyKnown
			case ethclient.ErrorReasonNonceTooLow:
				// NonceTooLow means a transaction with same nonce is already mined, this could mean:
				//   1. we have a nonce conflict
				//   2. our transaction is completed and we are waiting for the confirmation
				log.L(ctx).Debugf("Nonce too low for transaction ID: %s. new transaction hash: %s, recorded transaction hash: %s", signerNonce, txHash, calculatedTxHash)
				// otherwise, we revert back to track the old hash
				submissionError = nil
				submissionErrorReason = ""
				submissionOutcome = SubmissionOutcomeNonceTooLow
			default:
				log.L(ctx).Errorf("Submission error for transaction ID %s with hash %s (requires retry): %s", signerNonce, txHash, submissionError)
				submissionOutcome = SubmissionOutcomeFailedRequiresRetry
				return true, submissionError
			}
			return false, nil
		}
	})

	if retryError != nil {
		return nil, submissionTime, submissionErrorReason, SubmissionOutcomeFailedRequiresRetry, retryError
	}

	return txHash, submissionTime, submissionErrorReason, submissionOutcome, submissionError
}
