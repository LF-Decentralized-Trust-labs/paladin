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
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/preparedtxdistribution"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dispatchOperation struct {
	publicDispatches         []*PublicDispatch
	privateDispatches        []*components.ValidatedTransaction
	preparedTransactions     []*components.PreparedTransactionWithRefs
	preparedTxnDistributions []*preparedtxdistribution.PreparedTxnDistributionPersisted
}

type DispatchPersisted struct {
	ID                       string             `json:"id"`
	PrivateTransactionID     string             `json:"privateTransactionID"`
	PublicTransactionAddress tktypes.EthAddress `json:"publicTransactionAddress"`
	PublicTransactionID      uint64             `json:"publicTransactionID"`
}

// A dispatch sequence is a collection of private transactions that are submitted together for a given signing address in order
type PublicDispatch struct {
	PublicTxs                    []*components.PublicTxSubmission
	PrivateTransactionDispatches []*DispatchPersisted
}

// a dispatch batch is a collection of dispatch sequences that are submitted together with no ordering requirements between sequences
// purely for a database performance reason, they are included in the same transaction
type DispatchBatch struct {
	PublicDispatches     []*PublicDispatch
	PrivateDispatches    []*components.ValidatedTransaction
	PreparedTransactions []*components.PreparedTransactionWithRefs
}

// PersistDispatches persists the dispatches to the database and coordinates with the public transaction manager
// to submit public transactions.
func (s *syncPoints) PersistDispatchBatch(dCtx components.DomainContext, contractAddress tktypes.EthAddress, dispatchBatch *DispatchBatch, stateDistributions []*components.StateDistributionWithData, preparedTxnDistributions []*preparedtxdistribution.PreparedTxnDistribution) error {

	preparedTxnDistributionsPersisted := make([]*preparedtxdistribution.PreparedTxnDistributionPersisted, 0, len(dispatchBatch.PreparedTransactions))
	for _, preparedTxnDistribution := range preparedTxnDistributions {
		preparedTxnDistributionsPersisted = append(preparedTxnDistributionsPersisted, &preparedtxdistribution.PreparedTxnDistributionPersisted{
			ID:              preparedTxnDistribution.ID,
			PreparedTxnID:   preparedTxnDistribution.PreparedTxnID,
			IdentityLocator: preparedTxnDistribution.IdentityLocator,
			DomainName:      preparedTxnDistribution.Domain,
			ContractAddress: preparedTxnDistribution.ContractAddress,
		})
	}

	// Send the write operation with all of the batch sequence operations to the flush worker
	op := s.writer.Queue(dCtx.Ctx(), &syncPointOperation{
		domainContext:   dCtx,
		contractAddress: contractAddress,
		dispatchOperation: &dispatchOperation{
			publicDispatches:         dispatchBatch.PublicDispatches,
			privateDispatches:        dispatchBatch.PrivateDispatches,
			preparedTransactions:     dispatchBatch.PreparedTransactions,
			preparedTxnDistributions: preparedTxnDistributionsPersisted,
		},
	})

	//wait for the flush to complete
	_, err := op.WaitFlushed(dCtx.Ctx())
	return err
}

func (s *syncPoints) PersistDeployDispatchBatch(ctx context.Context, dispatchBatch *DispatchBatch) error {

	// Send the write operation with all of the batch sequence operations to the flush worker
	op := s.writer.Queue(ctx, &syncPointOperation{
		dispatchOperation: &dispatchOperation{
			publicDispatches: dispatchBatch.PublicDispatches,
		},
	})

	//wait for the flush to complete
	_, err := op.WaitFlushed(ctx)
	return err
}

func (s *syncPoints) writeDispatchOperations(ctx context.Context, dbTX *gorm.DB, dispatchOperations []*dispatchOperation) (postCommits []func(), err error) {

	// For each operation in the batch, we need to call the baseledger transaction manager to allocate its nonce
	// which it can only guaranteed to be gapless and unique if it is done during the database transaction that inserts the dispatch record.

	// Build lists of things to insert (we are insert only)
	for _, op := range dispatchOperations {

		//for each batchSequence operation, call the public transaction manager to allocate a nonce
		//and persist the intent to send the states to the distribution list.
		for _, dispatchSequenceOp := range op.publicDispatches {
			if len(dispatchSequenceOp.PrivateTransactionDispatches) == 0 {
				continue
			}

			// Call the public transaction manager persist to the database under the current transaction
			pubTXCb, publicTxns, err := s.pubTxMgr.WriteNewTransactions(ctx, dbTX, dispatchSequenceOp.PublicTxs)
			if err != nil {
				log.L(ctx).Errorf("Error submitting public transactions: %s", err)
				return nil, err
			}
			postCommits = append(postCommits, pubTXCb)

			//TODO this results in an `INSERT` for each dispatchSequence
			//Would it be more efficient to pass an array for the whole flush?
			// could get complicated on the public transaction manager side because
			// it needs to allocate a nonce for each dispatch and that is specific to signing key
			for dispatchIndex, dispatch := range dispatchSequenceOp.PrivateTransactionDispatches {

				//fill in the foreign key before persisting in our dispatch table
				dispatch.PublicTransactionAddress = publicTxns[dispatchIndex].From
				dispatch.PublicTransactionID = *publicTxns[dispatchIndex].LocalID
				dispatch.ID = uuid.New().String()
			}

			log.L(ctx).Debugf("Writing dispatch batch %d", len(dispatchSequenceOp.PrivateTransactionDispatches))

			err = dbTX.
				Table("dispatches").
				Clauses(clause.OnConflict{
					Columns: []clause.Column{
						{Name: "private_transaction_id"},
						{Name: "public_transaction_address"},
						{Name: "public_transaction_id"},
					},
					DoNothing: true, // immutable
				}).
				Create(dispatchSequenceOp.PrivateTransactionDispatches).
				Error

			if err != nil {
				log.L(ctx).Errorf("Error persisting dispatches: %s", err)
				return nil, err
			}

		}

		if len(op.privateDispatches) > 0 {
			txPostCommit, err := s.txMgr.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, op.privateDispatches)
			if err != nil {
				log.L(ctx).Errorf("Error persisting private dispatches: %s", err)
				return nil, err
			}
			postCommits = append(postCommits, txPostCommit)
		}

		if len(op.preparedTransactions) > 0 {
			log.L(ctx).Debugf("Writing prepared transactions locally  %d", len(op.preparedTransactions))

			txPostCommit, err := s.txMgr.WritePreparedTransactions(ctx, dbTX, op.preparedTransactions)
			if err != nil {
				log.L(ctx).Errorf("Error persisting prepared transactions: %s", err)
				return nil, err
			}
			postCommits = append(postCommits, txPostCommit)
		}

		if len(op.preparedTxnDistributions) == 0 {
			log.L(ctx).Debug("No prepared transaction distributions to persist")
		} else {

			log.L(ctx).Debugf("Writing distribution record to send prepared transaction to remote node %d", len(op.preparedTxnDistributions))
			err := dbTX.
				Table("prepared_txn_distributions").
				Clauses(clause.OnConflict{
					Columns: []clause.Column{
						{Name: "prepared_txn_id"},
						{Name: "identity_locator"},
					},
					DoNothing: true, // immutable
				}).
				Create(op.preparedTxnDistributions).
				Error

			if err != nil {
				log.L(ctx).Errorf("Error persisting prepared transaction distributions: %s", err)
				return nil, err
			}
		}

		if len(op.stateDistributions) == 0 {
			log.L(ctx).Debug("No state distributions to persist")
		} else {
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
				return nil, err
			}
		}

	}
	return postCommits, nil
}
