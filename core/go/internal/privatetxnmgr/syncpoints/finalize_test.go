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
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func TestWriteFinalizeOperations(t *testing.T) {
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)
	testRevertReason := "test error"
	testTxnID := uuid.New()
	testContractAddress := tktypes.RandAddress()

	finalizeOperationsByContractAddress := map[tktypes.EthAddress][]*finalizeOperation{
		*testContractAddress: {
			{
				TransactionID:  testTxnID,
				FailureMessage: testRevertReason,
			},
		},
	}
	dbTX := m.persistence.P.DB()

	expectedReceipts := []*components.ReceiptInput{
		{
			ReceiptType:     components.RT_FailedWithMessage,
			ContractAddress: testContractAddress,
			TransactionID:   testTxnID,
			FailureMessage:  testRevertReason,
		},
	}

	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts).Return(nil)
	err := s.writeFinalizeOperations(ctx, dbTX, finalizeOperationsByContractAddress)
	assert.NoError(t, err)
}
