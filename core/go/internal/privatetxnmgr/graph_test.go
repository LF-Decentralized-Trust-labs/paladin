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

package privatetxnmgr

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func NewMockTransactionProcessorForTesting(t *testing.T, transactionID uuid.UUID, inputStateIDs []string, outputStateIDs []string, endorsed bool, signer string) *ptmgrtypes.MockTransactionFlow {
	mockTransactionProcessor := ptmgrtypes.NewMockTransactionFlow(t)
	mockTransactionProcessor.On("ID", mock.Anything).Return(transactionID).Maybe()
	mockTransactionProcessor.On("InputStateIDs", mock.Anything).Return(inputStateIDs).Maybe()
	mockTransactionProcessor.On("OutputStateIDs", mock.Anything).Return(outputStateIDs).Maybe()
	mockTransactionProcessor.On("IsEndorsed", mock.Anything, mock.Anything).Return(endorsed).Maybe()
	mockTransactionProcessor.On("Signer", mock.Anything).Return(signer).Maybe()
	return mockTransactionProcessor
}

func TestAddTransactions(t *testing.T) {
	ctx := context.Background()
	signer := pldtypes.RandHex(32)

	TxID0 := uuid.New()
	mockTransactionProcessor0 := NewMockTransactionProcessorForTesting(t, TxID0, []string{}, []string{"S0"}, false, signer)

	TxID1 := uuid.New()
	mockTransactionProcessor1 := NewMockTransactionProcessorForTesting(t, TxID1, []string{}, []string{"S1A", "S1B"}, false, signer)

	TxID2 := uuid.New()
	mockTransactionProcessor2 := NewMockTransactionProcessorForTesting(t, TxID2, []string{}, []string{"S2"}, false, signer)

	TxID3 := uuid.New()
	mockTransactionProcessor3 := NewMockTransactionProcessorForTesting(t, TxID3, []string{"S0", "S1A"}, []string{"S3"}, false, signer)

	testGraph := NewGraph()
	testGraph.AddTransaction(ctx, mockTransactionProcessor0)
	testGraph.AddTransaction(ctx, mockTransactionProcessor1)
	testGraph.AddTransaction(ctx, mockTransactionProcessor2)
	testGraph.AddTransaction(ctx, mockTransactionProcessor3)

	assert.True(t, testGraph.IncludesTransaction(TxID0.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID1.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID2.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID3.String()))

}

func TestRemoveTransactions(t *testing.T) {
	ctx := context.Background()

	testGraph := NewGraph()
	signer := pldtypes.RandHex(32)

	TxID0 := uuid.New()
	mockTransactionProcessor0 := NewMockTransactionProcessorForTesting(t, TxID0, []string{}, []string{"S0"}, false, signer)

	TxID1 := uuid.New()
	mockTransactionProcessor1 := NewMockTransactionProcessorForTesting(t, TxID1, []string{}, []string{"S1A", "S1B"}, false, signer)

	TxID2 := uuid.New()
	mockTransactionProcessor2 := NewMockTransactionProcessorForTesting(t, TxID2, []string{}, []string{"S2"}, false, signer)

	TxID3 := uuid.New()
	mockTransactionProcessor3 := NewMockTransactionProcessorForTesting(t, TxID3, []string{"S0", "S1A"}, []string{"S3"}, false, signer)

	testGraph.AddTransaction(ctx, mockTransactionProcessor0)
	testGraph.AddTransaction(ctx, mockTransactionProcessor1)
	testGraph.AddTransaction(ctx, mockTransactionProcessor2)
	testGraph.AddTransaction(ctx, mockTransactionProcessor3)

	testGraph.RemoveTransactions(ctx, []string{"tx1", "tx2"})

	assert.True(t, testGraph.IncludesTransaction(TxID0.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID1.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID2.String()))
	assert.True(t, testGraph.IncludesTransaction(TxID3.String()))

}

func TestScenario1(t *testing.T) {
	// 5 transactions, 0,1,2,3,4
	// 3 depends on 0 and 1
	// 4 depends on 1 and 2
	// 0, 1, 3 and 4 are endorsed
	// only 0, 1 and 3 should be dispatchable (because 4 depends on 2 which is not endorsed)

	// 3 is sequenced after both 0 and 1 because it depends on both
	// order of 0 and 1 does not matter because they are not dependent on each other

	// build the matrix by adding transactions
	ctx := context.Background()
	testGraph := NewGraph()
	signer := pldtypes.RandHex(32)

	TxID0 := uuid.New()
	mockTransactionProcessor0 := NewMockTransactionProcessorForTesting(t, TxID0, []string{}, []string{"S0"}, true, signer)

	TxID1 := uuid.New()
	mockTransactionProcessor1 := NewMockTransactionProcessorForTesting(t, TxID1, []string{}, []string{"S1A", "S1B"}, true, signer)

	//Only 2 is not endorsed
	TxID2 := uuid.New()
	mockTransactionProcessor2 := NewMockTransactionProcessorForTesting(t, TxID2, []string{}, []string{"S2"}, false, signer)

	TxID3 := uuid.New()
	mockTransactionProcessor3 := NewMockTransactionProcessorForTesting(t, TxID3, []string{"S0", "S1A"}, []string{"S3"}, true, signer)

	TxID4 := uuid.New()
	mockTransactionProcessor4 := NewMockTransactionProcessorForTesting(t, TxID4, []string{"S1B", "S2"}, []string{"S4"}, true, signer)

	testGraph.AddTransaction(ctx, mockTransactionProcessor0)
	testGraph.AddTransaction(ctx, mockTransactionProcessor1)
	testGraph.AddTransaction(ctx, mockTransactionProcessor2)
	testGraph.AddTransaction(ctx, mockTransactionProcessor3)
	testGraph.AddTransaction(ctx, mockTransactionProcessor4)

	dispatchable, err := testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)

	assert.Len(t, dispatchable, 1)
	dispatchableTransactions := dispatchable[signer]

	require.NoError(t, err)
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	assert.True(t, ((dispatchableTransactions[0].ID(ctx) == TxID0 && dispatchableTransactions[1].ID(ctx) == TxID1) ||
		(dispatchableTransactions[0].ID(ctx) == TxID1 && dispatchableTransactions[1].ID(ctx) == TxID0)))

	//transaction 2 is not endorsed so should not be in the dispatchable list
	assert.Equal(t, TxID3, dispatchableTransactions[2].ID(ctx))

	// GetDispatchableTransactions is a read only operation so we can call it again and get the same result
	dispatchable, err = testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, dispatchable, 1)

	dispatchableTransactions = dispatchable[signer]
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	assert.True(t, ((dispatchableTransactions[0].ID(ctx) == TxID0 && dispatchableTransactions[1].ID(ctx) == TxID1) ||
		(dispatchableTransactions[0].ID(ctx) == TxID1 && dispatchableTransactions[1].ID(ctx) == TxID0)))
	assert.Equal(t, TxID3, dispatchableTransactions[2].ID(ctx))

	testGraph.RemoveTransactions(ctx, dispatchable.IDs(ctx))

	dispatchable, err = testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, dispatchable, 0)
}

func TestScenario2(t *testing.T) {
	// Test the breadth first search
	// 6 transactions, 0,1,2,3,4, 5
	// 4 depends on 2 which depends on 0
	// 5 depends on 3 which depends on 1
	// all are endorsed

	// ordering should respect the dependency relationships ( 4 before 2, 2 before 0, 5 before 3, 3 before 1)
	// ordering should also bias towards transactions closer to the independent edge of the graph (1 before 2 and 0 before 3 even though they are not directly dependent)

	// build the matrix by adding transactions
	ctx := context.Background()
	testGraph := NewGraph()
	signer := pldtypes.RandHex(32)

	TxID0 := uuid.New()
	mockTransactionProcessor0 := NewMockTransactionProcessorForTesting(t, TxID0, []string{}, []string{"S0"}, true, signer)

	TxID1 := uuid.New()
	mockTransactionProcessor1 := NewMockTransactionProcessorForTesting(t, TxID1, []string{}, []string{"S1"}, true, signer)

	//Only 2 is not endorsed
	TxID2 := uuid.New()
	mockTransactionProcessor2 := NewMockTransactionProcessorForTesting(t, TxID2, []string{"S0"}, []string{"S2"}, true, signer)

	TxID3 := uuid.New()
	mockTransactionProcessor3 := NewMockTransactionProcessorForTesting(t, TxID3, []string{"S1"}, []string{"S3"}, true, signer)

	TxID4 := uuid.New()
	mockTransactionProcessor4 := NewMockTransactionProcessorForTesting(t, TxID4, []string{"S2"}, []string{"S4"}, true, signer)

	TxID5 := uuid.New()
	mockTransactionProcessor5 := NewMockTransactionProcessorForTesting(t, TxID5, []string{"S3"}, []string{"S5"}, true, signer)

	testGraph.AddTransaction(ctx, mockTransactionProcessor0)
	testGraph.AddTransaction(ctx, mockTransactionProcessor1)
	testGraph.AddTransaction(ctx, mockTransactionProcessor2)
	testGraph.AddTransaction(ctx, mockTransactionProcessor3)
	testGraph.AddTransaction(ctx, mockTransactionProcessor4)
	testGraph.AddTransaction(ctx, mockTransactionProcessor5)

	dispatchable, err := testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, dispatchable, 1)

	dispatchableTransactions := dispatchable[signer]
	require.Len(t, dispatchableTransactions, 6)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	isBefore := func(tx1, tx2 string) bool {
		foundTx1 := false
		for _, tx := range dispatchableTransactions {
			switch tx.ID(ctx).String() {
			case tx1:
				foundTx1 = true
			case tx2:
				return foundTx1
			}
		}
		assert.Failf(t, "%s not found", tx2)
		return false
	}
	assert.True(t, isBefore(TxID0.String(), TxID2.String()))
	assert.True(t, isBefore(TxID0.String(), TxID3.String()))
	assert.True(t, isBefore(TxID1.String(), TxID2.String()))
	assert.True(t, isBefore(TxID1.String(), TxID2.String()))
	assert.True(t, isBefore(TxID2.String(), TxID4.String()))
	assert.True(t, isBefore(TxID2.String(), TxID5.String()))
	assert.True(t, isBefore(TxID3.String(), TxID4.String()))
	assert.True(t, isBefore(TxID3.String(), TxID5.String()))

}
