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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

type AsyncRequestType int

const (
	ActionSuspend AsyncRequestType = iota
	ActionResume
	ActionCompleted
)

func (ptm *pubTxManager) persistSuspendedFlag(ctx context.Context, from pldtypes.EthAddress, nonce uint64, suspended bool) error {
	log.L(ctx).Infof("Setting suspend status to '%t' for transaction %s:%d", suspended, from, nonce)
	return ptm.p.DB().
		WithContext(ctx).
		Table("public_txns").
		Where(`"from" = ?`, from).
		Where("nonce = ?", nonce).
		UpdateColumn("suspended", suspended).
		Error
}

// TODO: this code needs to stop using from and nonce as the way of identifying a transaction. It didn't get edited
// with the move to delayed nonce assignment, where pubTXID became the primary key for a public transaction instead
// of from and nonce as a composite primary key. This isn't a problem for dispatching a confirm action because a
// confirmed transaction must have a nonce, but it isn't guaranteed to work for suspend and resume. Those actions
// have been copied across but aren't wired up above this level so they aren't obviously broken yet.
func (ptm *pubTxManager) dispatchAction(ctx context.Context, from pldtypes.EthAddress, nonce uint64, action AsyncRequestType) error {
	ptm.inFlightOrchestratorMux.Lock()
	defer ptm.inFlightOrchestratorMux.Unlock()
	inFlightOrchestrator, orchestratorInFlight := ptm.inFlightOrchestrators[from]
	switch action {
	case ActionCompleted:
		// Only need to pass this on if there's an orchestrator in flight for this signing address
		if orchestratorInFlight {
			return inFlightOrchestrator.dispatchAction(ctx, nonce, action)
		}
	case ActionSuspend, ActionResume:
		suspended := false
		if action == ActionSuspend {
			suspended = true
		}
		if !orchestratorInFlight {
			// no in-flight orchestrator for the signing address, it's OK to update the DB directly
			return ptm.persistSuspendedFlag(ctx, from, nonce, suspended)
		}
		// has to be done in the context of the orchestrator
		return inFlightOrchestrator.dispatchAction(ctx, nonce, action)
	}
	return nil
}

func (oc *orchestrator) dispatchAction(ctx context.Context, nonce uint64, action AsyncRequestType) (err error) {
	oc.inFlightTxsMux.Lock()
	defer oc.inFlightTxsMux.Unlock()
	var pending *inFlightTransactionStageController
	for _, inflight := range oc.inFlightTxs {
		if inflight.stateManager.GetNonce() == nonce {
			pending = inflight
			break
		}
	}
	if pending != nil {
		switch action {
		case ActionCompleted:
			_, err = pending.NotifyStatusUpdate(ctx, InFlightStatusConfirmReceived)
		case ActionResume, ActionSuspend:
			// ActionResume...
			suspendedFlag := false
			newStatus := InFlightStatusPending
			// .. or ActionSuspend
			if action == ActionSuspend {
				suspendedFlag = true
				newStatus = InFlightStatusSuspending
			}
			_, _ = pending.NotifyStatusUpdate(ctx, newStatus)
			// Ok we've now got the lock that means we can write to the DB
			// No optimization of this write, as it's a user action from the side of normal processing
			err = oc.persistSuspendedFlag(ctx, oc.signingAddress, nonce, suspendedFlag)
		}
		oc.MarkInFlightTxStale()
	}
	return err
}

func (ptm *pubTxManager) dispatchUpdate(update *transactionUpdate) {
	// updateMux must be locked by the called
	ptm.updates = append(ptm.updates, update)
	ptm.MarkInFlightOrchestratorsStale()
}

func (oc *orchestrator) dispatchUpdate(update *transactionUpdate) {
	oc.updateMux.Lock()
	defer oc.updateMux.Unlock()
	oc.updates = append(oc.updates, update)
	oc.MarkInFlightTxStale()
}
