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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"golang.org/x/crypto/sha3"
)

type assembleRequestFromCoordinator struct {
	coordinatorsBlockHeight int64
	stateLocksJSON          []byte
	requestID               uuid.UUID
}

// Transaction tracks the state of a transaction that is being sent by the local node in Sender state.
type Transaction struct {
	stateMachine *StateMachine
	*components.PrivateTransaction
	engineIntegration                common.EngineIntegration
	messageSender                    MessageSender
	currentDelegate                  string
	latestAssembleRequest            *assembleRequestFromCoordinator
	latestFulfilledAssembleRequestID uuid.UUID
	emit                             common.EmitEvent
	signerAddress                    *tktypes.EthAddress
	latestSubmissionHash             *tktypes.Bytes32
	nonce                            *uint64
}

func NewTransaction(
	ctx context.Context,
	pt *components.PrivateTransaction,
	messageSender MessageSender,
	clock common.Clock,
	emit common.EmitEvent,
	engineIntegration common.EngineIntegration,

) (*Transaction, error) {
	txn := &Transaction{
		PrivateTransaction: pt,
		engineIntegration:  engineIntegration,
		emit:               emit,
		messageSender:      messageSender,
	}

	txn.InitializeStateMachine(State_Initial)

	return txn, nil
}

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

func ptrTo[T any](v T) *T {
	return &v
}

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by reading maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (t *Transaction) GetCurrentState() State {
	return t.stateMachine.currentState
}

func (t *Transaction) GetSignerAddress() *tktypes.EthAddress {
	return t.signerAddress
}

func (t *Transaction) GetLatestSubmissionHash() *tktypes.Bytes32 {
	return t.latestSubmissionHash
}

func (t *Transaction) GetNonce() *uint64 {
	return t.nonce
}
