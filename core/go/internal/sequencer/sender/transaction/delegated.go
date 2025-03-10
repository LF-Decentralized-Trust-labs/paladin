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

	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

func action_SendDispatchConfirmationResponse(ctx context.Context, txn *Transaction) error {
	txn.messageSender.SendDispatchConfirmationResponse(ctx)
	return nil
}

// Validate that the assemble request matches the current delegate
func validator_AssembleRequestMatches(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	assembleRequestEvent, ok := event.(*AssembleRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *AssembleRequestReceivedEvent, got %T", event)
		return false, nil
	}

	return assembleRequestEvent.Coordinator == txn.currentDelegate, nil

}

func validator_DispatchConfirmationRequestMatchesAssembledDelegation(ctx context.Context, txn *Transaction, event common.Event) (bool, error) {
	dispatchConfirmationRequestEvent, ok := event.(*DispatchConfirmationRequestReceivedEvent)
	if !ok {
		log.L(ctx).Errorf("expected event type *DispatchConfirmationRequestReceivedEvent, got %T", event)
		return false, nil
	}
	txnHash, err := txn.Hash(ctx)
	if err != nil {
		log.L(ctx).Errorf("error hashing transaction: %s", err)
		return false, err
	}
	if dispatchConfirmationRequestEvent.Coordinator != txn.currentDelegate {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Expected coordinator %s, got %s", txn.ID.String(), txn.currentDelegate, dispatchConfirmationRequestEvent.Coordinator)
		return false, nil
	}
	if !txnHash.Equals(dispatchConfirmationRequestEvent.PostAssemblyHash) {
		log.L(ctx).Debugf("DispatchConfirmationRequest invalid for transaction %s.  Transaction hash does not match.", txn.ID.String())
		return false, nil
	}
	return true, nil
}
