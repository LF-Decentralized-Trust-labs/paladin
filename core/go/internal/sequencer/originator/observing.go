/*
 * Copyright © 2025 Kaleido, Inc.
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

package originator

import (
	"context"
	"fmt"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/transaction"
)

func (o *originator) applyHeartbeatReceived(ctx context.Context, event *HeartbeatReceivedEvent) error {
	o.timeOfMostRecentHeartbeat = o.clock.Now()
	o.activeCoordinatorNode = event.From
	o.latestCoordinatorSnapshot = &event.CoordinatorSnapshot
	for _, dispatchedTransaction := range event.DispatchedTransactions {
		//if any of the dispatched transactions were sent by this originator, ensure that we have an up to date view of its state
		if dispatchedTransaction.Originator == o.nodeName {
			txn := o.transactionsByID[dispatchedTransaction.ID]
			if txn == nil {
				//unexpected situation to be in.  We trust our memory of transactions over the coordinator's, so we ignore this transaction
				log.L(ctx).Warnf("received heartbeat from %s with dispatched transaction %s but no transaction found in memory", o.activeCoordinatorNode, dispatchedTransaction.ID)
				continue
			}
			if dispatchedTransaction.LatestSubmissionHash != nil {
				//if the dispatched transaction has a hash, then we can update our view of the transaction
				txnSubmittedEvent := &transaction.SubmittedEvent{}
				txnSubmittedEvent.TransactionID = dispatchedTransaction.ID
				txnSubmittedEvent.SignerAddress = dispatchedTransaction.Signer
				txnSubmittedEvent.LatestSubmissionHash = *dispatchedTransaction.LatestSubmissionHash
				if dispatchedTransaction.Nonce != nil {
					txnSubmittedEvent.Nonce = *dispatchedTransaction.Nonce
				}

				err := txn.HandleEvent(ctx, txnSubmittedEvent)
				if err != nil {
					msg := fmt.Sprintf("error handling transaction submitted event for transaction %s: %v", txn.ID, err)
					log.L(ctx).Error(msg)
					return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
				}
				o.submittedTransactionsByHash[*dispatchedTransaction.LatestSubmissionHash] = &dispatchedTransaction.ID
			} else if dispatchedTransaction.Nonce != nil {
				//if the dispatched transaction has a nonce but no hash, then it is sequenced
				err := txn.HandleEvent(ctx, &transaction.NonceAssignedEvent{
					BaseEvent: transaction.BaseEvent{
						TransactionID: dispatchedTransaction.ID,
					},
					Nonce: *dispatchedTransaction.Nonce,
				})

				if err != nil {
					msg := fmt.Sprintf("error handling nonce assigned event for transaction %s: %v", txn.ID, err)
					log.L(ctx).Error(msg)
					return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
				}
			}
		}
	}

	// TODO process other lists in the heartbeat event.
	// Note: sending dropped transaction re-delegations (i.e. those we are tracking but which th heartbeat doesn't mention)
	// is handled by state machine guards

	return nil
}

func guard_HeartbeatThresholdExceeded(ctx context.Context, o *originator) bool {
	if o.timeOfMostRecentHeartbeat == nil {
		//we have never seen a heartbeat so that was a really long time ago, certainly longer than any threshold
		return true
	}
	if o.clock.HasExpired(o.timeOfMostRecentHeartbeat, o.heartbeatThresholdMs) {
		return true
	}
	return false
}
