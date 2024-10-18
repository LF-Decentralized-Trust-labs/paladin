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

type delegationAckOperation struct {
	ID           uuid.UUID `json:"id"`
	DelegationID uuid.UUID `json:"delegation_id"`
}

func (s *syncPoints) QueueDelegationAck(ctx context.Context, contractAddress tktypes.EthAddress, delegationID uuid.UUID, onCommit func(context.Context), onRollback func(context.Context, error)) {

	op := s.writer.Queue(ctx, &syncPointOperation{
		contractAddress: contractAddress,
		delegationAckOperation: &delegationAckOperation{
			ID:           uuid.New(),
			DelegationID: delegationID,
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

func (s *syncPoints) writeDelegationAckOperations(ctx context.Context, dbTX *gorm.DB, delegationAckOperations []*delegationAckOperation) error {

	// For each operation in the batch, we simply need insert a row to the database
	log.L(ctx).Debugf("Writing delegations %d", len(delegationAckOperations))
	err := dbTX.
		Table("transaction_delegation_acknowledgements").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "delegation"},
			},
			DoNothing: true, // immutable
		}).
		Create(delegationAckOperations).
		Error

	if err != nil {
		log.L(ctx).Errorf("Error persisting delegation acknowledgments: %s", err)
		return err
	}

	return nil
}
