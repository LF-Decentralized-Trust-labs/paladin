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
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	"github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type GatherSignaturesResult struct {
}

type GatherSignaturesStage struct {
	sequencer enginespi.Sequencer
}

func NewGatherSignaturesStage(sequencer enginespi.Sequencer) *GatherSignaturesStage {
	return &GatherSignaturesStage{
		sequencer: sequencer,
	}
}

func (as *GatherSignaturesStage) Name() string {
	return "gather_signatures"
}

func (as *GatherSignaturesStage) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) *enginespi.TxProcessPreReq {

	return nil
}

type signaturesActionResult struct {
	Signatures []*prototk.AttestationResult
}

func (as *GatherSignaturesStage) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService, stageEvents []*enginespi.StageEvent) (unprocessedStageEvents []*enginespi.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep enginespi.StageProcessNextStep) {
	tx := tsg.HACKGetPrivateTx()

	unprocessedStageEvents = []*enginespi.StageEvent{}
	nextStep = enginespi.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == as.Name() { // the current stage does not care about events from other stages yet (may need to be for interrupts)
			if se.Data != nil {
				switch v := se.Data.(type) {
				case *signaturesActionResult:
					// process any signatures that have been gathered
					log.L(ctx).Debugf("Adding %d signatures to transaction %s", len(v.Signatures), tx.ID.String())
					tx.PostAssembly.Signatures = append(tx.PostAssembly.Signatures, v.Signatures...)
					if !hasOutstandingSignatureRequests(tx) {
						nextStep = enginespi.NextStepNewStage
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

func hasOutstandingSignatureRequests(tx *components.PrivateTransaction) bool {
	outstandingSignatureRequests := false
out:
	for _, attRequest := range tx.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			found := false
			for _, signatures := range tx.PostAssembly.Signatures {
				if signatures.Name == attRequest.Name {
					found = true
					break
				}
			}
			if !found {
				outstandingSignatureRequests = true
				// no point checking any further, we have at least one outstanding signature request
				break out
			}
		}
	}
	return outstandingSignatureRequests
}

func (as *GatherSignaturesStage) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) bool {
	tx := tsg.HACKGetPrivateTx()
	return isAssembled(tx) && !isDispatched(tx) && hasOutstandingSignatureRequests(tx)
}

func (as *GatherSignaturesStage) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs enginespi.StageFoundationService) (actionOutput interface{}, actionTriggerErr error) {
	tx := tsg.HACKGetPrivateTx()
	log.L(ctx).Debugf("GatherSignaturesStage.PerformAction tx: %s", tx.ID.String())

	if tx.PostAssembly == nil {
		log.L(ctx).Errorf("PostAssembly is nil. Should never have reached this stage without a PostAssembly")
		return nil, i18n.NewError(ctx, msgs.MsgEngineInternalError, "")
	}

	if tx.PostAssembly.OutputStatesPotential != nil && tx.PostAssembly.OutputStates == nil {
		//TODO - a bit of a chicken and egg situation here.
		// We need to write the potential states to the domain before we can sign or endorse the transaction
		// however, this is something that we would prefer to defer until we are confident that this transaction will be
		// added to a sequence.
		// Currently, the sequencer waits for endorsement before giving us that confidence so we are forced to write the potential states here.

		//TODO regardless of the chicken and egg situation, this should really be done on the process events thread
		err := sfs.DomainAPI().WritePotentialStates(ctx, tx)
		if err != nil {
			//TODO better error message
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError, errorMessage)
		}
	}

	err := as.sequencer.AssignTransaction(ctx, tx.ID.String())
	if err != nil {
		log.L(ctx).Errorf("Failed to assign transaction to sequencer: %s", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgEngineInternalError)
	}
	signaturesActionResult := &signaturesActionResult{}

	attPlan := tx.PostAssembly.AttestationPlan
	attResults := tx.PostAssembly.Endorsements

	for _, attRequest := range attPlan {
		switch attRequest.AttestationType {
		case prototk.AttestationType_SIGN:
			toBeComplete := true
			for _, ar := range attResults {
				if ar.GetAttestationType().Type() == attRequest.GetAttestationType().Type() {
					toBeComplete = false
					break
				}
			}
			if toBeComplete {

				for _, partyName := range attRequest.Parties {
					keyHandle, verifier, err := sfs.KeyManager().ResolveKey(ctx, partyName, attRequest.Algorithm)
					if err != nil {
						log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s): %s", partyName, attRequest.Algorithm, err)

						return nil, err
					}
					// TODO this could be calling out to a remote signer, should we be doing these in parallel?
					signaturePayload, err := sfs.KeyManager().Sign(ctx, &proto.SignRequest{
						KeyHandle: keyHandle,
						Algorithm: attRequest.Algorithm,
						Payload:   attRequest.Payload,
					})
					if err != nil {
						log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, attRequest.Algorithm, err)
						return nil, err
					}
					signaturesActionResult.Signatures = append(signaturesActionResult.Signatures, &prototk.AttestationResult{
						Name:            attRequest.Name,
						AttestationType: attRequest.AttestationType,
						Verifier: &prototk.ResolvedVerifier{
							Lookup:    partyName,
							Algorithm: attRequest.Algorithm,
							Verifier:  verifier,
						},
						Payload: signaturePayload.Payload,
					})
				}

			}
		}

	}
	return signaturesActionResult, nil
}
