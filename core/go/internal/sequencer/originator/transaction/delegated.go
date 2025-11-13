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
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func action_SendPreDispatchResponse(ctx context.Context, txn *Transaction) error {
	// MRW TODO - sending a dispatch response should be based on some sanity check that we are OK for the coordinator
	// to proceed to dispatch. Not sure if that belongs here, or somewhere else, but at the moment we always reply OK/proceed.
	return txn.transportWriter.SendPreDispatchResponse(ctx, txn.currentDelegate, txn.latestPreDispatchRequestID, txn.PreAssembly.TransactionSpecification)
}

// Validate that the assemble request matches the current delegate
func validator_AssembleRequestMatches(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	assembleRequestEvent, ok := event.(*AssembleRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *AssembleRequestReceivedEvent, got %T", event)
		return false, nil
	}

	log.L(ctx).Debugf("originator transaction validating assemble request - event coordinator %s, TX current delegate = %s", assembleRequestEvent.Coordinator, txn.currentDelegate)
	return assembleRequestEvent.Coordinator == txn.currentDelegate, nil

}

func validator_PreDispatchRequestMatchesAssembledDelegation(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	preDispatchRequestEvent, ok := event.(*PreDispatchRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *PreDispatchRequestReceivedEvent, got %T", event)
		return false, nil
	}
	txnHash, err := txn.Hash(ctx)
	if err != nil {
		log.L(ctx).Errorf("error hashing transaction: %s", err)
		return false, err
	}
	if preDispatchRequestEvent.Coordinator != txn.currentDelegate {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Expected coordinator %s, got %s", txn.ID.String(), txn.currentDelegate, preDispatchRequestEvent.Coordinator)
		return false, nil
	}
	if !txnHash.Equals(preDispatchRequestEvent.PostAssemblyHash) {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Transaction hash does not match.", txn.ID.String())
		return false, nil
	}

	// If validation passed, store the request ID to use in the response
	txn.latestPreDispatchRequestID = preDispatchRequestEvent.RequestID
	return true, nil
}
