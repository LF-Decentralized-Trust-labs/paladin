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

package sender

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/common"
	"github.com/kaleido-io/paladin/core/internal/sequencer/sender/transaction"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type sender struct {
	/* State */
	stateMachine                 *StateMachine
	activeCoordinator            string
	activeCoordinatorBlockHeight uint64
	timeOfMostRecentHeartbeat    common.Time
	transactionsByID             map[uuid.UUID]*transaction.Transaction
	transactionsOrdered          []*uuid.UUID
	currentBlockHeight           uint64

	/* Config */
	blockRangeSize       uint64
	contractAddress      *tktypes.EthAddress
	blockHeightTolerance uint64
	committee            map[string][]string
	heartbeatThresholdMs common.Duration

	/* Dependencies */
	messageSender    MessageSender
	clock            common.Clock
	stateIntegration common.StateIntegration
	emit             common.EmitEvent
}

func NewSender(
	ctx context.Context,
	messageSender MessageSender,
	committeeMembers []string,
	clock common.Clock,
	emit common.EmitEvent,
	stateIntegration common.StateIntegration,
	blockRangeSize uint64,
	contractAddress *tktypes.EthAddress,
	heartbeatPeriodMs int,
	heartbeatThresholdIntervals int,
) (*sender, error) {
	s := &sender{
		transactionsByID:     make(map[uuid.UUID]*transaction.Transaction),
		messageSender:        messageSender,
		blockRangeSize:       blockRangeSize,
		contractAddress:      contractAddress,
		clock:                clock,
		stateIntegration:     stateIntegration,
		emit:                 emit,
		heartbeatThresholdMs: clock.Duration(heartbeatPeriodMs * heartbeatThresholdIntervals),
	}
	s.committee = make(map[string][]string)
	for _, member := range committeeMembers {
		memberLocator := tktypes.PrivateIdentityLocator(member)
		memberNode, err := memberLocator.Node(ctx, false)
		if err != nil {
			log.L(ctx).Errorf("Error resolving node for member %s: %v", member, err)
			return nil, err
		}

		memberIdentity, err := memberLocator.Identity(ctx)
		if err != nil {
			log.L(ctx).Errorf("Error resolving identity for member %s: %v", member, err)
			return nil, err
		}

		if _, ok := s.committee[memberNode]; !ok {
			s.committee[memberNode] = make([]string, 0)
		}

		s.committee[memberNode] = append(s.committee[memberNode], memberIdentity)

	}
	s.InitializeStateMachine(State_Idle)
	return s, nil
}

func (s *sender) propagateEventToTransaction(ctx context.Context, event transaction.Event) error {
	if txn := s.transactionsByID[event.GetTransactionID()]; txn != nil {
		return txn.HandleEvent(ctx, event)
	} else {
		log.L(ctx).Debugf("Ignoring Event because transaction not known to this coordinator %s", event.GetTransactionID().String())
	}
	return nil
}

func (s *sender) createTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	newTxn, err := transaction.NewTransaction(ctx, txn)
	if err != nil {
		log.L(ctx).Errorf("Error creating transaction: %v", err)
		return err
	}
	s.transactionsByID[txn.ID] = newTxn
	s.transactionsOrdered = append(s.transactionsOrdered, &txn.ID)

	return nil

}

func (s *sender) transactionsOrderedByCreatedTime(ctx context.Context) ([]*components.PrivateTransaction, error) {
	ordered := make([]*components.PrivateTransaction, len(s.transactionsOrdered))
	for i, id := range s.transactionsOrdered {
		ordered[i] = s.transactionsByID[*id].PrivateTransaction
	}
	return ordered, nil
}

func ptrTo[T any](v T) *T {
	return &v
}
