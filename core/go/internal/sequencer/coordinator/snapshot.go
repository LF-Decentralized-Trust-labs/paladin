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

package coordinator

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator/transaction"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
)

func action_SendHeartbeat(ctx context.Context, c *coordinator) error {
	return c.sendHeartbeat(ctx, c.contractAddress)
}

func (c *coordinator) sendHeartbeat(ctx context.Context, contractAddress *pldtypes.EthAddress) error {
	snapshot := c.getSnapshot(ctx)
	log.L(ctx).Debugf("sending heartbeats for sequencer %s", contractAddress.String())
	var err error
	c.originatorNodePoolMutex.RLock()
	defer c.originatorNodePoolMutex.RUnlock()
	for _, node := range c.originatorNodePool {
		if node != c.nodeName {
			log.L(ctx).Debugf("sending heartbeat to %s", node)
			err = c.transportWriter.SendHeartbeat(ctx, node, contractAddress, snapshot)
			if err != nil {
				log.L(ctx).Errorf("error sending heartbeat to %s: %v", node, err)
			}
		}
	}
	return err
}

func (c *coordinator) getSnapshot(ctx context.Context) *common.CoordinatorSnapshot {
	log.L(ctx).Infof("creating snapshot for sequencer %s", c.contractAddress.String())
	// This function is called from the sequencer loop so is safe to read internal state
	pooledTransactions := make([]*common.Transaction, 0, len(c.transactionsByID))
	dispatchedTransactions := make([]*common.DispatchedTransaction, 0, len(c.transactionsByID))
	confirmedTransactions := make([]*common.ConfirmedTransaction, 0, len(c.transactionsByID))

	//Snapshot contains a coarse grained view of transactions state.
	// All known transactions fall into one of 3 categories
	// 1. Pooled transactions - these are transactions that have been delegated but not yet dispatched
	// 2. Dispatched transactions - these are transactions that are past the point of no return, the precise status (ready for collection, dispatched, nonce assigned, submitted to a blockchain node) is dependant on parallel processing from this point onward
	// 3. Confirmed transactions - these are transactions that have been confirmed by the network
	for _, txn := range c.transactionsByID {
		log.L(ctx).Infof("next transaction to assess current status of %s. Current state: %s", txn.ID.String(), txn.GetCurrentState().String())
		switch txn.GetCurrentState() {
		// pooled transactions are those that have been delegated but not yet dispatched, this includes the various states from being delegated up to being ready for dispatch
		case transaction.State_Reverted:
			//NOOP - this transaction is just waiting to be cleaned up so we don't include it in the snapshot
		case transaction.State_Blocked:
			fallthrough
		case transaction.State_Confirming_Dispatchable:
			fallthrough
		case transaction.State_Endorsement_Gathering:
			fallthrough
		case transaction.State_PreAssembly_Blocked:
			fallthrough
		case transaction.State_Assembling:
			fallthrough
		case transaction.State_Pooled:
			pooledTransactions = append(pooledTransactions, &common.Transaction{
				ID: txn.ID,
			})
		case transaction.State_Ready_For_Dispatch:
			//this is already past the point of no return.  It is as good as dispatched, just waiting for the the dispatcher thread to collect it so we include it in the dispatched transactions
			// of the snapshot
			fallthrough
		case transaction.State_Submitted:
			fallthrough
		case transaction.State_SubmissionPrepared:
			fallthrough
		case transaction.State_Dispatched:
			dispatchedTransaction := &common.DispatchedTransaction{}
			dispatchedTransaction.ID = txn.ID
			dispatchedTransaction.Originator = txn.Originator()
			signerAddressPtr := txn.GetSignerAddress()
			if signerAddressPtr != nil {
				dispatchedTransaction.Signer = *signerAddressPtr
				dispatchedTransaction.Nonce = txn.GetNonce()
				dispatchedTransaction.LatestSubmissionHash = txn.GetLatestSubmissionHash()
			} else {
				log.L(ctx).Warnf("Transaction %s has no signer address", txn.ID)
			}

			dispatchedTransactions = append(dispatchedTransactions, dispatchedTransaction)

		case transaction.State_Confirmed:
			log.L(ctx).Infof("heartbeat snapshot building, transaction ID %s is in State_Confirmed, sending to heartbeat receipients", txn.ID.String())
			confirmedTransaction := &common.ConfirmedTransaction{}
			confirmedTransaction.ID = txn.ID

			signerAddressPtr := txn.GetSignerAddress()
			if signerAddressPtr != nil {
				confirmedTransaction.Signer = *signerAddressPtr
			} else {
				log.L(ctx).Warnf("Transaction %s has no signer address", txn.ID)
			}
			confirmedTransaction.Nonce = txn.GetNonce()
			confirmedTransaction.LatestSubmissionHash = txn.GetLatestSubmissionHash()
			confirmedTransaction.RevertReason = txn.GetRevertReason()
			confirmedTransactions = append(confirmedTransactions, confirmedTransaction)
		}

	}
	return &common.CoordinatorSnapshot{
		//FlushPoints:            flushPoints,
		DispatchedTransactions: dispatchedTransactions,
		PooledTransactions:     pooledTransactions,
		ConfirmedTransactions:  confirmedTransactions,
		//CoordinatorState:       state,
		//BlockHeight:            blockHeight,
	}
}
