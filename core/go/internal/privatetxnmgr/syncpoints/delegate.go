// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syncpoints

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type delegateOperation struct {
	ID             uuid.UUID `json:"id"`
	TransactionID  uuid.UUID `json:"transaction_id"`
	DelegateNodeID string    `json:"delegate_node_id"`
}

// DelegateTransaction writes a record to the local database recording that the given transaction has been delegated to the given delegate
// then triggers a reliable cross node handshake to transmit that delegation to the delegate node and record their acknowledgement
func (s *syncPoints) QueueDelegation(ctx context.Context, contractAddress tktypes.EthAddress, transactionID uuid.UUID, delegateNodeID string, onCommit func(context.Context), onRollback func(context.Context, error)) {

	op := s.writer.Queue(ctx, &syncPointOperation{
		contractAddress: contractAddress,
		delegateOperation: &delegateOperation{
			ID:             uuid.New(),
			TransactionID:  transactionID,
			DelegateNodeID: delegateNodeID,
		},
	})
	go func() {
		if _, err := op.WaitFlushed(ctx); err != nil {
			onRollback(ctx, err)
		} else {
			onCommit(ctx)
		}
	}()
}

func (s *syncPoints) writeDelegateOperations(ctx context.Context, dbTX *gorm.DB, delegateOperations []*delegateOperation) error {

	// For each operation in the batch, we simply need insert a row to the database
	log.L(ctx).Debugf("Writing delegations %d", len(delegateOperations))
	err := dbTX.
		Table("transaction_delegations").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "transaction_id"},
				{Name: "delegate_node_id"},
			},
			DoNothing: true, // immutable
		}).
		Create(delegateOperations).
		Error

	if err != nil {
		log.L(ctx).Errorf("Error persisting delegations: %s", err)
		return err
	}

	return nil
}
