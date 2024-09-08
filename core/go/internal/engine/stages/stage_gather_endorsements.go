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
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	engineProto "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/core/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type GatherEndorsementsStage struct {
	sequencer enginespi.Sequencer
}

func NewGatherEndorsementsStage(sequencer enginespi.Sequencer) *GatherEndorsementsStage {
	return &GatherEndorsementsStage{
		sequencer: sequencer,
	}
}

func (as *GatherEndorsementsStage) Name() string {
	return "gather_endorsements"
}

func (as *GatherEndorsementsStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq {

	return nil
}

type endorsementResponse struct {
	revertReason *string
	endorsement  *prototk.AttestationResult
}
type endorsementActionResult struct {
	endorsementResponses []*endorsementResponse
}

func hasOutstandingEndorsementRequests(tx *components.PrivateTransaction) bool {
	outstandingEndorsementRequests := false
out:
	for _, attRequest := range tx.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			found := false
			for _, endorsement := range tx.PostAssembly.Endorsements {
				if endorsement.Name == attRequest.Name {
					found = true
					break
				}
			}
			if !found {
				outstandingEndorsementRequests = true
				// no point checking any further, we have at least one outstanding endorsement request
				break out
			}
		}
	}
	return outstandingEndorsementRequests
}

func (as *GatherEndorsementsStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	tx := tsg.HACKGetPrivateTx()

	unprocessedStageEvents = []*enginespi.StageEvent{}
	nextStep = enginespi.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == as.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case *endorsementActionResult:
					// process any enodrsements or signatures that have been gathered
					log.L(ctx).Debugf("Processing %d endorsements for transaction %s", len(v.endorsementResponses), tx.ID.String())
					for _, er := range v.endorsementResponses {
						as.processEndorsementResponse(ctx, tx, er)
					}

				case *enginespi.TransactionDispatched:
					if isEndorsed(tx) {
						tx.Signer = "TODO"
						nextStep = enginespi.NextStepNewStage
					} else {
						//TODO this is an error, we should never have gotten here without endorsements
						log.L(ctx).Errorf("Transaction dispatched without endorsements")
					}
				case *engineProto.EndorsementResponse:
					log.L(ctx).Debugf("Processing remote endorsement for transaction %s", tx.ID.String())
					var revertReason *string
					if v.GetRevertReason() != "" {
						revertReason = confutil.P(v.GetRevertReason())
					}
					endorsement := &prototk.AttestationResult{}
					err := v.GetEndorsement().UnmarshalTo(endorsement)
					if err != nil {
						// TODO this is only temproary until we stop using anypb in EndorsementResponse
						log.L(ctx).Errorf("Wrong type received in EndorsementResponse")
						break
					}

					as.processEndorsementResponse(ctx, tx, &endorsementResponse{
						revertReason: revertReason,
						endorsement:  endorsement,
					})
				}
			}
			//TODO: panic error, retry when data is nil?

		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}

func (as *GatherEndorsementsStage) processEndorsementResponse(ctx context.Context, tx *components.PrivateTransaction, er *endorsementResponse) {
	if er.revertReason != nil {
		log.L(ctx).Infof("Endorsement for transaction %s was rejected: %s", tx.ID.String(), *er.revertReason)
		//TODO
	} else {
		log.L(ctx).Infof("Adding endorsement to transaction %s", tx.ID.String())
		tx.PostAssembly.Endorsements = append(tx.PostAssembly.Endorsements, er.endorsement)
		if er.endorsement.Constraints != nil {
			for _, constraint := range er.endorsement.Constraints {
				switch constraint {
				case prototk.AttestationResult_ENDORSER_MUST_SUBMIT:
					//TODO endorser must submit?
					//TODO other constraints

				default:
					log.L(ctx).Errorf("Unsupported constraint: %s", constraint)
				}
			}
		}
		if !hasOutstandingEndorsementRequests(tx) {
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
	}
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

func (as *GatherEndorsementsStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()
	// any asembled transactions are in this stage until they are dispatched
	return isAssembled(tx) && !isDispatched(tx) && !hasOutstandingSignatureRequests(tx) && hasOutstandingEndorsementRequests(tx)
}

func (as *GatherEndorsementsStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	tx := tsg.HACKGetPrivateTx()
	log.L(ctx).Debugf("GatherEndorsementsStage.PerformAction tx: %s", tx.ID.String())

	if tx.PostAssembly == nil {
		log.L(ctx).Errorf("PostAssembly is nil. Should never have reached this stage without a PostAssembly")
		return nil, i18n.NewError(ctx, msgs.MsgEngineInternalError, "")
	}

	err := as.sequencer.AssignTransaction(ctx, tx.ID.String())
	if err != nil {
		log.L(ctx).Errorf("Failed to assign transaction to sequencer: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
	}
	endorsementActionResult := &endorsementActionResult{}

	attPlan := tx.PostAssembly.AttestationPlan
	attResults := tx.PostAssembly.Endorsements
	for _, attRequest := range attPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_SIGN:
			// no op. Signatures are gathered in the GatherSignaturesStage
		case prototk.AttestationType_ENDORSE:
			//TODO not sure this is the best way to check toBeComplete - take a closer look and think about this
			toBeComplete := true
			for _, ar := range attResults {
				if ar.GetAttestationType().Type() == attRequest.GetAttestationType().Type() {
					toBeComplete = false
					break
				}
			}
			if toBeComplete {

				for _, party := range attRequest.GetParties() {

					message := enginespi.StageEvent{
						Stage:           as.Name(),
						Data:            attRequest.GetPayload(),
						ContractAddress: tx.Inputs.Domain,
						TxID:            tx.ID.String(),
					}
					messageBytes, err := json.Marshal(message)
					if err != nil {
						//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
						log.L(ctx).Errorf("Failed to marshal message payload: %s", err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
					}

					partyLocator := types.PrivateIdentityLocator(party)
					partyNode, err := partyLocator.Node(ctx, true)
					if err != nil {
						log.L(ctx).Errorf("Failed to get node name from locator %s: %s", party, err)
						return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
					}

					if sfs.IdentityResolver().IsCurrentNode(partyNode) || partyNode == "" {
						// This is a local party, so we can endorse it directly
						endorsement, revertReason, err := sfs.EndorsementGatherer().GatherEndorsement(ctx, tx, party, attRequest)
						if err != nil {
							log.L(ctx).Errorf("Failed to gather endorsement for party %s: %s", party, err)
							//TODO specific error message
							return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
						}
						endorsementActionResult.endorsementResponses = append(endorsementActionResult.endorsementResponses, &endorsementResponse{
							revertReason: revertReason,
							endorsement:  endorsement,
						})

					} else {
						err = sfs.TransportManager().Send(ctx, &components.TransportMessage{
							MessageType: "endorsementRequest",
							Destination: types.PrivateIdentityLocator(party),
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
		case prototk.AttestationType_GENERATE_PROOF:
			errorMessage := "AttestationType_GENERATE_PROOF is not implemented yet"
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgEngineInternalError, errorMessage)
		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			return nil, i18n.NewError(ctx, msgs.MsgEngineInternalError, errorMessage)
		}

	}
	return endorsementActionResult, nil
}
