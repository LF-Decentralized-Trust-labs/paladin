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
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dispatchOperation struct {
	dispatches         []*DispatchSequence
	stateDistributions []*statedistribution.StateDistributionPersisted
}

type DispatchPersisted struct {
	ID                       string             `json:"id"`
	PrivateTransactionID     string             `json:"privateTransactionID"`
	PublicTransactionAddress tktypes.EthAddress `json:"publicTransactionAddress"`
	PublicTransactionNonce   uint64             `json:"publicTransactionNonce"`
}

// A dispatch sequence is a collection of private transactions that are submitted together for a given signing address in order
type DispatchSequence struct {
	PublicTxBatch                components.PublicTxBatch
	PrivateTransactionDispatches []*DispatchPersisted
}

// a dispatch batch is a collection of dispatch sequences that are submitted together with no ordering requirements between sequences
// purely for a database performance reason, they are included in the same transaction
type DispatchBatch struct {
	DispatchSequences []*DispatchSequence
}

// PersistDispatches persists the dispatches to the database and coordinates with the public transaction manager
// to submit public transactions.
func (s *syncPoints) PersistDispatchBatch(ctx context.Context, contractAddress tktypes.EthAddress, dispatchBatch *DispatchBatch, stateDistributions []*statedistribution.StateDistribution) error {

	stateDistributionsPersisted := make([]*statedistribution.StateDistributionPersisted, 0, len(stateDistributions))
	for _, stateDistribution := range stateDistributions {
		stateDistributionsPersisted = append(stateDistributionsPersisted, &statedistribution.StateDistributionPersisted{
			ID:              stateDistribution.ID,
			StateID:         stateDistribution.StateID,
			IdentityLocator: stateDistribution.IdentityLocator,
			DomainName:      stateDistribution.Domain,
			ContractAddress: stateDistribution.ContractAddress,
		})
	}
	// Send the write operation with all of the batch sequence operations to the flush worker
	op := s.writer.Queue(ctx, &syncPointOperation{
		contractAddress: contractAddress,
		dispatchOperation: &dispatchOperation{
			dispatches:         dispatchBatch.DispatchSequences,
			stateDistributions: stateDistributionsPersisted,
		},
	})

	//wait for the flush to complete
	_, err := op.WaitFlushed(ctx)
	return err
}

func (s *syncPoints) writeDispatchOperations(ctx context.Context, dbTX *gorm.DB, dispatchOperations []*dispatchOperation) error {

	// For each operation in the batch, we need to call the baseledger transaction manager to allocate its nonce
	// which it can only guaranteed to be gapless and unique if it is done during the database transaction that inserts the dispatch record.

	// Build lists of things to insert (we are insert only)
	for _, op := range dispatchOperations {

		//for each batchSequence operation, call the public transaction manager to allocate a nonce
		//and persist the intent to send the states to the distribution list.
		for _, dispatchSequenceOp := range op.dispatches {
			// Call the public transaction manager to allocate nonces for all transactions in the sequence
			// and persist them to the database under the current transaction
			pubBatch := dispatchSequenceOp.PublicTxBatch
			err := pubBatch.Submit(ctx, dbTX)
			if err != nil {
				log.L(ctx).Errorf("Error submitting public transaction: %s", err)
				// TODO  this is a really bad situation because it will cause all dispatches in the flush to rollback
				// Should we skip this dispatch ( or this mini batch of dispatches?)
				return err
			}
			publicTxIDs := pubBatch.Accepted()
			if len(publicTxIDs) != len(dispatchSequenceOp.PrivateTransactionDispatches) {
				errorMessage := fmt.Sprintf("Expected %d public transaction IDs, got %d", len(dispatchSequenceOp.PrivateTransactionDispatches), len(publicTxIDs))
				log.L(ctx).Error(errorMessage)
				return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
			}

			//TODO this results in an `INSERT` for each dispatchSequence
			//Would it be more efficient to pass an array for the whole flush?
			// could get complicated on the public transaction manager side because
			// it needs to allocate a nonce for each dispatch and that is specific to signing key
			for dispatchIndex, dispatch := range dispatchSequenceOp.PrivateTransactionDispatches {

				//fill in the foreign key before persisting in our dispatch table
				dispatch.PublicTransactionAddress = publicTxIDs[dispatchIndex].PublicTx().From
				dispatch.PublicTransactionNonce = publicTxIDs[dispatchIndex].PublicTx().Nonce.Uint64()

				dispatch.ID = uuid.New().String()
			}
			log.L(ctx).Debugf("Writing dispatch batch %d", len(dispatchSequenceOp.PrivateTransactionDispatches))

			err = dbTX.
				Table("dispatches").
				Clauses(clause.OnConflict{
					Columns: []clause.Column{
						{Name: "private_transaction_id"},
						{Name: "public_transaction_address"},
						{Name: "public_transaction_nonce"},
					},
					DoNothing: true, // immutable
				}).
				Create(dispatchSequenceOp.PrivateTransactionDispatches).
				Error

			if err != nil {
				log.L(ctx).Errorf("Error persisting dispatches: %s", err)
				return err
			}

		}

		if len(op.stateDistributions) == 0 {
			log.L(ctx).Debug("No state distributions to persist")
			continue
		}

		log.L(ctx).Debugf("Writing state distributions %d", len(op.stateDistributions))
		err := dbTX.
			Table("state_distributions").
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "state_id"},
					{Name: "identity_locator"},
				},
				DoNothing: true, // immutable
			}).
			Create(op.stateDistributions).
			Error

		if err != nil {
			log.L(ctx).Errorf("Error persisting state distributions: %s", err)
			return err
		}
	}
	return nil
}
