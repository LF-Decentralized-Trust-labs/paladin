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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/sequencer/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
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
	sender               string // The fully qualified identity of the sender e.g. "member1@node1"
	senderNode           string // The node the sender is running on e.g. "node1"
	senderIdentity       string // The member ID e.g. "member1"
	signerAddress        *pldtypes.EthAddress
	latestSubmissionHash *pldtypes.Bytes32
	nonce                *uint64
	stateMachine         *StateMachine
	revertReason         pldtypes.HexBytes

	//TODO move the fields that are really just fine grained state info.  Move them into the stateMachine struct ( consider separate structs for each concrete state)
	heartbeatIntervalsSinceStateChange               int
	pendingAssembleRequest                           *common.IdempotentRequest
	cancelAssembleTimeoutSchedule                    func()
	cancelEndorsementRequestTimeoutSchedule          func()
	cancelDispatchConfirmationRequestTimeoutSchedule func()
	onCleanup                                        func(context.Context)                           // function to be called when the transaction is removed from memory, e.g. when it is confirmed or reverted
	pendingEndorsementRequests                       map[string]map[string]*common.IdempotentRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	// Pending endorsements mutex
	pendingEndorsementsMutex           sync.Mutex // MRW TODO still to confirm if this is needed
	pendingDispatchConfirmationRequest *common.IdempotentRequest
	latestError                        string
	dependencies                       *pldapi.TransactionDependencies
	previousTransaction                *Transaction
	nextTransaction                    *Transaction
	onReadyForDispatch                 func(context.Context, *Transaction)
	// dependencies                                     []uuid.UUID //TODO figure out naming of these fields and their relationship with the PrivateTransaction fields
	// dependents                                       []uuid.UUID

	//Configuration
	requestTimeout        common.Duration
	assembleTimeout       common.Duration
	errorCount            int
	finalizingGracePeriod int // number of heartbeat intervals that the transaction will remain in one of the terminal states ( Reverted or Confirmed) before it is removed from memory and no longer reported in heartbeats
	// Dependencies
	clock              common.Clock
	messageSender      MessageSender
	grapher            Grapher
	engineIntegration  common.EngineIntegration
	notifyOfTransition OnStateTransition
	emit               common.EmitEvent
	metrics            metrics.DistributedSequencerMetrics
}

// TODO think about naming of this compared to the OnTransitionTo func in the state machine
type OnStateTransition func(ctx context.Context, t *Transaction, to, from State) // function to be invoked when transitioning into this state.  Called after transitioning event has been applied and any actions have fired

func NewTransaction(
	ctx context.Context,
	sender string,
	pt *components.PrivateTransaction,
	messageSender MessageSender,
	clock common.Clock,
	emit common.EmitEvent,
	engineIntegration common.EngineIntegration,
	requestTimeout,
	assembleTimeout common.Duration,
	finalizingGracePeriod int,
	grapher Grapher,
	onStateTransition OnStateTransition,
	onCleanup func(context.Context),
	onReadyForDispatch func(context.Context, *Transaction),
	metrics metrics.DistributedSequencerMetrics,
) (*Transaction, error) {
	senderIdentity, senderNode, err := pldtypes.PrivateIdentityLocator(sender).Validate(ctx, "", false)
	if err != nil {
		log.L(ctx).Errorf("[Sequencer] error validating sender %s: %s", sender, err)
		return nil, err
	}
	txn := &Transaction{
		sender:                sender,
		senderIdentity:        senderIdentity,
		senderNode:            senderNode,
		PrivateTransaction:    pt,
		messageSender:         messageSender,
		clock:                 clock,
		grapher:               grapher,
		requestTimeout:        requestTimeout,
		assembleTimeout:       assembleTimeout,
		finalizingGracePeriod: finalizingGracePeriod,
		engineIntegration:     engineIntegration,
		notifyOfTransition:    onStateTransition,
		onCleanup:             onCleanup,
		emit:                  emit,
		dependencies:          &pldapi.TransactionDependencies{},
		onReadyForDispatch:    onReadyForDispatch,
		metrics:               metrics,
	}
	txn.InitializeStateMachine(State_Initial)
	grapher.Add(context.Background(), txn)
	return txn, nil
}

func (t *Transaction) cleanup(_ context.Context) error {
	return t.grapher.Forget(t.ID)
}

func (t *Transaction) GetSignerAddress() *pldtypes.EthAddress {
	return t.signerAddress
}

func (t *Transaction) GetNonce() *uint64 {
	return t.nonce
}

func (t *Transaction) GetState() State {
	return t.stateMachine.currentState
}

func (t *Transaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	return t.latestSubmissionHash
}

func (t *Transaction) GetRevertReason() pldtypes.HexBytes {
	return t.revertReason
}

// Hash method of Transaction
func (t *Transaction) Hash(ctx context.Context) (*pldtypes.Bytes32, error) {
	if t.PrivateTransaction == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PrivateTransaction")
	}
	if t.PostAssembly == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without PostAssembly")
	}

	// MRW TODO - MUST DO - this was relying on only signatures being present, but Pente contracts reject transactions that have both signatures and endorsements.
	// if len(t.PostAssembly.Signatures) == 0 {
	// 	return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "Cannot hash transaction without at least one Signature")
	// }

	hash := sha3.NewLegacyKeccak256()

	if len(t.PostAssembly.Signatures) != 0 {
		for _, signature := range t.PostAssembly.Signatures {
			hash.Write(signature.Payload)
		}
	}

	var h32 pldtypes.Bytes32
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

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by reading maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (t *Transaction) GetCurrentState() State {
	return t.stateMachine.currentState
}

func (t *Transaction) GetErrorCount() int {
	return t.errorCount
}
