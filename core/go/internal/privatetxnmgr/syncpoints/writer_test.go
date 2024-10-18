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
	"github.com/stretchr/testify/require"
)

func TestRunBatchFinalizeOperations(t *testing.T) {
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)

	testRevertReason := "test error"
	testTxnID := uuid.New()
	testContractAddress := tktypes.RandAddress()
	dbTX := m.persistence.P.DB()
	testSyncPointOperations := []*syncPointOperation{
		{
			contractAddress: *testContractAddress,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID,
				FailureMessage: testRevertReason,
			},
		},
	}

	expectedReceipts := []*components.ReceiptInput{
		{
			ReceiptType:     components.RT_FailedWithMessage,
			ContractAddress: testContractAddress,
			TransactionID:   testTxnID,
			FailureMessage:  testRevertReason,
		},
	}

	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts).Return(nil)

	res, err := s.runBatch(ctx, dbTX, testSyncPointOperations)
	assert.NoError(t, err)
	require.Len(t, res, 1)

}

func TestRunBatchFinalizeOperationsMixedContractAddresses(t *testing.T) {
	//given that multiple WriteKeys can be matched to a single worker, there is no
	//guarantee that the contract address will be the same for all the operations in one batch
	// so need to make sure we handle this case
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)

	testRevertReason1 := "test error1"
	testRevertReason2a := "test error2a"
	testRevertReason2b := "test error2b"
	testTxnID1 := uuid.New()
	testTxnID2a := uuid.New()
	testTxnID2b := uuid.New()
	testContractAddress1 := tktypes.RandAddress()
	testContractAddress2 := tktypes.RandAddress()
	dbTX := m.persistence.P.DB()
	testSyncPointOperations := []*syncPointOperation{
		{
			contractAddress: *testContractAddress1,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID1,
				FailureMessage: testRevertReason1,
			},
		},
		{
			contractAddress: *testContractAddress2,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID2a,
				FailureMessage: testRevertReason2a,
			},
		},
		{
			contractAddress: *testContractAddress2,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID2b,
				FailureMessage: testRevertReason2b,
			},
		},
	}

	expectedReceipts1 := []*components.ReceiptInput{
		{
			ReceiptType:     components.RT_FailedWithMessage,
			ContractAddress: testContractAddress1,
			TransactionID:   testTxnID1,
			FailureMessage:  testRevertReason1,
		},
	}
	expectedReceipts2 := []*components.ReceiptInput{
		{
			ReceiptType:     components.RT_FailedWithMessage,
			ContractAddress: testContractAddress2,
			TransactionID:   testTxnID2a,
			FailureMessage:  testRevertReason2a,
		},
		{
			ReceiptType:     components.RT_FailedWithMessage,
			ContractAddress: testContractAddress2,
			TransactionID:   testTxnID2b,
			FailureMessage:  testRevertReason2b,
		},
	}

	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts1).Return(nil)
	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts2).Return(nil)

	res, err := s.runBatch(ctx, dbTX, testSyncPointOperations)
	assert.NoError(t, err)
	require.Len(t, res, 3)

}
