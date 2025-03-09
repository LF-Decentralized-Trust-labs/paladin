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
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
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
	engineIntegration         common.EngineIntegration
	messageSender             MessageSender
	currentDelegate           string
	inprogressAssembleRequest *assembleRequestFromCoordinator
	emit                      common.EmitEvent
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

func ptrTo[T any](v T) *T {
	return &v
}
