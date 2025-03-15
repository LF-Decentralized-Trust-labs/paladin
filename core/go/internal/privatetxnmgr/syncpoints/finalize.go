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
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// a transaction finalization operation is an update to the transaction managers tables
// to record a failed transaction.  nothing gets written to any tables owned by the private transaction manager
// but the write is coordinated by our flush writer to minimize the number of database transactions
type finalizeOperation struct {
	Domain         string
	TransactionID  uuid.UUID
	FailureMessage string
}

// QueueTransactionFinalize
func (s *syncPoints) QueueTransactionFinalize(ctx context.Context, domain string, contractAddress tktypes.EthAddress, transactionID uuid.UUID, failureMessage string, onCommit func(context.Context), onRollback func(context.Context, error)) {

	op := s.writer.Queue(ctx, &syncPointOperation{
		domainContext:   nil, // finalize does not depend on the flushing of any states
		contractAddress: contractAddress,
		finalizeOperation: &finalizeOperation{
			Domain:         domain,
			TransactionID:  transactionID,
			FailureMessage: failureMessage,
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

func (s *syncPoints) writeFailureOperations(ctx context.Context, dbTX persistence.DBTX, finalizeOperations []*finalizeOperation) error {

	// We are only responsible for failures. Success receipts are written on the DB transaction of the event handler,
	// so they are guaranteed to be written in sequence for each confirmed domain private transaction.
	//
	// However, a syncpoint gets triggered for every finalize so that we can flush the Domain Context to the DB
	// so that all states are stored, before we clear out the transaction from the in-memory Domain Context.
	failureReceipts := make([]*components.ReceiptInput, 0)
	for _, op := range finalizeOperations {
		if op.FailureMessage != "" {
			failureReceipts = append(failureReceipts, &components.ReceiptInput{
				ReceiptType:    components.RT_FailedWithMessage,
				Domain:         op.Domain,
				TransactionID:  op.TransactionID,
				FailureMessage: op.FailureMessage,
			})
		}
	}
	if len(failureReceipts) > 0 {
		return s.txMgr.FinalizeTransactions(ctx, dbTX, failureReceipts)
	}
	return nil

}
