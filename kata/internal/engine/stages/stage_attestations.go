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

package stages

import (
	"context"
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type AttestationResult struct {
}

type AttestationStage struct {
	sequencer types.Sequencer
}

func NewAttestationStage(sequencer types.Sequencer) *AttestationStage {
	return &AttestationStage{
		sequencer: sequencer,
	}
}

func (as *AttestationStage) Name() string {
	return "attestation"
}

func (as *AttestationStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {

	return nil
}

func (as *AttestationStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	tx := tsg.HACKGetPrivateTx()

	unprocessedStageEvents = []*types.StageEvent{}
	nextStep = types.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == as.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case *prototk.AttestationResult: // TODO, we need to check the attestation matches the current version
					if txUpdates == nil {
						txUpdates = &transactionstore.TransactionUpdate{}
					}
					tx.PostAssembly.Endorsements = append(tx.PostAssembly.Endorsements, v)

					if len(tx.PostAssembly.Endorsements) < len(tx.PostAssembly.AttestationPlan) {
						log.L(ctx).Infof("Transaction %s has %d endorsements out of %d", tx.ID.String(), len(tx.PostAssembly.Endorsements), len(tx.PostAssembly.AttestationPlan))
					} else {
						//TODO should really call out to the engine to publish this event because it needs
						// to go to other nodes too?

						//Tell the sequencer that this transaction has been endorsed and wait until it publishes a TransactionDispatched event before moving to the next stage
						err := as.sequencer.HandleTransactionEndorsedEvent(ctx, &sequence.TransactionEndorsedEvent{
							TransactionId: tx.ID.String(),
						})
						if err != nil {
							//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
							log.L(ctx).Errorf("Failed to publish transaction endorsed event: %s", err)
						}
					}
				case *types.TransactionDispatched:
					if isEndorsed(tx) {
						tx.Signer = "TODO"
						nextStep = types.NextStepNewStage
					} else {
						//TODO this is an error, we should never have gotten here without endorsements
						log.L(ctx).Errorf("Transaction dispatched without endorsements")
					}
				}
			}
			//TODO: panic error, retry when data is nil?

		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func isAssembled(tx *components.PrivateTransaction) bool {
	return tx.PostAssembly != nil &&
		tx.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_OK
}

func isDispatched(tx *components.PrivateTransaction) bool {
	return tx.Signer != "" // TODO this is the only check we can do for now to see if the transaction has been dispatched
}

func isEndorsed(tx *components.PrivateTransaction) bool {
	return len(tx.PostAssembly.AttestationPlan) > 0 &&
		len(tx.PostAssembly.Endorsements) >= len(tx.PostAssembly.AttestationPlan)
}

func (as *AttestationStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()
	// any asembled transactions are in this stage until they are dispatched
	return isAssembled(tx) && !isDispatched(tx)

}

func (as *AttestationStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	tx := tsg.HACKGetPrivateTx()
	log.L(ctx).Debugf("AttestationStage.PerformAction tx: %s", tx.ID.String())

	if tx.PostAssembly == nil {
		log.L(ctx).Errorf("PostAssembly is nil. Should never have reached this stage without a PostAssembly")
		return nil, i18n.NewError(ctx, msgs.MsgEngineInternalError, "")
	}

	err := as.sequencer.AssignTransaction(ctx, tx.ID.String())
	if err != nil {
		log.L(ctx).Errorf("Failed to assign transaction to sequencer: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
	}

	attPlan := tx.PostAssembly.AttestationPlan
	attResults := tx.PostAssembly.Endorsements
	for _, ap := range attPlan {
		toBeComplete := true
		for _, ar := range attResults {
			if ar.GetAttestationType().Type() == ap.GetAttestationType().Type() {
				toBeComplete = false
				break
			}
		}
		if toBeComplete {
			for _, party := range ap.GetParties() {
				message := types.StageEvent{
					Stage:           as.Name(),
					Data:            ap.GetPayload(),
					ContractAddress: tx.Inputs.Domain,
					TxID:            tx.ID.String(),
				}
				messageBytes, err := json.Marshal(message)
				if err != nil {
					//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
					log.L(ctx).Errorf("Failed to marshal message payload: %s", err)
					return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
				}
				err = sfs.TransportManager().Send(ctx, &components.TransportMessageInput{
					MessageType: "endorsementRequest",
					Destination: party,
					Payload:     messageBytes,
				})
				if err != nil {
					//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
					log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
					return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
				}
			}
		}
	}
	return nil, nil
}
