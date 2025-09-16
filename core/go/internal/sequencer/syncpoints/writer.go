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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/flushwriter"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

/*
Dispatching a transaction to the baseledger transaction manager is a point of no return and as part
of the hand over, we must get some assurance from the baseledger transaction manager that ordering will
be preserved.  So, we need to have the nonce allocated and written to the baseledger transaction manager's
database table in the same database transaction as the we update the state of the private transaction
table to record the dispatch. It would be a bottleneck on performance if we were to create a new database
transaction for each transaction that is dispatched given that there could be many hundreds, or even thousands
per second across all smart contract domains.  So we have a flush worker pattern here where a small number (relative to
the number of transaction processing threads ) of worker threads batch up several transactions across multiple
domain instances and run the database update for the dispatch and call the baseledger transaction manager
to atomically allocate and record the nonce under that same transaction.
*/

// the submit will happen on the user transaction manager's flush writer context so
// that it can be co-ordinated with the user transaction submission
// do we have any other checkpoints (e.g. on delegate?)

// a syncPointOperation is either a dispatch (handover to public transaction manager)
// or a finalizer (handover to TxManager to mark a transaction as reverted)
// or a delegate (intent to handover to a remote coordinator)
// or receipt of an acknowledgement from a remote coordinator
// or a receipt of a delegation from a remote assembler
// but never more than one of these.  We probably could make the mutually exclusive nature more explicit by using interfaces but its not worth the added complexity

type syncPointOperation struct {
	contractAddress   pldtypes.EthAddress
	domainContext     components.DomainContext
	finalizeOperation *finalizeOperation
	dispatchOperation *dispatchOperation
}

func (dso *syncPointOperation) WriteKey() string {
	return dso.contractAddress.String()
}

type noResult struct{}

func (s *syncPoints) runBatch(ctx context.Context, dbTX persistence.DBTX, values []*syncPointOperation) ([]flushwriter.Result[*noResult], error) {

	finalizeOperations := make([]*finalizeOperation, 0, len(values))
	dispatchOperations := make([]*dispatchOperation, 0, len(values))
	domainContextsToFlush := make(map[uuid.UUID]components.DomainContext)

	for _, op := range values {
		if op.domainContext != nil {
			domainContextsToFlush[op.domainContext.Info().ID] = op.domainContext
		}
		if op.finalizeOperation != nil {
			finalizeOperations = append(finalizeOperations, op.finalizeOperation)
		}
		if op.dispatchOperation != nil {
			dispatchOperations = append(dispatchOperations, op.dispatchOperation)
		}
	}

	// We flush all of the affected domain contexts first, as they might contain states we need to refer
	// to in the DB transaction below using foreign key relationships
	// We must track if we're returning an error with a nil callback, and ensure that in those cases
	// we call the dbTXCallback with the error for any contexts we've received back from a Flush() call
	var err error
	log.L(ctx).Infof("SyncPoints flush-writer: domain=contexts=%d finalizeOperations=%d dispatchOperations=%d",
		len(domainContextsToFlush), len(finalizeOperations), len(dispatchOperations))
	for _, dc := range domainContextsToFlush {
		err = dc.Flush(dbTX) // err variable must not be re-allocated
		if err != nil {
			return nil, err
		}
	}

	// If we have any finalizers, we need to call them now
	//big assumption here that all operations in the batch have the same `contractAddress` which happens to be a safe
	// assumption at time of coding because WriteKey returns the contract address
	// but probably should consider a less brittle way to codify this assertion
	if err == nil && len(finalizeOperations) > 0 {
		err = s.writeFailureOperations(ctx, dbTX, finalizeOperations) // err variable must not be re-allocated
	}

	if err == nil && len(dispatchOperations) > 0 {
		err = s.writeDispatchOperations(ctx, dbTX, dispatchOperations) // err variable must not be re-allocated
	}

	if err != nil {
		return nil, err
	}

	// We don't actually provide any result, so just build an array of nil results
	return make([]flushwriter.Result[*noResult], len(values)), nil

}
