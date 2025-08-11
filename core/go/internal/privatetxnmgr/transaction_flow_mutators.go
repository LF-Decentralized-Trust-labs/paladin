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

package privatetxnmgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func (tf *transactionFlow) ApplyEvent(ctx context.Context, event ptmgrtypes.PrivateTransactionEvent) {
	tf.statusLock.Lock()
	defer tf.statusLock.Unlock()

	//First we update our in memory record of the transaction with the data from the event
	switch event := event.(type) {
	case *ptmgrtypes.TransactionSubmittedEvent:
		tf.applyTransactionSubmittedEvent(ctx, event)
	case *ptmgrtypes.TransactionSwappedInEvent:
		tf.applyTransactionSwappedInEvent(ctx, event)
	case *ptmgrtypes.TransactionSignedEvent:
		tf.applyTransactionSignedEvent(ctx, event)
	case *ptmgrtypes.TransactionEndorsedEvent:
		tf.applyTransactionEndorsedEvent(ctx, event)
	case *ptmgrtypes.TransactionAssembledEvent:
		tf.applyTransactionAssembledEvent(ctx, event)
	case *ptmgrtypes.TransactionAssembleFailedEvent:
		tf.applyTransactionAssembleFailedEvent(ctx, event)
	case *ptmgrtypes.TransactionDispatchedEvent:
		tf.applyTransactionDispatchedEvent(ctx, event)
	case *ptmgrtypes.TransactionPreparedEvent:
		tf.applyTransactionPreparedEvent(ctx, event)
	case *ptmgrtypes.TransactionConfirmedEvent:
		tf.applyTransactionConfirmedEvent(ctx, event)
	case *ptmgrtypes.TransactionRevertedEvent:
		tf.applyTransactionRevertedEvent(ctx, event)
	case *ptmgrtypes.TransactionDelegationAcknowledgedEvent:
		tf.applyTransactionDelegationAcknowledgedEvent(ctx, event)
	case *ptmgrtypes.ResolveVerifierResponseEvent:
		tf.applyResolveVerifierResponseEvent(ctx, event)
	case *ptmgrtypes.ResolveVerifierErrorEvent:
		tf.applyResolveVerifierErrorEvent(ctx, event)
	case *ptmgrtypes.TransactionFinalizedEvent:
		tf.applyTransactionFinalizedEvent(ctx, event)
	case *ptmgrtypes.TransactionFinalizeError:
		tf.applyTransactionFinalizeError(ctx, event)
	case *ptmgrtypes.TransactionNudgeEvent:
		tf.applyTransactionNudgeEvent(ctx, event)
	case *ptmgrtypes.DelegationForInFlightEvent:
		tf.applyDelegationForInFlightEvent(ctx, event)

	default:
		log.L(ctx).Warnf("Unknown event type: %T", event)
	}
}

func (tf *transactionFlow) applyTransactionSubmittedEvent(ctx context.Context, _ *ptmgrtypes.TransactionSubmittedEvent) {
	log.L(ctx).Debug("transactionFlow:applyTransactionSubmittedEvent")

	tf.latestEvent = "TransactionSubmittedEvent"

}

func (tf *transactionFlow) applyTransactionSwappedInEvent(ctx context.Context, _ *ptmgrtypes.TransactionSwappedInEvent) {
	log.L(ctx).Debug("transactionFlow:applyTransactionSwappedInEvent")

	tf.latestEvent = "TransactionSwappedInEvent"

}

func (tf *transactionFlow) applyTransactionAssembledEvent(ctx context.Context, event *ptmgrtypes.TransactionAssembledEvent) {
	log.L(ctx).Debug("transactionFlow:applyTransactionAssembledEvent")

	tf.latestEvent = "TransactionAssembledEvent"
	tf.transaction.PostAssembly = event.PostAssembly
	tf.assemblePending = false
	if tf.transaction.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_REVERT {
		// Not sure if any domains actually use this but it is a valid response to indicate failure
		log.L(ctx).Errorf("AssemblyResult is AssembleTransactionResponse_REVERT")
		revertReason := ""
		if event.PostAssembly.RevertReason != nil {
			revertReason = *event.PostAssembly.RevertReason
		}
		tf.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerAssembleRevert), revertReason))
		tf.assembleCoordinator.Complete(event.AssembleRequestID)
		return
	}
	if tf.transaction.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_PARK {

		log.L(ctx).Infof("AssemblyResult is AssembleTransactionResponse_PARK")
		tf.status = "parked"
		tf.assemblePending = false
		tf.assembleCoordinator.Complete(event.AssembleRequestID)
		return
	}
	tf.status = "assembled"
	tf.writeAndLockStates(ctx)

	//allow assembly thread to proceed
	tf.assembleCoordinator.Complete(event.AssembleRequestID)

}

func (tf *transactionFlow) applyTransactionAssembleFailedEvent(ctx context.Context, event *ptmgrtypes.TransactionAssembleFailedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionAssembleFailedEvent: RequestID: '%s' Error: %s ", event.AssembleRequestID, event.Error)
	tf.latestEvent = "TransactionAssembleFailedEvent"
	tf.latestError = event.Error
	// set assemblePending to false so that the transaction can be re-assembled
	tf.assemblePending = false
	tf.assembleCoordinator.Complete(event.AssembleRequestID)
}

func (tf *transactionFlow) applyTransactionSignedEvent(ctx context.Context, event *ptmgrtypes.TransactionSignedEvent) {
	tf.latestEvent = "TransactionSignedEvent"
	log.L(ctx).Debugf("Adding signature to transaction %s", tf.transaction.ID.String())
	tf.transaction.PostAssembly.Signatures = append(tf.transaction.PostAssembly.Signatures, event.AttestationResult)

}

func (tf *transactionFlow) applyTransactionEndorsedEvent(ctx context.Context, event *ptmgrtypes.TransactionEndorsedEvent) {
	tf.latestEvent = "TransactionEndorsedEvent"
	log.L(ctx).Debugf("transactionFlow:applyTransactionEndorsedEvent: TransactionID: '%s' IdempotencyKey: '%s' Party: %s ", event.TransactionID, event.IdempotencyKey, event.Party)

	//if this response does not match a pending request, then we ignore it
	pendingRequestsForAttRequestName, ok := tf.pendingEndorsementRequests[event.AttestationRequestName]
	if !ok {
		log.L(ctx).Warnf("Received endorsement for unknown request name %s", event.AttestationRequestName)
		return
	}
	pendingRequest, ok := pendingRequestsForAttRequestName[event.Party]
	if !ok {
		log.L(ctx).Warnf("Received endorsement for unknown party %s", event.Party)
		return
	}

	if pendingRequest.idempotencyKey != event.IdempotencyKey {
		log.L(ctx).Debugf("Pending request idempotencyKey %s does not match endorsement idempotencyKey %s, assuming response from obsolete request", pendingRequest.idempotencyKey, event.IdempotencyKey)
		return
	}
	//we have (had) a pending request for this endorsement but it is no longer pending because we now have a response
	delete(pendingRequestsForAttRequestName, event.Party)

	if event.RevertReason != nil {
		log.L(ctx).Infof("Endorsement for transaction %s was rejected: %s", tf.transaction.ID.String(), *event.RevertReason)
		// endorsement errors trigger a re-assemble
		// if the reason for the endorsement error is a change of state of the universe since the transaction was assembled, then the re-assemble may fail and cause the transaction to be reverted
		// on the other hand, the re-assemble may result in an endorsable version of the transaction.
		// either way, we trigger the re-assembly and hope for the best
		//TODO - there may be other endorsements that are en route, based on the previous assembly.  Need to make sure that
		// we discard them when they do return.
		//only apply at this stage, action will be taken later
		tf.transaction.PostAssembly = nil
		// remove all pending endorsement request records because they are no longer valid
		tf.pendingEndorsementRequests = make(map[string]map[string]*endorsementRequest)

	} else {
		log.L(ctx).Infof("Adding endorsement from %s to transaction %s", event.Endorsement.Verifier.Lookup, tf.transaction.ID.String())
		tf.transaction.PostAssembly.Endorsements = append(tf.transaction.PostAssembly.Endorsements, event.Endorsement)

	}
}

func (tf *transactionFlow) applyTransactionDispatchedEvent(ctx context.Context, event *ptmgrtypes.TransactionDispatchedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionDispatchedEvent transactionID:%s nonce:%d signingAddress:%s", tf.transaction.ID.String(), event.Nonce, event.SigningAddress)
	tf.latestEvent = "TransactionDispatchedEvent"
	tf.status = "dispatched"
	tf.dispatched = true
}

func (tf *transactionFlow) applyTransactionPreparedEvent(ctx context.Context, _ *ptmgrtypes.TransactionPreparedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionPreparedEvent transactionID:%s ", tf.transaction.ID.String())
	tf.latestEvent = "TransactionPreparedEvent"
	tf.status = "prepared"
	tf.prepared = true
}

func (tf *transactionFlow) applyTransactionConfirmedEvent(ctx context.Context, event *ptmgrtypes.TransactionConfirmedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionConfirmedEvent transactionID:%s contractAddress: %s", tf.transaction.ID.String(), event.ContractAddress)
	tf.latestEvent = "TransactionConfirmedEvent"
	tf.status = "confirmed"
	tf.finalizeRequired = true
}

func (tf *transactionFlow) applyTransactionRevertedEvent(ctx context.Context, _ *ptmgrtypes.TransactionRevertedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionRevertedEvent transactionID:%s", tf.transaction.ID.String())
	tf.latestEvent = "TransactionRevertedEvent"
	tf.status = "reverted"
}

func (tf *transactionFlow) applyTransactionDelegationAcknowledgedEvent(ctx context.Context, event *ptmgrtypes.TransactionDelegationAcknowledgedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionDelegationAcknowledgedEvent transactionID:%s", tf.transaction.ID.String())
	if event.DelegationRequestID != tf.pendingDelegationRequestID {
		log.L(ctx).Warnf("Received delegation acknowledgment for unknown request %s", event.DelegationRequestID)
		return
	}
	tf.latestEvent = "TransactionDelegationAcknowledgedEvent"
	tf.status = "delegated"
	tf.delegated = true
	tf.delegatePending = false
	if tf.delegateRequestTimer != nil {
		tf.delegateRequestTimer.Stop()
	}
	tf.delegateRequestTimer = nil
}

func (tf *transactionFlow) applyResolveVerifierResponseEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierResponseEvent) {
	log.L(ctx).Debug("applyResolveVerifierResponseEvent")
	tf.latestEvent = "ResolveVerifierResponseEvent"

	if tf.transaction.PreAssembly.Verifiers == nil {
		tf.transaction.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0, len(tf.transaction.PreAssembly.RequiredVerifiers))
	}
	// assuming that the order of resolved verifiers in .PreAssembly.Verifiers does not need to match the order of .PreAssembly.RequiredVerifiers
	tf.transaction.PreAssembly.Verifiers = append(tf.transaction.PreAssembly.Verifiers, &prototk.ResolvedVerifier{
		Lookup:       *event.Lookup,
		Algorithm:    *event.Algorithm,
		Verifier:     *event.Verifier,
		VerifierType: *event.VerifierType,
	})

}

func (tf *transactionFlow) applyResolveVerifierErrorEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierErrorEvent) {
	tf.latestEvent = "ResolveVerifierErrorEvent"
	log.L(ctx).Errorf("Failed to resolve verifier %s: %s", *event.Lookup, *event.ErrorMessage)
	//it is possible that this identity was valid when the transaction was assembled but is no longer valid
	// all we can do it try to re-assemble the transaction
	// TODO we might have other resolver verifieres in progress.  Need to make sure that when they are received, we only apply them if they
	// happen to match the requirements new assembled transaction and if that is still nil, then discard them
	tf.transaction.PostAssembly = nil
}

func (tf *transactionFlow) applyTransactionFinalizedEvent(ctx context.Context, _ *ptmgrtypes.TransactionFinalizedEvent) {
	log.L(ctx).Debugf("transactionFlow:applyTransactionFinalizedEvent transactionID:%s", tf.transaction.ID.String())
	tf.latestEvent = "TransactionFinalizedEvent"
	tf.complete = true
	log.L(ctx).Debug("HandleTransactionFinalizedEvent")
}

func (tf *transactionFlow) applyTransactionFinalizeError(ctx context.Context, event *ptmgrtypes.TransactionFinalizeError) {
	log.L(ctx).Errorf("applyTransactionFinalizeError transaction %s: %s", tf.transaction.ID, event.ErrorMessage)

	tf.latestEvent = "TransactionFinalizeError"
	tf.finalizeRequired = true
	tf.finalizePending = false
}

func (tf *transactionFlow) applyTransactionNudgeEvent(ctx context.Context, _ *ptmgrtypes.TransactionNudgeEvent) {
	log.L(ctx).Infof("applyTransactionNudgeEvent transaction %s", tf.transaction.ID)
	tf.latestEvent = "TransactionNudgeEvent"

	//nothing really changes here, we just trigger a re-evaluation of the transaction state and next actions

}

func (tf *transactionFlow) applyDelegationForInFlightEvent(ctx context.Context, event *ptmgrtypes.DelegationForInFlightEvent) {
	log.L(ctx).Infof("applyDelegationForInFlightEvent transaction %s blockHeight=%d", tf.transaction.ID, event.BlockHeight)
	tf.latestEvent = "DelegationForInFlightEvent"
	if tf.delegatePending && event.BlockHeight > tf.delegateRequestBlockHeight {
		tf.status = "new"
		tf.delegated = false
		tf.delegatePending = false
		if tf.delegateRequestTimer != nil {
			tf.delegateRequestTimer.Stop()
		}
		tf.delegateRequestTimer = nil
		log.L(ctx).Infof("delegation cancelled for transaction %s", tf.transaction.ID)
	}

}
