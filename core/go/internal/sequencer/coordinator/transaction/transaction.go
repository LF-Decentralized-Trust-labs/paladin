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
	heartbeatIntervalsSinceStateChange               int
	pendingAssembleRequest                           *common.IdempotentRequest
	cancelAssembleTimeoutSchedule                    func()
	cancelEndorsementRequestTimeoutSchedule          func()
	cancelDispatchConfirmationRequestTimeoutSchedule func()
	pendingEndorsementRequests                       map[string]map[string]*common.IdempotentRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	pendingDispatchConfirmationRequest               *common.IdempotentRequest
	latestError                                      string
	dependencies                                     []uuid.UUID //TODO figure out naming of these fields and their relationship with the PrivateTransaction fields
	dependents                                       []uuid.UUID
	preAssembleDependents                            []uuid.UUID
	previousTransaction                              *Transaction
	nextTransaction                                  *Transaction

	requestTimeout  common.Duration
	assembleTimeout common.Duration
	errorCount      int
	// Dependencies
	clock              common.Clock
	messageSender      MessageSender
	grapher            Grapher
	stateIntegration   common.StateIntegration
	notifyOfTransition OnStateTransition
	emit               common.EmitEvent
}

// TODO think about naming of this compared to the OnTransitionTo func in the state machine
type OnStateTransition func(ctx context.Context, t *Transaction, to, from State) // function to be invoked when transitioning into this state.  Called after transitioning event has been applied and any actions have fired

func NewTransaction(ctx context.Context, sender string, pt *components.PrivateTransaction, messageSender MessageSender, clock common.Clock, emit common.EmitEvent, stateIntegration common.StateIntegration, requestTimeout, assembleTimeout common.Duration, grapher Grapher, onStateTransition OnStateTransition) (*Transaction, error) {
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
		emit:               emit,
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

func (t *Transaction) Sender() string {
	return t.sender
}

func (t *Transaction) SenderNode() string {
	return t.senderNode
}

func (t *Transaction) SenderIdentity() string {
	return t.senderIdentity
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
