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
	"fmt"
	"time"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	engineProto "github.com/kaleido-io/paladin/core/pkg/proto/engine"

	"github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewPaladinTransactionProcessor(ctx context.Context, transaction *components.PrivateTransaction, nodeID string, components components.AllComponents, domainAPI components.DomainSmartContract, sequencer ptmgrtypes.Sequencer, publisher ptmgrtypes.Publisher, endorsementGatherer ptmgrtypes.EndorsementGatherer, identityResolver components.IdentityResolver) ptmgrtypes.TxProcessor {
	return &PaladinTxProcessor{
		stageErrorRetry:     10 * time.Second,
		sequencer:           sequencer,
		domainAPI:           domainAPI,
		nodeID:              nodeID,
		components:          components,
		publisher:           publisher,
		endorsementGatherer: endorsementGatherer,
		transaction:         transaction,
		status:              "new",
		identityResolver:    identityResolver,
	}
}

type PaladinTxProcessor struct {
	stageErrorRetry     time.Duration
	components          components.AllComponents
	nodeID              string
	domainAPI           components.DomainSmartContract
	sequencer           ptmgrtypes.Sequencer
	transaction         *components.PrivateTransaction
	publisher           ptmgrtypes.Publisher
	endorsementGatherer ptmgrtypes.EndorsementGatherer
	status              string
	latestEvent         string
	identityResolver    components.IdentityResolver
}

func (ts *PaladinTxProcessor) Init(ctx context.Context) {
}

func (ts *PaladinTxProcessor) GetStatus(ctx context.Context) ptmgrtypes.TxProcessorStatus {
	return ptmgrtypes.TxProcessorActive
}

func (ts *PaladinTxProcessor) GetTxStatus(ctx context.Context) (components.PrivateTxStatus, error) {
	return components.PrivateTxStatus{
		TxID:   ts.transaction.ID.String(),
		Status: ts.status,
	}, nil
}

func (ts *PaladinTxProcessor) HandleTransactionSubmittedEvent(ctx context.Context, event *ptmgrtypes.TransactionSubmittedEvent) {
	ts.latestEvent = "HandleTransactionSubmittedEvent"
	// if the transaction is ready to be assembled, go ahead and do that otherwise, we assume some future event will trigger that
	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	} else {
		err := ts.requestVerifierResolution(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to request verifier resolution: %s", err)
			//TODO error handling and retry
		}
	}
}

func (ts *PaladinTxProcessor) isReadyToAssemble(ctx context.Context) bool {
	log.L(ctx).Debug("PaladinTxProcessor:isReadyToAssemble")

	if ts.transaction.PreAssembly != nil {
		// assume they are all resolved until we find one in RequiredVerifiers that is not in Verifiers
		verifieresResolved := true
		for _, v := range ts.transaction.PreAssembly.RequiredVerifiers {
			thisVerifierIsResolved := false
			for _, rv := range ts.transaction.PreAssembly.Verifiers {
				if rv.Lookup == v.Lookup {
					thisVerifierIsResolved = true
					break
				}
			}
			if !thisVerifierIsResolved {
				verifieresResolved = false
			}
		}
		if verifieresResolved {
			return true
		} else {
			log.L(ctx).Infof("Transaction %s not ready to assemble. Waiting for verifiers to be resolved", ts.transaction.ID.String())
			return false
		}
	}
	log.L(ctx).Infof("Transaction %s not ready to assemble. PreAssembly is nil", ts.transaction.ID.String())
	return false

}

func (ts *PaladinTxProcessor) HandleVerifierResolvedEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierResponseEvent) {
	log.L(ctx).Debug("PaladinTxProcessor:HandleVerifierResolvedEvent")

	ts.latestEvent = "HandleVerifierResolvedEvent"

	// if the transaction is ready to be assembled, go ahead and do that otherwise, we assume some future event will trigger that
	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	} else {
		log.L(ctx).Debug("not ready to assemble")

	}
}

func (ts *PaladinTxProcessor) assembleTransaction(ctx context.Context) {

	log.L(ctx).Debug("PaladinTxProcessor:assembleTransaction")

	if ts.transaction.PostAssembly != nil {
		log.L(ctx).Debug("already assembled")
		return
	}

	//syncronously assemble the transaction then inform the local sequencer and remote nodes for any parties in the
	// privacy group that need to know about the transaction
	// this could be other parties that have potential to attempt to spend the same state(s) as this transaction is assembled to spend
	// or parties that could potentially spend the output states of this transaction
	// or parties that will be needed to endorse or notarize this transaction
	err := ts.domainAPI.AssembleTransaction(ts.endorsementGatherer.DomainContext(), ts.transaction)
	if err != nil {
		log.L(ctx).Errorf("AssembleTransaction failed: %s", err)
		return
		// TODO assembly failed, need to revert the transaction
	}
	ts.status = "assembled"
	// inform the sequencer that the transaction has been assembled
	err = ts.sequencer.HandleTransactionAssembledEvent(ctx, &sequence.TransactionAssembledEvent{
		TransactionId: ts.transaction.ID.String(),
		NodeId:        ts.nodeID,
		InputStateId:  stateIDs(ts.transaction.PostAssembly.InputStates),
		OutputStateId: stateIDs(ts.transaction.PostAssembly.OutputStates),
	})
	if err != nil {
		log.L(ctx).Errorf("HandleTransactionAssembledEvent failed: %s", err)
		panic("todo")
	}

	if ts.transaction.PostAssembly == nil {
		log.L(ctx).Errorf("PostAssembly is nil. Should never have reached this stage without a PostAssembly")
		//return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "")
	}

	if ts.transaction.PostAssembly.OutputStatesPotential != nil && ts.transaction.PostAssembly.OutputStates == nil {
		//TODO - a bit of a chicken and egg situation here.
		// We need to write the potential states to the domain before we can sign or endorse the transaction
		// however, this is something that we would prefer to defer until we are confident that this transaction will be
		// added to a sequence.
		// Currently, the sequencer waits for endorsement before giving us that confidence so we are forced to write the potential states here.

		err := ts.domainAPI.WritePotentialStates(ts.endorsementGatherer.DomainContext(), ts.transaction)
		if err != nil {
			//TODO better error message
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			//return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		}
	}

	err = ts.sequencer.AssignTransaction(ctx, ts.transaction.ID.String())
	if err != nil {
		log.L(ctx).Errorf("Failed to assign transaction to sequencer: %s", err)
		//TODO return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError)
	}

	//start an async process to gather signatures
	// this will emit a TransactionSignedEvent for each signature collected
	if ts.hasOutstandingSignatureRequests() {
		ts.requestSignatures(ctx)
	} else {
		ts.requestEndorsements(ctx)
	}
}

func (ts *PaladinTxProcessor) HandleTransactionAssembledEvent(ctx context.Context, event *ptmgrtypes.TransactionAssembledEvent) {
	//TODO inform the sequencer about a transaction assembled by another node
	ts.latestEvent = "HandleTransactionAssembledEvent"
}

func (ts *PaladinTxProcessor) HandleTransactionSignedEvent(ctx context.Context, event *ptmgrtypes.TransactionSignedEvent) {
	ts.latestEvent = "HandleTransactionSignedEvent"
	log.L(ctx).Debugf("Adding signature to transaction %s", ts.transaction.ID.String())
	ts.transaction.PostAssembly.Signatures = append(ts.transaction.PostAssembly.Signatures, event.AttestationResult)
	if !ts.hasOutstandingSignatureRequests() {
		ts.status = "signed"
		ts.requestEndorsements(ctx)
	}
}

func (ts *PaladinTxProcessor) HandleTransactionEndorsedEvent(ctx context.Context, event *ptmgrtypes.TransactionEndorsedEvent) {
	ts.latestEvent = "HandleTransactionEndorsedEvent"
	if event.RevertReason != nil {
		log.L(ctx).Infof("Endorsement for transaction %s was rejected: %s", ts.transaction.ID.String(), *event.RevertReason)
		//TODO
	} else {
		log.L(ctx).Infof("Adding endorsement to transaction %s", ts.transaction.ID.String())
		ts.transaction.PostAssembly.Endorsements = append(ts.transaction.PostAssembly.Endorsements, event.Endorsement)
		if event.Endorsement.Constraints != nil {
			for _, constraint := range event.Endorsement.Constraints {
				switch constraint {
				case prototk.AttestationResult_ENDORSER_MUST_SUBMIT:
					//TODO endorser must submit?
					//TODO other constraints

				default:
					log.L(ctx).Errorf("Unsupported constraint: %s", constraint)
				}
			}
		}
		if !ts.hasOutstandingEndorsementRequests() {
			ts.status = "endorsed"
			//resolve the signing address here before informing the sequencer about endorsement
			// because endorsement will could trigger a dispatch but
			// a change of signing address could affect the dispatchabiliy of the transaction and/or any transations that depend on it

			if err := ts.domainAPI.ResolveDispatch(ctx, ts.transaction); err != nil {
				log.L(ctx).Errorf("Failed to resolve dispatch for transaction %s: %s", ts.transaction.ID.String(), err)
				//TODO
			}
			err := ts.sequencer.HandleTransactionDispatchResolvedEvent(ctx, &sequence.TransactionDispatchResolvedEvent{
				TransactionId: ts.transaction.ID.String(),
				Signer:        ts.transaction.Signer,
			})
			if err != nil {
				log.L(ctx).Errorf("Failed to publish transaction dispatch resolved event: %s", err)
			}

			//TODO should really call out to the engine to publish this event because it needs
			// to go to other nodes too?

			//Tell the sequencer that this transaction has been endorsed and wait until it publishes a TransactionDispatched event before moving to the next stage
			err = ts.sequencer.HandleTransactionEndorsedEvent(ctx, &sequence.TransactionEndorsedEvent{
				TransactionId: ts.transaction.ID.String(),
			})
			if err != nil {
				//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
				log.L(ctx).Errorf("Failed to publish transaction endorsed event: %s", err)
			}
		}
	}
}

func (ts *PaladinTxProcessor) HandleTransactionDispatchedEvent(ctx context.Context, event *ptmgrtypes.TransactionDispatchedEvent) {
	ts.latestEvent = "HandleTransactionDispatchedEvent"
	ts.status = "dispatched"
}

func (ts *PaladinTxProcessor) HandleTransactionConfirmedEvent(ctx context.Context, event *ptmgrtypes.TransactionConfirmedEvent) {
	ts.latestEvent = "HandleTransactionConfirmedEvent"
	ts.status = "confirmed"
}

func (ts *PaladinTxProcessor) HandleTransactionRevertedEvent(ctx context.Context, event *ptmgrtypes.TransactionRevertedEvent) {
	ts.latestEvent = "HandleTransactionRevertedEvent"
	ts.status = "reverted"
}

func (ts *PaladinTxProcessor) HandleTransactionDelegatedEvent(ctx context.Context, event *ptmgrtypes.TransactionDelegatedEvent) {
	ts.latestEvent = "HandleTransactionDelegatedEvent"
	ts.status = "delegated"
}

func (ts *PaladinTxProcessor) HandleResolveVerifierResponseEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierResponseEvent) {
	log.L(ctx).Debug("HandleResolveVerifierResponseEvent")
	ts.latestEvent = "HandleResolveVerifierResponseEvent"
	if event == nil {
		log.L(ctx).Error("event is nil")
		return
	}
	if event.Lookup == nil {
		log.L(ctx).Error("Lookup is nil")
		return
	}
	if event.Algorithm == nil {
		log.L(ctx).Error("Algorithm is nil")
		return
	}
	if event.Verifier == nil {
		log.L(ctx).Error("Verifier is nil")
		return
	}

	if ts.transaction.PreAssembly.Verifiers == nil {
		ts.transaction.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0, len(ts.transaction.PreAssembly.RequiredVerifiers))
	}
	// assuming that the order of resolved verifiers in .PreAssembly.Verifiers does not need to match the order of .PreAssembly.RequiredVerifiers
	ts.transaction.PreAssembly.Verifiers = append(ts.transaction.PreAssembly.Verifiers, &prototk.ResolvedVerifier{
		Lookup:    *event.Lookup,
		Algorithm: *event.Algorithm,
		Verifier:  *event.Verifier,
	})

	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	}
}

func (ts *PaladinTxProcessor) HandleResolveVerifierErrorEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierErrorEvent) {
	ts.latestEvent = "HandleResolveVerifierErrorEvent"
	log.L(ctx).Errorf("Failed to resolve verifier %s: %s", *event.Lookup, *event.ErrorMessage)
	//TODO - mark error on the transaction so that it gets retried or reverted?
}

func (ts *PaladinTxProcessor) requestSignature(ctx context.Context, attRequest *prototk.AttestationRequest, partyName string) {

	keyHandle, verifier, err := ts.components.KeyManager().ResolveKey(ctx, partyName, attRequest.Algorithm, attRequest.VerifierType)
	if err != nil {
		log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s): %s", partyName, attRequest.Algorithm, err)

		//TODO return nil, err
	}
	// TODO this could be calling out to a remote signer, should we be doing these in parallel?
	signaturePayload, err := ts.components.KeyManager().Sign(ctx, &signerapi.SignRequest{
		KeyHandle:   keyHandle,
		Algorithm:   attRequest.Algorithm,
		Payload:     attRequest.Payload,
		PayloadType: attRequest.PayloadType,
	})
	if err != nil {
		log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, attRequest.Algorithm, err)
		//TODO return nil, err
	}
	log.L(ctx).Debugf("payload: %x signed %x by %s (%s)", attRequest.Payload, signaturePayload.Payload, partyName, verifier)

	if err = ts.publisher.PublishTransactionSignedEvent(ctx,
		ts.transaction.ID.String(),
		&prototk.AttestationResult{
			Name:            attRequest.Name,
			AttestationType: attRequest.AttestationType,
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       partyName,
				Algorithm:    attRequest.Algorithm,
				Verifier:     verifier,
				VerifierType: attRequest.VerifierType,
			},
			Payload:     signaturePayload.Payload,
			PayloadType: &attRequest.PayloadType,
		},
	); err != nil {
		log.L(ctx).Errorf("failed to public event for party %s (verifier=%s,algorithm=%s): %s", partyName, verifier, attRequest.Algorithm, err)
		//TODO return error
	}
}

func (ts *PaladinTxProcessor) requestSignatures(ctx context.Context) {

	attPlan := ts.transaction.PostAssembly.AttestationPlan
	attResults := ts.transaction.PostAssembly.Endorsements

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
					go ts.requestSignature(ctx, attRequest, partyName)
				}
			}
		}
	}
}
func (ts *PaladinTxProcessor) requestEndorsement(ctx context.Context, party string, attRequest *prototk.AttestationRequest) {

	partyLocator := tktypes.PrivateIdentityLocator(party)
	partyNode, err := partyLocator.Node(ctx, true)
	if err != nil {
		log.L(ctx).Errorf("Failed to get node name from locator %s: %s", party, err)
		//TODO return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError)
	}

	if ts.transaction == nil {
		log.L(ctx).Error("Transaction  is nil")
		return
	}
	if ts.transaction.PreAssembly == nil {
		log.L(ctx).Error("PreAssembly is nil")
		return
	}
	if ts.transaction.PostAssembly == nil {
		log.L(ctx).Error("PostAssembly is nil")
		return
	}
	if partyNode == ts.nodeID || partyNode == "" {
		// This is a local party, so we can endorse it directly
		endorsement, revertReason, err := ts.endorsementGatherer.GatherEndorsement(
			ctx,
			ts.transaction.PreAssembly.TransactionSpecification,
			ts.transaction.PreAssembly.Verifiers,
			ts.transaction.PostAssembly.Signatures,
			toEndorsableList(ts.transaction.PostAssembly.InputStates),
			toEndorsableList(ts.transaction.PostAssembly.ReadStates),
			toEndorsableList(ts.transaction.PostAssembly.OutputStates),
			party,
			attRequest)
		if err != nil {
			log.L(ctx).Errorf("Failed to gather endorsement for party %s: %s", party, err)
			return
			//TODO specific error message
			//TODO return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError)
		}
		if err = ts.publisher.PublishTransactionEndorsedEvent(ctx,
			ts.transaction.ID.String(),
			endorsement,
			revertReason,
		); err != nil {
			log.L(ctx).Errorf("Failed to publish endorsement event for party %s: %s", party, err)
			return
			//TODO specific error message
		}

	} else {
		// This is a remote party, so we need to send an endorsement request to the remote node

		attRequstAny, err := anypb.New(attRequest)
		if err != nil {
			log.L(ctx).Error("Error marshalling attestation request", err)
			//TODO return nil, err
		}

		transactionSpecificationAny, err := anypb.New(ts.transaction.PreAssembly.TransactionSpecification)
		if err != nil {
			log.L(ctx).Error("Error marshalling transaction specification", err)
			//TODO return nil, err
		}
		verifiers := make([]*anypb.Any, len(ts.transaction.PreAssembly.Verifiers))
		for i, verifier := range ts.transaction.PreAssembly.Verifiers {
			verifierAny, err := anypb.New(verifier)
			if err != nil {
				log.L(ctx).Error("Error marshalling verifier", err)
				//TODO return nil, err
			}
			verifiers[i] = verifierAny
		}
		signatures := make([]*anypb.Any, len(ts.transaction.PostAssembly.Signatures))
		for i, signature := range ts.transaction.PostAssembly.Signatures {
			signatureAny, err := anypb.New(signature)
			if err != nil {
				log.L(ctx).Error("Error marshalling signature", err)
				//TODO return nil, err
			}
			signatures[i] = signatureAny
		}

		inputStates := make([]*anypb.Any, len(ts.transaction.PostAssembly.InputStates))
		endorseableInputStates := toEndorsableList(ts.transaction.PostAssembly.InputStates)
		for i, inputState := range endorseableInputStates {
			inputStateAny, err := anypb.New(inputState)
			if err != nil {
				log.L(ctx).Error("Error marshalling input state", err)
				//TODO return nil, err
			}
			inputStates[i] = inputStateAny
		}

		outputStates := make([]*anypb.Any, len(ts.transaction.PostAssembly.OutputStates))
		endorseableOutputStates := toEndorsableList(ts.transaction.PostAssembly.OutputStates)
		for i, outputState := range endorseableOutputStates {
			outputStateAny, err := anypb.New(outputState)
			if err != nil {
				log.L(ctx).Error("Error marshalling output state", err)
				//TODO return nil, err
			}
			outputStates[i] = outputStateAny
		}

		endorsementRequest := &engineProto.EndorsementRequest{
			ContractAddress:          ts.transaction.Inputs.To.String(),
			TransactionId:            ts.transaction.ID.String(),
			AttestationRequest:       attRequstAny,
			Party:                    party,
			TransactionSpecification: transactionSpecificationAny,
			Verifiers:                verifiers,
			Signatures:               signatures,
			InputStates:              inputStates,
			OutputStates:             outputStates,
		}

		endorsementRequestBytes, err := proto.Marshal(endorsementRequest)
		if err != nil {
			log.L(ctx).Error("Error marshalling endorsement request", err)
			//TODO return nil, err
		}
		err = ts.components.TransportManager().Send(ctx, &components.TransportMessage{
			MessageType: "EndorsementRequest",
			Destination: tktypes.PrivateIdentityLocator(party),
			ReplyTo:     tktypes.PrivateIdentityLocator(ts.nodeID),
			Payload:     endorsementRequestBytes,
		})
		if err != nil {
			//TODO need better error handling here.  Should we retry? Should we fail the transaction? Should we try sending the other requests?
			log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
			//TODO return nil, i18n.WrapError(ctx, err, msgs.MsgPrivateTxManagerInternalError)
		}
	}
}

func (ts *PaladinTxProcessor) requestEndorsements(ctx context.Context) {
	attPlan := ts.transaction.PostAssembly.AttestationPlan
	attResults := ts.transaction.PostAssembly.Endorsements
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
					ts.requestEndorsement(ctx, party, attRequest)
				}

			}
		case prototk.AttestationType_GENERATE_PROOF:
			errorMessage := "AttestationType_GENERATE_PROOF is not implemented yet"
			log.L(ctx).Error(errorMessage)
			//TODO return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			//TODO return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		}

	}
}

func (ts *PaladinTxProcessor) hasOutstandingSignatureRequests() bool {
	outstandingSignatureRequests := false
out:
	for _, attRequest := range ts.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			found := false
			for _, signatures := range ts.transaction.PostAssembly.Signatures {
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

func (ts *PaladinTxProcessor) hasOutstandingEndorsementRequests() bool {
	outstandingEndorsementRequests := false
out:
	for _, attRequest := range ts.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			found := false
			for _, endorsement := range ts.transaction.PostAssembly.Endorsements {
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

func (ts *PaladinTxProcessor) PrepareTransaction(ctx context.Context) (*components.PrivateTransaction, error) {

	prepError := ts.domainAPI.PrepareTransaction(ts.endorsementGatherer.DomainContext(), ts.transaction)
	if prepError != nil {
		log.L(ctx).Errorf("Error preparing transaction: %s", prepError)
		return nil, prepError
	}
	return ts.transaction, nil
}

func toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

func stateIDs(states []*components.FullState) []string {
	stateIDs := make([]string, 0, len(states))
	for _, state := range states {
		stateIDs = append(stateIDs, state.ID.String())
	}
	return stateIDs
}

func (ts *PaladinTxProcessor) requestVerifierResolution(ctx context.Context) error {

	if ts.transaction.PreAssembly.Verifiers == nil {
		ts.transaction.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0, len(ts.transaction.PreAssembly.RequiredVerifiers))
	}
	for _, v := range ts.transaction.PreAssembly.RequiredVerifiers {
		ts.identityResolver.ResolveVerifierAsync(
			ctx,
			v.Lookup,
			v.Algorithm,
			v.VerifierType,
			func(ctx context.Context, verifier string) {
				//response event needs to be handled by the orchestrator so that the dispatch to a handling thread is done in fairness to all other in flight transactions
				ts.publisher.PublishResolveVerifierResponseEvent(ctx, ts.transaction.ID.String(), v.Lookup, v.Algorithm, verifier)
			},
			func(ctx context.Context, err error) {
				ts.publisher.PublishResolveVerifierErrorEvent(ctx, ts.transaction.ID.String(), v.Lookup, v.Algorithm, err.Error())
			},
		)
	}
	return nil

}
