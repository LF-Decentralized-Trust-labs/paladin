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
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"golang.org/x/crypto/sha3"
)

type TransactionState string

const (
	TransactionState_Pooled                TransactionState = "TransactionState_Pooled"
	TransactionState_Assembled             TransactionState = "TransactionState_Assembled"
	TransactionState_ConfirmingForDispatch TransactionState = "TransactionState_ConfirmingForDispatch"
	TransactionState_Dispatched            TransactionState = "TransactionState_Dispatched"
	TransactionState_Submitted             TransactionState = "TransactionState_Submitted"
	TransactionState_Rejected              TransactionState = "TransactionState_Rejected"
	TransactionState_ConfirmedSuccess      TransactionState = "TransactionState_ConfirmedSuccess"
	TransactionState_ConfirmedReverted     TransactionState = "TransactionState_ConfirmedReverted"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

// Transaction represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
type Transaction struct {
	*components.PrivateTransaction
	sender               string
	senderIdentity       string
	senderNode           string
	signerAddress        *tktypes.EthAddress
	latestSubmissionHash *tktypes.Bytes32
	nonce                *uint64
	stateMachine         *StateMachine

	//TODO move the fields that are really just fine grained state info.  Move them into the stateMachine struct ( consider separate structs for each concrete state)
	heartbeatIntervalsSinceStateChange int
	pendingAssembleRequest             *common.IdempotentRequest
	pendingEndorsementRequests         map[string]map[string]*common.IdempotentRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	pendingDispatchConfirmationRequest *common.IdempotentRequest
	latestError                        string
	dependencies                       []uuid.UUID //TODO figure out naming of these fields and their relationship with the PrivateTransaction fields
	dependents                         []uuid.UUID
	preAssembleDependents              []uuid.UUID
	previousTransaction                *Transaction
	nextTransaction                    *Transaction

	requestTimeout  common.Duration
	assembleTimeout common.Duration
	errorCount      int
	// Dependencies
	clock              common.Clock
	messageSender      MessageSender
	grapher            Grapher
	stateIntegration   common.StateIntegration
	notifyOfTransition OnStateTransition
}

// TODO think about naming of this compared to the OnTransitionTo func in the state machine
type OnStateTransition func(ctx context.Context, t *Transaction, to, from State) // function to be invoked when transitioning into this state.  Called after transitioning event has been applied and any actions have fired

func NewTransaction(ctx context.Context, sender string, pt *components.PrivateTransaction, messageSender MessageSender, clock common.Clock, stateIntegration common.StateIntegration, requestTimeout, assembleTimeout common.Duration, grapher Grapher, onStateTransition OnStateTransition) (*Transaction, error) {
	senderIdentity, senderNode, err := tktypes.PrivateIdentityLocator(sender).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("Error validating sender %s: %s", sender, err)
		return nil, err
	}
	txn := &Transaction{
		sender:             sender,
		senderIdentity:     senderIdentity,
		senderNode:         senderNode,
		PrivateTransaction: pt,
		messageSender:      messageSender,
		clock:              clock,
		grapher:            grapher,
		requestTimeout:     requestTimeout,
		assembleTimeout:    assembleTimeout,
		stateIntegration:   stateIntegration,
		notifyOfTransition: onStateTransition,
	}
	txn.InitializeStateMachine(State_Initial)
	grapher.Add(context.Background(), txn)
	return txn, nil
}

func (t *Transaction) cleanup(_ context.Context) error {
	return t.grapher.Forget(t.ID)
}

func (t *Transaction) GetSignerAddress() *tktypes.EthAddress {
	return t.signerAddress
}

func (t *Transaction) GetNonce() *uint64 {
	return t.nonce
}

func (t *Transaction) GetState() State {
	return t.stateMachine.currentState
}

func (t *Transaction) GetLatestSubmissionHash() *tktypes.Bytes32 {
	return t.latestSubmissionHash
}

// Hash method of Transaction
func (t *Transaction) Hash(ctx context.Context) (*tktypes.Bytes32, error) {
	if t.PrivateTransaction == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PrivateTransaction")
	}
	if t.PostAssembly == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PostAssembly")
	}

	if len(t.PostAssembly.Signatures) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without at least one Signature")
	}

	hash := sha3.NewLegacyKeccak256()
	for _, signature := range t.PostAssembly.Signatures {
		hash.Write(signature.Payload)
	}
	var h32 tktypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil

}

func (t *Transaction) SetPreviousTransaction(ctx context.Context, previousTransaction *Transaction) {
	//TODO consider moving this to the PreAssembly part of PrivateTransaction and specifying a responsibility of the sender to set this.
	// this is probably part of the decision on whether we expect the sender to include all current inflight transactions in every delegation request.
	t.previousTransaction = previousTransaction
}

func (t *Transaction) SetNextTransaction(ctx context.Context, nextTransaction *Transaction) {
	//TODO consider moving this to the PreAssembly part of PrivateTransaction and specifying a responsibility of the sender to set this.
	// this is probably part of the decision on whether we expect the sender to include all current inflight transactions in every delegation request.
	t.nextTransaction = nextTransaction
}

// SignatureAttestationName is a method of Transaction that returns the name of the attestation in the attestation plan that is a signature
func (t *Transaction) SignatureAttestationName() (string, error) {
	for _, attRequest := range t.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			return attRequest.Name, nil
		}
	}
	return "", nil
}

// TODO would this be more obvious as a state exit/cleanup function?
func (t *Transaction) applyDispatchConfirmation(_ context.Context, requestID uuid.UUID) error {
	t.pendingDispatchConfirmationRequest = nil
	return nil
}

func (t *Transaction) applyEndorsement(ctx context.Context, endorsement *prototk.AttestationResult, requestID uuid.UUID) error {
	pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsement.Name]
	if !ok {
		log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because no pending request found for attestation request name %s", t.ID, endorsement.Verifier.Lookup, endorsement.Name)
		return nil
	}
	if pendingRequest, ok := pendingRequestsForAttRequest[endorsement.Verifier.Lookup]; ok {
		if pendingRequest.IdempotencyKey() == requestID {
			log.L(ctx).Infof("Endorsement received for transaction %s from %s", t.ID, endorsement.Verifier.Lookup)
			delete(t.pendingEndorsementRequests[endorsement.Name], endorsement.Verifier.Lookup)
			t.PostAssembly.Endorsements = append(t.PostAssembly.Endorsements, endorsement)
		} else {
			log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because idempotency key %s does not match expected %s ", t.ID, endorsement.Verifier.Lookup, requestID.String(), pendingRequest.IdempotencyKey().String())
		}
	} else {
		log.L(ctx).Infof("Ignoring endorsement response for transaction %s from %s because no pending request found", t.ID, endorsement.Verifier.Lookup)
	}

	return nil
}

func (t *Transaction) applyPostAssembly(ctx context.Context, postAssembly *components.TransactionPostAssembly) error {

	//TODO the response from the assembler actually contains outputStatesPotential so we need to write them to the store and then add the OutputState ids to the index
	t.PostAssembly = postAssembly
	for _, state := range postAssembly.OutputStates {
		err := t.grapher.AddMinter(ctx, state.ID, t)
		if err != nil {
			msg := fmt.Sprintf("Error adding minter for state %s: %s while applying postAssembly", state.ID, err)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
	}
	return t.calculatePostAssembleDependencies(ctx)
}

func (t *Transaction) Sender() string {
	return t.sender
}

func (t *Transaction) SenderNode() string {
	return t.senderNode
}

func (t *Transaction) SenderIdentity() string {
	return t.senderIdentity
}

func (d *Transaction) IsEndorsed(ctx context.Context) bool {
	return !d.hasUnfulfilledEndorsementRequirements(ctx)
}

func (d *Transaction) OutputStateIDs(_ context.Context) []string {

	//We use the output states here not the OutputStatesPotential because it is not possible for another transaction
	// to spend a state unless it has been written to the state store and at that point we have the state ID
	outputStateIDs := make([]string, len(d.PostAssembly.OutputStates))
	for i, outputState := range d.PostAssembly.OutputStates {
		outputStateIDs[i] = outputState.ID.String()
	}
	return outputStateIDs
}

func (d *Transaction) InputStateIDs(_ context.Context) []string {

	inputStateIDs := make([]string, len(d.PostAssembly.InputStates))
	for i, inputState := range d.PostAssembly.InputStates {
		inputStateIDs[i] = inputState.ID.String()
	}
	return inputStateIDs
}

func (d *Transaction) Txn() *components.PrivateTransaction {
	return d.PrivateTransaction
}

// TODO reorganize into a separate .go ( and separate struct / methods) for EndorsementRequest.
func (d *Transaction) hasUnfulfilledEndorsementRequirements(ctx context.Context) bool {
	return len(d.unfulfilledEndorsementRequirements(ctx)) > 0
}

// TODO rework this.  There are actually 2 things to keep track of a) unfulfilled endorsement requirements.  i.e. there is a thing in the plan and we don't have an attestation for it and b) outstanding requests.  i.e. we have sent a request and not had a response yet
func (d *Transaction) unfulfilledEndorsementRequirements(ctx context.Context) []*endorsementRequirement {
	unfulfilledEndorsementRequirements := make([]*endorsementRequirement, 0)
	if d.PostAssembly == nil {
		log.L(ctx).Debug("PostAssembly is nil so there are no outstanding endorsement requirements")
		return unfulfilledEndorsementRequirements
	}
	for _, attRequest := range d.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				for _, endorsement := range d.PostAssembly.Endorsements {
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType

					log.L(ctx).Infof("endorsement matched=%t: request[name=%s,party=%s,verifierType=%s] endorsement[name=%s,party=%s,verifierType=%s] verifier=%s",
						found,
						attRequest.Name, party, attRequest.VerifierType,
						endorsement.Name, endorsement.Verifier.Lookup, endorsement.Verifier.VerifierType,
						endorsement.Verifier.Verifier,
					)
					if found {
						break
					}
				}
				if !found {
					log.L(ctx).Debugf("endorsement requirement for %s unfulfilled for transaction %s", party, d.ID)
					unfulfilledEndorsementRequirements = append(unfulfilledEndorsementRequirements, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}
	return unfulfilledEndorsementRequirements
}

func (t *Transaction) sendAssembleRequest(ctx context.Context) error {
	//assemble requests have a short and long timeout
	// the short timeout is for toleration of unreliable networks whereby the action is to retry the request with the same idempotency key
	// the long timeout is to prevent an unavailable transaction sender/assemble from holding up the entire contract / privacy group given that the assemble step is single threaded
	// the action for the long timeout is to return the transaction to the mempool and let another transaction be selected

	//We Nudge the IdempotentRequest every heartbeat to implement the short retry and the state machine will deal with the long timeout via the guard assembleTimeoutExpired

	t.pendingAssembleRequest = common.NewIdempotentRequest(ctx, t.clock, t.assembleTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
		return t.messageSender.SendAssembleRequest(ctx, t.sender, t.ID, idempotencyKey, t.PreAssembly)
	})
	return t.pendingAssembleRequest.Nudge(ctx)

}

func (t *Transaction) assembleTimeoutExceeded(ctx context.Context) bool {
	if t.pendingAssembleRequest == nil {
		//strange situation to be in if we get to the point of this being nil, should immediately leave the state where we ever ask this question
		// however we go here, the answer to the question is "false" because there is no pending request to timeout but log this as it is a strange situation
		// and might be an indicator of another issue
		log.L(ctx).Infof("assembleTimeoutExceeded called on transaction %s with no pending assemble request", t.ID)
		return false
	}
	return t.clock.HasExpired(t.pendingAssembleRequest.FirstRequestTime(), t.assembleTimeout)

}

func (t *Transaction) isNotReady() bool {
	//test against the list of states that we consider to be past the point of ready as there is more chance of us noticing
	// a failing test if we add new states in the future and forget to update this list
	return t.GetState() != State_Confirmed &&
		t.GetState() != State_Submitted &&
		t.GetState() != State_Dispatched &&
		t.GetState() != State_Ready_For_Dispatch
}

// Function hasDependenciesNotReady checks if the transaction has any dependencies that themselves are not ready for dispatch
func (t *Transaction) hasDependenciesNotReady(ctx context.Context) bool {
	//TODO rethink the name of this function

	//We already calculated the dependencies when we got assembled and there is no way we could have picked up new
	// dependencies without a re-assemble
	// some of them might have been confirmed and removed from our list to avoid a memory leak so this is not necessarily the complete list of dependencies
	// but it should contain all the ones that are not ready for dispatch

	if t.previousTransaction != nil && t.previousTransaction.isNotReady() {
		return true
	}

	dependencies := t.dependencies
	if t.PreAssembly != nil {
		dependencies = append(dependencies, t.PreAssembly.Dependencies...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {
			//assume the dependency has been confirmed and no longer in memory
			//hasUnknownDependencies guard will be used to explicitly ensure the correct thing happens
			continue
		}

		if dependency.isNotReady() {
			return true
		}
	}

	return false
}

func (t *Transaction) isNotAssembled() bool {
	//test against the list of states that we consider to be past the point of assemble as there is more chance of us noticing
	// a failing test if we add new states in the future and forget to update this list

	return t.GetState() != State_Endorsement_Gathering &&
		t.GetState() != State_Confirming_Dispatch &&
		t.GetState() != State_Ready_For_Dispatch &&
		t.GetState() != State_Dispatched &&
		t.GetState() != State_Submitted &&
		t.GetState() != State_Confirmed
}

// Function hasDependenciesNotAssembled checks if the transaction has any dependencies that have not been assembled yet
func (t *Transaction) hasDependenciesNotAssembled(ctx context.Context) bool {

	// we cannot have unassembled dependencies other than those that were provided to us in the PreAssemble or the one we determined as previousTransaction when we initially received an ordered list of delegated transactions.
	if t.previousTransaction != nil && t.previousTransaction.isNotAssembled() {
		return true
	}

	for _, dependencyID := range t.PreAssembly.Dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {
			//assume the dependency has been confirmed and no longer in memory
			//hasUnknownDependencies guard will be used to explicitly ensure the correct thing happens
			continue
		}
		if dependency.isNotAssembled() {
			return true
		}
	}

	return false
}

// Function hasUnknownDependencies checks if the transaction has any dependencies the coordinator does not have in memory.  These might be long gone confirmed to base ledger or maybe the delegation request for them hasn't reached us yet. At this point, we don't know
func (t *Transaction) hasUnknownDependencies(ctx context.Context) bool {

	dependencies := t.dependencies
	if t.PreAssembly != nil {
		dependencies = append(dependencies, t.PreAssembly.Dependencies...)
	}

	for _, dependencyID := range dependencies {
		dependency := t.grapher.TransactionByID(ctx, dependencyID)
		if dependency == nil {

			return true
		}

	}

	//if there are are any dependencies declared, they are all known to the current in memory context ( grapher)
	return false
}

// Function sendEndorsementRequests iterates through the attestation plan and for each endorsement request that has not been fulfilled
// sends an endorsement request to the appropriate party unless there was a recent request (i.e. within the retry threshold)
// it is safe to call this function multiple times and on a frequent basis (e.g. every heartbeat interval while in the endorsement gathering state) as it will not send duplicate requests unless they have timedout
func (t *Transaction) sendEndorsementRequests(ctx context.Context) error {

	if t.pendingEndorsementRequests == nil {
		t.pendingEndorsementRequests = make(map[string]map[string]*common.IdempotentRequest)
	}

	for _, endorsementRequirement := range t.unfulfilledEndorsementRequirements(ctx) {

		pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name]
		if !ok {
			pendingRequestsForAttRequest = make(map[string]*common.IdempotentRequest)
			t.pendingEndorsementRequests[endorsementRequirement.attRequest.Name] = pendingRequestsForAttRequest
		}
		pendingRequest, ok := pendingRequestsForAttRequest[endorsementRequirement.party]
		if !ok {
			pendingRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {
				return t.requestEndorsement(ctx, idempotencyKey, endorsementRequirement.party, endorsementRequirement.attRequest)
			})
			pendingRequestsForAttRequest[endorsementRequirement.party] = pendingRequest
		}
		err := pendingRequest.Nudge(ctx)
		if err != nil {
			log.L(ctx).Errorf("Failed to nudge endorsement request for party %s: %s", endorsementRequirement.party, err)
			t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), endorsementRequirement.party, err.Error())
		}

	}
	return nil
}

func (t *Transaction) sendDispatchConfirmationRequest(ctx context.Context) error {

	if t.pendingDispatchConfirmationRequest == nil {
		hash, err := t.Hash(ctx)
		if err != nil {
			return err
		}
		t.pendingDispatchConfirmationRequest = common.NewIdempotentRequest(ctx, t.clock, t.requestTimeout, func(ctx context.Context, idempotencyKey uuid.UUID) error {

			return t.messageSender.SendDispatchConfirmationRequest(
				ctx,
				t.sender,
				idempotencyKey,
				t.PreAssembly.TransactionSpecification,
				hash,
			)
		})

	}
	return t.pendingDispatchConfirmationRequest.Nudge(ctx)

}

func (t *Transaction) notifyDependentsOfAssembled(ctx context.Context) error {
	//this function is called when the transaction is successfully assembled
	// and we have a duty to inform all the transactions that are ordered behind us
	if t.nextTransaction != nil {
		err := t.nextTransaction.HandleEvent(ctx, &DependencyAssembledEvent{
			event: event{
				TransactionID: t.nextTransaction.ID,
			},
			DependencyID: t.ID,
		})
		if err != nil {
			log.L(ctx).Errorf("Error notifying next transaction %s of assembly of transaction %s: %s", t.nextTransaction.ID, t.ID, err)
			return err
		}
	}

	for _, dependentId := range t.dependents {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			msg := fmt.Sprintf("notifyDependentsOfReadiness: Dependent transaction %s not found in memory", dependentId)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
		err := dependent.HandleEvent(ctx, &DependencyAssembledEvent{
			event: event{
				TransactionID: t.nextTransaction.ID,
			},
			DependencyID: t.ID,
		})
		if err != nil {
			log.L(ctx).Errorf("Error notifying dependent transaction %s of assembly of transaction %s: %s", dependent.ID, t.ID, err)
			return err
		}
	}
	return nil
}

func (t *Transaction) notifyDependentsOfReadiness(ctx context.Context) error {
	//this function is called when the transaction enters the ready for dispatch state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependentId := range t.dependents {
		dependent := t.grapher.TransactionByID(ctx, dependentId)
		if dependent == nil {
			msg := fmt.Sprintf("notifyDependentsOfReadiness: Dependent transaction %s not found in memory", dependentId)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}
		dependent.HandleEvent(ctx, &DependencyReadyEvent{
			event: event{
				TransactionID: dependent.ID,
			},
			DependencyID: t.ID,
		})
	}
	return nil
}

func (t *Transaction) notifyDependentsOfRevert(ctx context.Context) error {
	//this function is called when the transaction enters the reverted state on a revert response from assemble
	// NOTE: at this point, we have not been assembled and therefore are not the minter of any state the only transactions that could possibly be dependent on us are those in the pool from the same sender

	for _, dependentID := range append(t.dependents, t.preAssembleDependents...) {
		dependentTxn := t.grapher.TransactionByID(ctx, dependentID)
		if dependentTxn != nil {
			dependentTxn.HandleEvent(ctx, &DependencyRevertedEvent{
				event: event{
					TransactionID: dependentID,
				},
				DependencyID: t.ID,
			})
		} else {
			//TODO can we Assume that the dependent is no longer in memory and doesn't need to know about this event?  Point to (write) the architecture doc that explains why this is safe

			msg := fmt.Sprintf("notifyDependentsOfRevert: Dependent transaction %s not found in memory", dependentID)
			log.L(ctx).Error(msg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
		}

	}

	return nil
}

func (t *Transaction) requestEndorsement(ctx context.Context, idempotencyKey uuid.UUID, party string, attRequest *prototk.AttestationRequest) error {

	err := t.messageSender.SendEndorsementRequest(
		ctx,
		idempotencyKey,
		party,
		attRequest,
		t.PreAssembly.TransactionSpecification,
		t.PreAssembly.Verifiers,
		t.PostAssembly.Signatures,
		t.PostAssembly.InputStates,
		t.PostAssembly.OutputStates,
		t.PostAssembly.InfoStates,
	)
	if err != nil {
		log.L(ctx).Errorf("Failed to send endorsement request to party %s: %s", party, err)
		t.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerEndorsementRequestError), party, err.Error())
	}
	return err
}

// TODO this is not called but should be called to build the endorsement request?
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

// TODO rename this function because it is not clear that its main purpose is to attach this transaction to the dependency as a dependent
func (t *Transaction) initializeDependencies(ctx context.Context) error {
	if t.PreAssembly == nil {
		msg := fmt.Sprintf("Cannot calculate dependencies for transaction %s without a PreAssembly", t.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	for _, dependencyID := range t.PreAssembly.Dependencies {
		dependencyTxn := t.grapher.TransactionByID(ctx, dependencyID)

		if nil == dependencyTxn {
			//either the dependency has been confirmed and no longer in memory or there was a overtake on the network and we have not received the delegation request for the dependency yet
			// in either case, the guards will stop this transaction from being assembled but will appear in the heartbeat messages so that the sender can take appropriate action (remove the dependency if it is confirmed, resend the dependency delegation request if it is an inflight transaction)

			//This should be relatively rare so worth logging as an info
			log.L(ctx).Infof("Dependency %s not found in memory for transaction %s", dependencyID, t.ID)
			continue
		}

		//TODO this should be idempotent.
		dependencyTxn.preAssembleDependents = append(dependencyTxn.preAssembleDependents, t.ID)
	}

	return nil

}

func (t *Transaction) calculatePostAssembleDependencies(ctx context.Context) error {
	//Dependencies can arise because  we have been assembled to spend states that were produced by other transactions
	// or because there are other transactions from the same sender that have not been dispatched yet or because the user has declared explicit dependencies
	// this function calculates the dependencies relating to states and sets up the reverse association
	// it is assumed that the other dependencies have already been set up when the transaction was first received by the coordinator TODO correct this comment line with more accurate description of when we expect the static dependencies to have been calculated.  Or make it more vague.
	if t.PostAssembly == nil {
		msg := fmt.Sprintf("Cannot calculate dependencies for transaction %s without a PostAssembly", t.ID)
		log.L(ctx).Error(msg)
		return i18n.NewError(ctx, msgs.MsgSequencerInternalError, msg)
	}

	found := make(map[uuid.UUID]bool)
	t.dependencies = make([]uuid.UUID, 0, len(t.PostAssembly.InputStates)+len(t.PostAssembly.ReadStates))
	for _, state := range append(t.PostAssembly.InputStates, t.PostAssembly.ReadStates...) {
		dependency, err := t.grapher.LookupMinter(ctx, state.ID)
		if err != nil {
			errMsg := fmt.Sprintf("Error looking up dependency for state %s: %s", state.ID, err)
			log.L(ctx).Error(errMsg)
			return i18n.NewError(ctx, msgs.MsgSequencerInternalError, errMsg)
		}
		if dependency == nil {
			log.L(ctx).Infof("No minter found for state %s", state.ID)
			//assume the state was produced by a confirmed transaction
			//TODO should we validate this by checking the domain context? If not, explain why this is safe in the architecture doc
			continue
		}
		if found[dependency.ID] {
			continue
		}
		found[dependency.ID] = true
		t.dependencies = append(t.dependencies, dependency.ID)
		//also set up the reverse association
		dependency.dependents = append(dependency.dependents, t.ID)
	}
	return nil
}

func (t *Transaction) writeLockAndDistributeStates(ctx context.Context) error {
	return t.stateIntegration.WriteLockAndDistributeStatesForTransaction(ctx, t.PrivateTransaction)
}

func (t *Transaction) incrementAssembleErrors(ctx context.Context) error {
	t.errorCount++
	return nil
}
