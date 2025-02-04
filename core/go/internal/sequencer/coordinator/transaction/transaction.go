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
	"time"

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
	sender                             string //TODO what is this?  A node? An identity? An identity locator?
	signerAddress                      *tktypes.EthAddress
	latestSubmissionHash               *tktypes.Bytes32
	nonce                              *uint64
	stateMachine                       *StateMachine
	heartbeatIntervalsSinceStateChange int
	pendingEndorsementRequests         map[string]map[string]*endorsementRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	latestError                        string
	dependencies                       []*Transaction
	dependents                         []*Transaction
	// Dependencies
	clock         common.Clock
	messageSender MessageSender
	stateIndex    StateIndex
}

func NewTransaction(sender string, pt *components.PrivateTransaction, messageSender MessageSender, clock common.Clock, stateIndex StateIndex) *Transaction {
	txn := &Transaction{
		sender:             sender,
		PrivateTransaction: pt,
		messageSender:      messageSender,
		clock:              clock,
		stateIndex:         stateIndex,
	}
	txn.InitializeStateMachine(State_Pooled)
	return txn
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

// SignatureAttestationName is a method of Transaction that returns the name of the attestation in the attestation plan that is a signature
func (t *Transaction) SignatureAttestationName() (string, error) {
	for _, attRequest := range t.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			return attRequest.Name, nil
		}
	}
	return "", nil
}

func (t *Transaction) applyEndorsement(_ context.Context, endorsement *prototk.AttestationResult, requestID string) error {
	//TODO check that this matches a pending request and that it has not timed out

	t.PostAssembly.Endorsements = append(t.PostAssembly.Endorsements, endorsement)

	return nil
}

func (t *Transaction) applyPostAssembly(ctx context.Context, postAssembly *components.TransactionPostAssembly) {
	//TODO check that this matches a pending request and that it has not timed out

	//TODO the response from the assembler actually contains outputStatesPotential so we need to write them to the store and then add the OutputState ids to the index
	t.PostAssembly = postAssembly
	for _, state := range postAssembly.OutputStates {
		t.stateIndex.AddMinter(ctx, state.ID, t)
	}
	t.calculateDependencies(ctx)
}

func (t *Transaction) Sender() string {
	return t.sender
}

func (d *Transaction) IsEndorsed(ctx context.Context) bool {
	return !d.hasOutstandingEndorsementRequests(ctx)
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
func (d *Transaction) hasOutstandingEndorsementRequests(ctx context.Context) bool {
	return len(d.outstandingEndorsementRequests(ctx)) > 0
}

// TODO rework this.  There are actually 2 things to keep track of a) unfulfilled endorsement requirements.  i.e. there is a thing in the plan and we don't have an attestation for it and b) outstanding requests.  i.e. we have sent a request and not had a response yet
func (d *Transaction) outstandingEndorsementRequests(ctx context.Context) []*endorsementRequirement {
	outstandingEndorsementRequests := make([]*endorsementRequirement, 0)
	if d.PostAssembly == nil {
		log.L(ctx).Debug("PostAssembly is nil so there are no outstanding endorsement requests")
		return outstandingEndorsementRequests
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
					log.L(ctx).Debugf("endorsement request for %s outstanding for transaction %s", party, d.ID)
					outstandingEndorsementRequests = append(outstandingEndorsementRequests, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}
	return outstandingEndorsementRequests
}

func (t *Transaction) sendAssembleRequest(ctx context.Context) error {
	return t.messageSender.SendAssembleRequest(ctx, t.sender, t.ID, t.PreAssembly)
}

type endorsementRequest struct {
	//time the request was made
	requestTime time.Time
	//unique string to identify the request (non unique across retries)
	idempotencyKey string
}

// Function recentlyRequested checks if the endorsement has been previously requested.  There are 3 possibilities, a) it was requested recently (retry threshold has not passed) b) it was requested but the request timed out c) it was never requested
// if a) retry is false.  if b) retry is true and the idempotency key is provided.  if c) retry is true and a new idempotency key is generated
func (r *endorsementRequest) checkForRetry(ctx context.Context, outstandingEndorsementRequest *endorsementRequirement) (bool, string) {
	//TODO
	// there is a request in the attestation plan and we do not have a response to match it
	// first lets see if we have recently sent a request for this endorsement and just need to be patient
	/*
		previousRequestTime := time.Time{}
		previousIdempotencyKey := ""
		if pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name]; ok {
			if r, ok := pendingRequestsForAttRequest[outstandingEndorsementRequest.party]; ok {
				previousRequestTime = r.requestTime
				previousIdempotencyKey = r.idempotencyKey
			}
		} else {
			tf.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name] = make(map[string]*endorsementRequest)
		}

		if !previousRequestTime.IsZero() && tf.clock.Now().Before(previousRequestTime.Add(tf.requestTimeout)) {
			//We have already sent a message for this request and the deadline has not passed
			log.L(ctx).Debugf("Transaction %s endorsement already requested %v", tf.transaction.ID.String(), previousRequestTime)
			return
		}
		if previousRequestTime.IsZero() {
			log.L(ctx).Infof("Transaction %s endorsement has never been requested for attestation request:%s, party:%s", tf.transaction.ID.String(), outstandingEndorsementRequest.attRequest.Name, outstandingEndorsementRequest.party)
		} else {
			log.L(ctx).Infof("Previous endorsement request for transaction:%s, attestation request:%s, party:%s sent at %v has timed out", tf.transaction.ID.String(), outstandingEndorsementRequest.attRequest.Name, outstandingEndorsementRequest.party, previousRequestTime)
		}

		if previousIdempotencyKey != "" {
			tf.logActionDebug(ctx, fmt.Sprintf("Previous endorsement request timed out. Sending new request with same idempotency key %s", previousIdempotencyKey))
			idempotencyKey = previousIdempotencyKey
		}
	*/

	return false, ""
}

func (t *Transaction) getPendingEndorsementRequest(ctx context.Context, attestationRequestName, party string) (*endorsementRequest, bool) {
	if pendingRequestsForAttRequest, ok := t.pendingEndorsementRequests[attestationRequestName]; ok {
		r, ok := pendingRequestsForAttRequest[party]
		return r, ok
	}
	return nil, false
}

// Function hasDependenciesNotReady checks if the transaction has any dependencies that themselves are not ready for dispatch
func (t *Transaction) hasDependenciesNotReady(ctx context.Context) bool {
	//TODO rethink the name of this function

	//We already calculated the dependencies when we got assembled and there is no way we could have picked up new
	// dependencies without a re-assemble
	// some of them might have been confirmed and removed from our list to avoid a memory leak so this is not necessarily the complete list of dependencies
	// but it should contain all the ones that are not ready for dispatch

	for _, dependency := range t.dependencies {
		//test against the list of states that we consider to be past the point of ready as there is more chance of us noticing
		// a failing test if we add new states in the future and forget to update this list
		if dependency.GetState() != State_Confirmed &&
			dependency.GetState() != State_Submitted &&
			dependency.GetState() != State_Dispatched &&
			dependency.GetState() != State_Ready_For_Dispatch {
			return true
		}
	}

	return false
}

// Function sendEndorsementRequests iterates through the attestation plan and for each endorsement request that has not been fulfilled
// sends an endorsement request to the appropriate party unless there was a recent request (i.e. within the retry threshold)
func (t *Transaction) sendEndorsementRequests(ctx context.Context) error {

	if t.pendingEndorsementRequests == nil {
		t.pendingEndorsementRequests = make(map[string]map[string]*endorsementRequest)
	}

	for _, outstandingEndorsementRequest := range t.outstandingEndorsementRequests(ctx) {
		idempotencyKey := uuid.New().String()

		if pendingRequestsForAttRequest, ok := t.getPendingEndorsementRequest(ctx, outstandingEndorsementRequest.attRequest.Name, outstandingEndorsementRequest.party); ok {
			//we have a previously made a request to this party for this attestation
			doRetry, previousIdempotencyKey := pendingRequestsForAttRequest.checkForRetry(ctx, outstandingEndorsementRequest)

			if doRetry {
				idempotencyKey = previousIdempotencyKey
			} else {
				//skip this endorsement request
				continue
			}
		}

		t.requestEndorsement(ctx, idempotencyKey, outstandingEndorsementRequest.party, outstandingEndorsementRequest.attRequest)
		if t.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name] == nil {
			t.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name] = make(map[string]*endorsementRequest)
		}
		t.pendingEndorsementRequests[outstandingEndorsementRequest.attRequest.Name][outstandingEndorsementRequest.party] =
			&endorsementRequest{
				requestTime:    t.clock.Now(),
				idempotencyKey: idempotencyKey,
			}
	}
	return nil
}

func (t *Transaction) sendDispatchConfirmationRequest(ctx context.Context) error {
	idempotencyKey := uuid.New().String()
	hash, err := t.Hash(ctx)
	if err != nil {
		return err
	}
	t.messageSender.SendDispatchConfirmationRequest(
		ctx,
		t.sender,
		idempotencyKey,
		t.PreAssembly.TransactionSpecification,
		hash,
	)

	return nil
}

func (t *Transaction) notifyDependentsOfReadiness(ctx context.Context) error {
	//this function is called when the transaction enters the ready for dispatch state
	// and we have a duty to inform all the transactions that are dependent on us that we are ready in case they are otherwise ready and are blocked waiting for us
	for _, dependent := range t.dependents {
		dependent.HandleEvent(ctx, &DependencyReadyEvent{
			event: event{
				TransactionID: dependent.ID,
			},
			DependencyID: t.ID,
		})
	}
	return nil
}

func (t *Transaction) requestEndorsement(ctx context.Context, idempotencyKey string, party string, attRequest *prototk.AttestationRequest) {

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

func (t *Transaction) calculateDependencies(ctx context.Context) {
	if t.PostAssembly == nil {
		log.L(ctx).Errorf("Cannot calculate dependencies for transaction %s without a PostAssembly", t.ID)
		//TODO should never get here so this is a panic or at least abort the the current contract
		return
	}

	found := make(map[string]bool)
	t.dependencies = make([]*Transaction, 0, len(t.PostAssembly.InputStates)+len(t.PostAssembly.ReadStates))
	for _, state := range append(t.PostAssembly.InputStates, t.PostAssembly.ReadStates...) {
		dependency, err := t.stateIndex.LookupMinter(ctx, state.ID)
		if err != nil {
			log.L(ctx).Errorf("Error looking up dependency for state %s: %s", state.ID, err)
			//TODO no good reason to expect an error here so this is a panic or at least abort the the current contract
			return
		}
		if dependency == nil {
			log.L(ctx).Infof("No minter found for state %s", state.ID)
			//assume the state was produced by a confirmed transaction
			//TODO should we validate this by checking the domain context?
			continue
		}
		if found[dependency.ID.String()] {
			continue
		}
		found[dependency.ID.String()] = true
		t.dependencies = append(t.dependencies, dependency)
		//also set up the reverse association
		dependency.dependents = append(dependency.dependents, t)
	}
}
