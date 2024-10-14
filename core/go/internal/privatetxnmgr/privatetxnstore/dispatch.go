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

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

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

// PersistDispatches persists the dispatches to the store and coordinates with the public transaction manager
// to submit public transactions.
func (s *store) PersistDispatchBatch(ctx context.Context, contractAddress tktypes.EthAddress, dispatchBatch *DispatchBatch, stateDistributions []*statedistribution.StateDistribution) error {

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
	op := s.writer.Queue(ctx, &dispatchSequenceOperation{
		contractAddress:    contractAddress,
		dispatches:         dispatchBatch.DispatchSequences,
		stateDistributions: stateDistributionsPersisted,
	})

	//wait for the flush to complete
	_, err := op.WaitFlushed(ctx)
	return err
}
