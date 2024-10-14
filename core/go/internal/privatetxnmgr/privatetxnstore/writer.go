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

package privatetxnstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

/*
Dispatching a transaction to the baseledger transaction manager is a point of no return and as part
of the hand over, we must get some assurance from the baseledger transaction manager that ordering will
be preserved.  So, we need to have the nonce allocated and written to the baseledger transaction manager's
database table in the same database transaction as the we update the state of the private transaction
table to record the dispatch. It would be a bottleneck on performance if we were to create a new database
transaction for each transaction that is dispatched given that there could be many hundreds, or even thousands
per second accross all smart contract domains.  So we have a flush worker pattern here where a small number (relative to
the number of transaction processing threads ) of worker threads batch up several transactions across multiple
domain instances and run the database update for the dispatch and call the baseledger transaction manager
to atomically allocate and record the nonce under that same transaction.
*/

// the submit will happen on the user transaction manager's flush writer context so
// that it can be co-ordinated with the user transaction submission
// do we have any other checkpoints (e.g. on delegate?)

type dispatchSequenceOperation struct {
	contractAddress    tktypes.EthAddress
	dispatches         []*DispatchSequence
	stateDistributions []*statedistribution.StateDistributionPersisted
}

func (dso *dispatchSequenceOperation) WriteKey() string {
	return dso.contractAddress.String()
}

type noResult struct{}

func (s *store) runBatch(ctx context.Context, dbTX *gorm.DB, values []*dispatchSequenceOperation) ([]flushwriter.Result[*noResult], error) {

	/*To reliably allocate a nonce in a gapless sequence without locking out a bunch of threads for too long and without gitting deadlocks:
	- Before we go into a database transaction, check that we have a fresh record of the latest nonce in memory for the given signing address - reading DB and/or calling out to the blockchain node if needed
	 - On the private transaction manager thread: take a lock on the nonce allocator for the signing address
	 - defer a call to release the lock with a rollback ( i.e. by default, if the stack unwinds, we don't change the "next" nonce value)
	 - send a message to the flush worker's channel to
	   		- allocate increment the next nonce (or nonces, depending on how many transactions are being dispatched for that signing address in that flush batch - noting that the same flush batch will be potentially processing transactions for multiple signign keys from multile private transaction manger threads)
			- write the records to both ( BLT manager and Private transaction manager) tables
	 - wait for the flush worker to complete the operation and release the lock with a rollforward (i.e. next reader to get the lock will see the new nonce value)
	   - NOTE we have held the lock for quite a long time by now so if any other private transaction manager threads are trying to get a nonce for the same signign key, then the are slowed down. But that's ok because we assume it is extremely rare that multiple private transaction managers will be trying to use the same signing key
	 - if the flush worker fails, and the DB transaction is rolled back we need to roll back the lock and the nonce value
	 	- is there a timeout on the private transaction manager thread waiting for the flush worker? If it hasn't signalled completion in time, do we assume failure? Do we read the database? If in doubt, we can tell the none allocator to refresh its cache by reading the DB and/or calling the blockchain node
	 - if the
	*/

	// For each operation in the batch, we need to call the baseledger transaction manager to allocate its nonce
	// which it can only guaranteed to be gapless and unique if it is done during the database transaction that inserts the dispatch record.
	// However, this is
	// Build lists of things to insert (we are insert only)

	for _, op := range values {
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
				return nil, err
			}
			publicTxIDs := pubBatch.Accepted()
			if len(publicTxIDs) != len(dispatchSequenceOp.PrivateTransactionDispatches) {
				errorMessage := fmt.Sprintf("Expected %d public transaction IDs, got %d", len(dispatchSequenceOp.PrivateTransactionDispatches), len(publicTxIDs))
				log.L(ctx).Error(errorMessage)
				return nil, i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
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
				return nil, err
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
			return nil, err
		}
	}

	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*noResult], len(values)), nil

}
