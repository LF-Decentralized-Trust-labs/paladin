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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
)

// a transaction finalization operation is an update to the transaction managers tables
// to record a failed transaction.  nothing gets written to any tables owned by the private transaction manager
// but the write is coordinated by our flush writer to minimize the number of database transactions
type finalizeOperation struct {
	Domain         string
	TransactionID  uuid.UUID
	FailureMessage string
	Originator     string
}

// QueueTransactionFinalize
func (s *syncPoints) QueueTransactionFinalize(ctx context.Context, domain string, contractAddress pldtypes.EthAddress, originator string, transactionID uuid.UUID, failureMessage string, onCommit func(context.Context), onRollback func(context.Context, error)) {

	op := s.writer.Queue(ctx, &syncPointOperation{
		domainContext:   nil, // finalize does not depend on the flushing of any states
		contractAddress: contractAddress,
		finalizeOperation: &finalizeOperation{
			Domain:         domain,
			TransactionID:  transactionID,
			FailureMessage: failureMessage,
			Originator:     originator,
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
	receiptsToDistribute := make([]*components.ReceiptInputWithOriginator, 0, len(finalizeOperations))
	for _, op := range finalizeOperations {
		if op.FailureMessage != "" {
			receiptsToDistribute = append(receiptsToDistribute, &components.ReceiptInputWithOriginator{
				Originator: op.Originator,
				ReceiptInput: components.ReceiptInput{
					ReceiptType:    components.RT_FailedWithMessage,
					Domain:         op.Domain,
					TransactionID:  op.TransactionID,
					FailureMessage: op.FailureMessage,
				},
			})
		}
	}
	return s.WriteOrDistributeReceipts(ctx, dbTX, receiptsToDistribute)

}

func (s *syncPoints) WriteOrDistributeReceipts(ctx context.Context, dbTX persistence.DBTX, receipts []*components.ReceiptInputWithOriginator) error {

	// Receipts need to go back to their originator, so we either store the receive ourselves locally - or
	// push it in a reliable message back to the sender.
	localFailureReceipts := make([]*components.ReceiptInput, 0)
	remoteSends := make([]*pldapi.ReliableMessage, 0)
	for _, r := range receipts {
		if r.FailureMessage != "" {
			node, _ := pldtypes.PrivateIdentityLocator(r.Originator).Node(ctx, true)
			log.L(ctx).Warnf("Failure receipt %s for node %s: %s", r.TransactionID, node, r.FailureMessage)
			if node != "" && node != s.transportMgr.LocalNodeName() {
				remoteSends = append(remoteSends, &pldapi.ReliableMessage{
					Node:        node,
					MessageType: pldapi.RMTReceipt.Enum(),
					Metadata:    pldtypes.JSONString(&r.ReceiptInput),
				})
			} else {
				localFailureReceipts = append(localFailureReceipts, &r.ReceiptInput)
			}
		}
	}
	var err error
	if len(localFailureReceipts) > 0 {
		err = s.txMgr.FinalizeTransactions(ctx, dbTX, localFailureReceipts)
	}
	if err == nil && len(remoteSends) > 0 {
		// We log and ignore errors here, because if it is a DB transaction error we will
		// fail the DB transaction. If it is a peer resolution error - then we are not
		// going to be able to get this receipt back. So we can only log it.
		if peerErr := s.transportMgr.SendReliable(ctx, dbTX, remoteSends...); peerErr != nil {
			log.L(ctx).Errorf("Failed to send receipts back to node: %s", peerErr)
		}
	}
	return err

}
