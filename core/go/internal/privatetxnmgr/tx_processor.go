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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"
	engineProto "github.com/kaleido-io/paladin/core/pkg/proto/engine"

	"github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewPaladinTransactionProcessor(ctx context.Context, transaction *components.PrivateTransaction, nodeID string, components components.AllComponents, domainAPI components.DomainSmartContract, sequencer ptmgrtypes.Sequencer, publisher ptmgrtypes.Publisher, endorsementGatherer ptmgrtypes.EndorsementGatherer, identityResolver components.IdentityResolver, syncPoints syncpoints.SyncPoints, transportWriter ptmgrtypes.TransportWriter) ptmgrtypes.TxProcessor {
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
		syncPoints:          syncPoints,
		transportWriter:     transportWriter,
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
	latestError         string
	identityResolver    components.IdentityResolver
	syncPoints          syncpoints.SyncPoints
	transportWriter     ptmgrtypes.TransportWriter
}

func (ts *PaladinTxProcessor) Init(ctx context.Context) {
}

func (ts *PaladinTxProcessor) GetStatus(ctx context.Context) ptmgrtypes.TxProcessorStatus {
	return ptmgrtypes.TxProcessorActive
}

func (ts *PaladinTxProcessor) GetTxStatus(ctx context.Context) (components.PrivateTxStatus, error) {
	return components.PrivateTxStatus{
		TxID:        ts.transaction.ID.String(),
		Status:      ts.status,
		LatestEvent: ts.latestEvent,
		LatestError: ts.latestError,
	}, nil
}

func (ts *PaladinTxProcessor) HandleTransactionSubmittedEvent(ctx context.Context, event *ptmgrtypes.TransactionSubmittedEvent) error {
	log.L(ctx).Debug("PaladinTxProcessor:HandleTransactionSubmittedEvent")

	ts.latestEvent = "TransactionSubmittedEvent"
	// if the transaction is ready to be assembled, go ahead and do that otherwise, we assume some future event will trigger that
	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	} else {
		ts.requestVerifierResolution(ctx)
	}
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionSwappedInEvent(ctx context.Context, event *ptmgrtypes.TransactionSwappedInEvent) error {
	log.L(ctx).Debug("PaladinTxProcessor:HandleTransactionSwappedInEvent")

	ts.latestEvent = "TransactionSwappedInEvent"
	// if the transaction is ready to be assembled, go ahead and do that otherwise, we assume some future event will trigger that
	if ts.transaction.PostAssembly == nil {
		log.L(ctx).Debug("no assembled yet")
		if ts.isReadyToAssemble(ctx) {
			ts.assembleTransaction(ctx)
		} else {
			ts.requestVerifierResolution(ctx)
		}
	} else {
		log.L(ctx).Debug("already assembled")
		ts.commenceCoordination(ctx)
	}

	return nil
}

func (ts *PaladinTxProcessor) isReadyToAssemble(ctx context.Context) bool {
	log.L(ctx).Debug("PaladinTxProcessor:isReadyToAssemble")

	if ts.transaction.PreAssembly != nil {
		// assume they are all resolved until we find one in RequiredVerifiers that is not in Verifiers
		verifiersResolved := true
		for _, v := range ts.transaction.PreAssembly.RequiredVerifiers {
			thisVerifierIsResolved := false
			for _, rv := range ts.transaction.PreAssembly.Verifiers {
				if rv.Lookup == v.Lookup {
					thisVerifierIsResolved = true
					break
				}
			}
			if !thisVerifierIsResolved {
				verifiersResolved = false
			}
		}
		if verifiersResolved {
			return true
		} else {
			log.L(ctx).Infof("Transaction %s not ready to assemble. Waiting for verifiers to be resolved", ts.transaction.ID.String())
			return false
		}
	}
	log.L(ctx).Infof("Transaction %s not ready to assemble. PreAssembly is nil", ts.transaction.ID.String())
	return false
}

func (ts *PaladinTxProcessor) revertTransaction(ctx context.Context, revertReason string) {
	log.L(ctx).Errorf("Reverting transaction %s: %s", ts.transaction.ID.String(), revertReason)
	//update the transaction as reverted and flush that to the txmgr database
	// so that the user can see that it is reverted and so that we stop retrying to assemble and endorse it

	ts.syncPoints.QueueTransactionFinalize(
		ctx,
		ts.domainAPI.Address(),
		ts.transaction.ID,
		revertReason,
		func(ctx context.Context) {
			//we are not on the main event loop thread so can't update in memory state here.
			// need to go back into the event loop
			log.L(ctx).Infof("Transaction %s finalize committed", ts.transaction.ID.String())
			go ts.publisher.PublishTransactionFinalizedEvent(ctx, ts.transaction.ID.String())
		},
		func(ctx context.Context, rollbackErr error) {
			//we are not on the main event loop thread so can't update in memory state here.
			// need to go back into the event loop
			log.L(ctx).Errorf("Transaction %s finalize rolled back: %s", ts.transaction.ID.String(), rollbackErr)
			go ts.publisher.PublishTransactionFinalizeError(ctx, ts.transaction.ID.String(), revertReason, rollbackErr)
		},
	)
}

func (ts *PaladinTxProcessor) HandleVerifierResolvedEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierResponseEvent) error {
	log.L(ctx).Debug("PaladinTxProcessor:HandleVerifierResolvedEvent")

	ts.latestEvent = "VerifierResolvedEvent"

	// if the transaction is ready to be assembled, go ahead and do that otherwise, we assume some future event will trigger that
	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	} else {
		log.L(ctx).Debug("not ready to assemble")
	}
	return nil
}

func (ts *PaladinTxProcessor) reassembleTransaction(ctx context.Context) {
	log.L(ctx).Debug("PaladinTxProcessor:reassembleTransaction")
	ts.transaction.PostAssembly = nil

	//It will get re added once it has been re-assembled
	ts.sequencer.RemoveTransaction(ctx, ts.transaction.ID.String())

	ts.assembleTransaction(ctx)
}

func (ts *PaladinTxProcessor) assembleTransaction(ctx context.Context) {

	log.L(ctx).Debug("PaladinTxProcessor:assembleTransaction")

	if ts.transaction.PostAssembly != nil {
		log.L(ctx).Debug("already assembled")
		return
	}

	//synchronously assemble the transaction then inform the local sequencer and remote nodes for any parties in the
	// privacy group that need to know about the transaction
	// this could be other parties that have potential to attempt to spend the same state(s) as this transaction is assembled to spend
	// or parties that could potentially spend the output states of this transaction
	// or parties that will be needed to endorse or notarize this transaction
	err := ts.domainAPI.AssembleTransaction(ts.endorsementGatherer.DomainContext(), ts.transaction)
	if err != nil {
		log.L(ctx).Errorf("AssembleTransaction failed: %s", err)
		ts.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerAssembleError), err.Error()))
		return
	}
	if ts.transaction.PostAssembly == nil {
		// This is most likely a programming error in the domain
		log.L(ctx).Errorf("PostAssembly is nil. Should never have reached this stage without a PostAssembly")
		ts.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), "AssembleTransaction returned nil PostAssembly"))
		return
	}
	if ts.transaction.PostAssembly.AssemblyResult == prototk.AssembleTransactionResponse_REVERT {
		// Not sure if any domains actually use this but it is a valid response to indicate failure
		log.L(ctx).Errorf("AssemblyResult is AssembleTransactionResponse_REVERT")
		ts.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerAssembleRevert)))
		return
	}
	ts.status = "assembled"
	if ts.transaction.PostAssembly.Signatures == nil {
		ts.transaction.PostAssembly.Signatures = make([]*prototk.AttestationResult, 0)
	}

	if ts.transaction.PostAssembly.AttestationPlan != nil {
		numEndorsers := 0
		endorser := "" // will only be used if there is only one
		for _, attRequest := range ts.transaction.PostAssembly.AttestationPlan {
			if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
				numEndorsers = numEndorsers + len(attRequest.Parties)
				endorser = attRequest.Parties[0]
			}
		}
		//in the special case of a single endorsers, we delegate to that endorser
		// NOTE: this is a bit of an assumption that this is the best course of action here
		// at this moment in time, it is a certainly that this means we are in the noto domain and
		// that single endorser is the notary and all transactions will be delegated there for endorsement
		// and dispatch to base ledger so we might as well delegate the coordination to it so that
		// it can maximize the optimistic spending of pending states

		if numEndorsers == 1 {
			endorserNode, err := tktypes.PrivateIdentityLocator(endorser).Node(ctx, true)
			if err != nil {
				log.L(ctx).Errorf("Failed to get node name from locator %s: %s", ts.transaction.PostAssembly.AttestationPlan[0].Parties[0], err)
				ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
				return
			}
			if endorserNode != ts.nodeID && endorserNode != "" {
				// TODO persist the delegation and send the request on the callback
				ts.status = "delegating"
				// TODO update to "delegated" once the ack has been received
				err := ts.transportWriter.SendDelegationRequest(
					ctx,
					uuid.New().String(),
					endorserNode,
					ts.transaction,
				)
				if err != nil {
					ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
				}
				return
			}
		}
	}
	//we haven't delegated, so we should commence to coordinate the flow here
	ts.commenceCoordination(ctx)
}

// we have decided to coordinate the endorsement flow and dispatch of this transaction locally
// either because it was submitted locally and we decided not to delegate or because it was delegated to us
func (ts *PaladinTxProcessor) commenceCoordination(ctx context.Context) {

	// inform the sequencer that the transaction has been assembled
	ts.sequencer.HandleTransactionAssembledEvent(ctx, &sequence.TransactionAssembledEvent{
		TransactionId: ts.transaction.ID.String(),
		NodeId:        ts.nodeID,
		InputStateId:  stateIDs(ts.transaction.PostAssembly.InputStates),
		OutputStateId: stateIDs(ts.transaction.PostAssembly.OutputStates),
	})

	if ts.transaction.PostAssembly.OutputStatesPotential != nil && ts.transaction.PostAssembly.OutputStates == nil {
		//TODO - a bit of a chicken and egg situation here.
		// We need to write the potential states to the domain before we can sign or endorse the transaction
		// however, this is something that we would prefer to defer until we are confident that this transaction will be
		// added to a sequence.
		// Currently, the sequencer waits for endorsement before giving us that confidence so we are forced to write the potential states here.

		err := ts.domainAPI.WritePotentialStates(ts.endorsementGatherer.DomainContext(), ts.transaction)
		if err != nil {
			//Any error from WritePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which ist most likely a programming error in the domain or the domain manager or privateTxManager
			// not much we can do other than revert the transaction with an internal error
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			ts.revertTransaction(ctx, i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage))
			return
		}
	}

	ts.sequencer.AssignTransaction(ctx, ts.transaction.ID.String())

	//start an async process to gather signatures
	// this will emit a TransactionSignedEvent for each signature collected
	if ts.hasOutstandingSignatureRequests() {
		ts.requestSignatures(ctx)
	} else {
		ts.requestEndorsements(ctx)
	}
}

func (ts *PaladinTxProcessor) HandleTransactionAssembledEvent(ctx context.Context, event *ptmgrtypes.TransactionAssembledEvent) error {
	//TODO inform the sequencer about a transaction assembled by another node
	ts.latestEvent = "TransactionAssembledEvent"
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionSignedEvent(ctx context.Context, event *ptmgrtypes.TransactionSignedEvent) error {
	ts.latestEvent = "TransactionSignedEvent"
	log.L(ctx).Debugf("Adding signature to transaction %s", ts.transaction.ID.String())
	ts.transaction.PostAssembly.Signatures = append(ts.transaction.PostAssembly.Signatures, event.AttestationResult)
	if !ts.hasOutstandingSignatureRequests() {
		ts.status = "signed"
		ts.requestEndorsements(ctx)
	}
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionEndorsedEvent(ctx context.Context, event *ptmgrtypes.TransactionEndorsedEvent) error {
	ts.latestEvent = "TransactionEndorsedEvent"
	if event.RevertReason != nil {
		log.L(ctx).Infof("Endorsement for transaction %s was rejected: %s", ts.transaction.ID.String(), *event.RevertReason)
		// endorsement errors trigger a re-assemble
		// if the reason for the endorsement error is a change of state of the universe since the transaction was assembled, then the re-assemble may fail and cause the transaction to be reverted
		// on the other hand, the re-assemble may result in an endorsable version of the transaction.
		// either way, we trigger the re-assembly and hope for the best
		ts.reassembleTransaction(ctx)
		return nil
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
		hasOutstandingEndorsementRequests, err := ts.hasOutstandingEndorsementRequests(ctx)
		if err != nil {
			ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), err.Error())
			return err
		}
		if !hasOutstandingEndorsementRequests {
			ts.status = "endorsed"
			//resolve the signing address here before informing the sequencer about endorsement
			// because endorsement will could trigger a dispatch but
			// a change of signing address could affect the dispatchabiliy of the transaction and/or any transactions that depend on it

			err := ts.domainAPI.ResolveDispatch(ctx, ts.transaction)
			if err != nil {
				log.L(ctx).Errorf("Failed to resolve dispatch for transaction %s: %s", ts.transaction.ID.String(), err)
				ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerResolveDispatchError), err.Error())
				return err
			}

			err = ts.sequencer.HandleTransactionDispatchResolvedEvent(ctx, &sequence.TransactionDispatchResolvedEvent{
				TransactionId: ts.transaction.ID.String(),
				Signer:        ts.transaction.Signer,
			})
			if err != nil {
				errorMessage := fmt.Sprintf("Failed to publish transaction dispatch resolved event: %s", err)
				log.L(ctx).Error(errorMessage)
				ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage)
				return err
			}

			//Tell the sequencer that this transaction has been endorsed and wait until it publishes a TransactionDispatched event before moving to the next stage
			err = ts.sequencer.HandleTransactionEndorsedEvent(ctx, &sequence.TransactionEndorsedEvent{
				TransactionId: ts.transaction.ID.String(),
			})
			if err != nil {
				errorMessage := fmt.Sprintf("Failed to publish transaction endorsed event: %s", err)
				log.L(ctx).Error(errorMessage)
				ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage)
				return err
			}
		}
	}
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionDispatchedEvent(ctx context.Context, event *ptmgrtypes.TransactionDispatchedEvent) error {
	ts.latestEvent = "TransactionDispatchedEvent"
	ts.status = "dispatched"
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionConfirmedEvent(ctx context.Context, event *ptmgrtypes.TransactionConfirmedEvent) error {
	ts.latestEvent = "TransactionConfirmedEvent"
	ts.status = "confirmed"
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionRevertedEvent(ctx context.Context, event *ptmgrtypes.TransactionRevertedEvent) error {
	ts.latestEvent = "TransactionRevertedEvent"
	ts.status = "reverted"
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionDelegatedEvent(ctx context.Context, event *ptmgrtypes.TransactionDelegatedEvent) error {
	ts.latestEvent = "TransactionDelegatedEvent"
	ts.status = "delegated"
	return nil
}

func (ts *PaladinTxProcessor) HandleResolveVerifierResponseEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierResponseEvent) error {
	log.L(ctx).Debug("HandleResolveVerifierResponseEvent")
	ts.latestEvent = "ResolveVerifierResponseEvent"

	if event.Lookup == nil {
		log.L(ctx).Error("Lookup is nil")
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Lookup")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Lookup")
	}
	if event.Algorithm == nil {
		log.L(ctx).Error("Algorithm is nil")
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Algorithm")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Algorithm")
	}
	if event.Verifier == nil {
		log.L(ctx).Error("Verifier is nil")
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Verifier")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Verifier")
	}

	if ts.transaction.PreAssembly.Verifiers == nil {
		ts.transaction.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, 0, len(ts.transaction.PreAssembly.RequiredVerifiers))
	}
	// assuming that the order of resolved verifiers in .PreAssembly.Verifiers does not need to match the order of .PreAssembly.RequiredVerifiers
	ts.transaction.PreAssembly.Verifiers = append(ts.transaction.PreAssembly.Verifiers, &prototk.ResolvedVerifier{
		Lookup:       *event.Lookup,
		Algorithm:    *event.Algorithm,
		Verifier:     *event.Verifier,
		VerifierType: *event.VerifierType,
	})

	if ts.isReadyToAssemble(ctx) {
		ts.assembleTransaction(ctx)
	}
	return nil
}

func (ts *PaladinTxProcessor) HandleResolveVerifierErrorEvent(ctx context.Context, event *ptmgrtypes.ResolveVerifierErrorEvent) error {
	ts.latestEvent = "ResolveVerifierErrorEvent"
	log.L(ctx).Errorf("Failed to resolve verifier %s: %s", *event.Lookup, *event.ErrorMessage)
	//it is possible that this identity was valid when the transaction was assembled but is no longer valid
	// all we can do it try to re-assemble the transaction
	ts.reassembleTransaction(ctx)
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionFinalizedEvent(ctx context.Context, event *ptmgrtypes.TransactionFinalizedEvent) error {
	ts.latestEvent = "TransactionFinalizedEvent"
	log.L(ctx).Debug("HandleTransactionFinalizedEvent")
	return nil
}

func (ts *PaladinTxProcessor) HandleTransactionFinalizeError(ctx context.Context, event *ptmgrtypes.TransactionFinalizeError) error {
	ts.latestEvent = "TransactionFinalizeError"
	log.L(ctx).Errorf("Failed to finalize transaction %s: %s", ts.transaction.ID, event.ErrorMessage)

	//try again
	ts.revertTransaction(ctx, event.ErrorMessage)
	return nil
}

func (ts *PaladinTxProcessor) requestSignature(ctx context.Context, attRequest *prototk.AttestationRequest, partyName string) {

	keyMgr := ts.components.KeyManager()
	unqualifiedLookup, err := tktypes.PrivateIdentityLocator(partyName).Identity(ctx)
	var resolvedKey *pldapi.KeyMappingAndVerifier
	if err == nil {
		resolvedKey, err = keyMgr.ResolveKeyNewDatabaseTX(ctx, unqualifiedLookup, attRequest.Algorithm, attRequest.VerifierType)
	}
	if err != nil {
		log.L(ctx).Errorf("Failed to resolve local signer for %s (algorithm=%s): %s", partyName, attRequest.Algorithm, err)
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerResolveError), partyName, attRequest.Algorithm, err.Error())
		return
	}
	// TODO this could be calling out to a remote signer, should we be doing these in parallel?
	signaturePayload, err := keyMgr.Sign(ctx, resolvedKey, attRequest.PayloadType, attRequest.Payload)
	if err != nil {
		log.L(ctx).Errorf("failed to sign for party %s (verifier=%s,algorithm=%s): %s", partyName, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err)
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerSignError), partyName, resolvedKey.Verifier.Verifier, attRequest.Algorithm, err.Error())
		return
	}
	log.L(ctx).Debugf("payload: %x signed %x by %s (%s)", attRequest.Payload, signaturePayload, partyName, resolvedKey.Verifier.Verifier)

	ts.publisher.PublishTransactionSignedEvent(ctx,
		ts.transaction.ID.String(),
		&prototk.AttestationResult{
			Name:            attRequest.Name,
			AttestationType: attRequest.AttestationType,
			Verifier: &prototk.ResolvedVerifier{
				Lookup:       partyName,
				Algorithm:    attRequest.Algorithm,
				Verifier:     resolvedKey.Verifier.Verifier,
				VerifierType: attRequest.VerifierType,
			},
			Payload:     signaturePayload,
			PayloadType: &attRequest.PayloadType,
		},
	)
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
		ts.publisher.PublishTransactionEndorsedEvent(ctx,
			ts.transaction.ID.String(),
			endorsement,
			revertReason,
		)

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
			Node:        partyNode,
			Component:   PRIVATE_TX_MANAGER_DESTINATION,
			ReplyTo:     ts.nodeID,
			Payload:     endorsementRequestBytes,
		})
		if err != nil {
			log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
			ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), party, err.Error())
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
			ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage)
		default:
			errorMessage := fmt.Sprintf("Unsupported attestation type: %s", attRequest.AttestationType)
			log.L(ctx).Error(errorMessage)
			ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInternalError), errorMessage)
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

func (ts *PaladinTxProcessor) hasOutstandingEndorsementRequests(ctx context.Context) (bool, error) {
	if ts.transaction.PostAssembly == nil || ts.transaction.PreAssembly == nil {
		return false, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, "Transaction not assembled")
	}
	outstandingEndorsementRequests := false
out:
	for _, attRequest := range ts.transaction.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				var verifier string
				for _, v := range ts.transaction.PreAssembly.Verifiers {
					if v.Lookup == party {
						verifier = v.Verifier
						break
					}
				}

				found := false
				for _, endorsement := range ts.transaction.PostAssembly.Endorsements {
					if endorsement.Name == attRequest.Name && endorsement.Verifier.Verifier == verifier {
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
	}
	return outstandingEndorsementRequests, nil
}

func (ts *PaladinTxProcessor) PrepareTransaction(ctx context.Context) (*components.PrivateTransaction, error) {

	prepError := ts.domainAPI.PrepareTransaction(ts.endorsementGatherer.DomainContext(), ts.transaction)
	if prepError != nil {
		log.L(ctx).Errorf("Error preparing transaction: %s", prepError)
		ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerPrepareError), prepError.Error())
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

func (ts *PaladinTxProcessor) requestVerifierResolution(ctx context.Context) {

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
				ts.publisher.PublishResolveVerifierResponseEvent(ctx, ts.transaction.ID.String(), v.Lookup, v.Algorithm, verifier, v.VerifierType)
			},
			func(ctx context.Context, err error) {
				ts.publisher.PublishResolveVerifierErrorEvent(ctx, ts.transaction.ID.String(), v.Lookup, v.Algorithm, err.Error())
			},
		)
	}
}

func (ts *PaladinTxProcessor) GetStateDistributions(ctx context.Context) []*statedistribution.StateDistribution {
	log.L(ctx).Debug("PaladinTxProcessor:GetStateDistributions")

	stateDistributions := make([]*statedistribution.StateDistribution, 0)
	if ts.transaction.PostAssembly == nil {
		log.L(ctx).Error("PostAssembly is nil")
		return stateDistributions
	}
	if ts.transaction.PostAssembly.OutputStates == nil {
		log.L(ctx).Debug("OutputStates is nil")
		return stateDistributions
	}
	for stateIndex, outputState := range ts.transaction.PostAssembly.OutputStates {
		//need the output state for the state ID and need the outputStatePotential for the distribution list
		outputStatePotential := ts.transaction.PostAssembly.OutputStatesPotential[stateIndex]

		for _, party := range outputStatePotential.DistributionList {
			stateDistributions = append(stateDistributions, &statedistribution.StateDistribution{
				ID:              uuid.New().String(),
				StateID:         outputState.ID.String(),
				IdentityLocator: party,
				Domain:          ts.domainAPI.Domain().Name(),
				ContractAddress: ts.transaction.Inputs.To.String(),
				SchemaID:        outputState.Schema.String(),
				StateDataJson:   string(outputState.Data), // the state data json is available on both but we take it
				// from the outputState to make sure it is the same json that was used to generate the hash
			})
		}
	}
	return stateDistributions
}
