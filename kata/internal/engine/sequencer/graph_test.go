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

package sequencer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTransactions(t *testing.T) {
	ctx := context.Background()

	testGraph := NewGraph()
	err := testGraph.AddTransaction(ctx, "tx0", []string{}, []string{"S0"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx1", []string{}, []string{"S1A", "S1B"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx2", []string{}, []string{"S2"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx3", []string{"S0", "S1A"}, []string{"S3"})
	require.NoError(t, err)

	assert.True(t, testGraph.IncludesTransaction("tx0"))
	assert.True(t, testGraph.IncludesTransaction("tx1"))
	assert.True(t, testGraph.IncludesTransaction("tx2"))
	assert.True(t, testGraph.IncludesTransaction("tx3"))

}

func TestRemoveTransactions(t *testing.T) {
	ctx := context.Background()

	testGraph := NewGraph()
	err := testGraph.AddTransaction(ctx, "tx0", []string{}, []string{"S0"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx1", []string{}, []string{"S1A", "S1B"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx2", []string{}, []string{"S2"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx3", []string{"S0", "S1A"}, []string{"S3"})
	require.NoError(t, err)

	err = testGraph.RemoveTransactions(ctx, []string{"tx1", "tx2"})
	assert.NoError(t, err)
	assert.True(t, testGraph.IncludesTransaction("tx0"))
	assert.False(t, testGraph.IncludesTransaction("tx1"))
	assert.False(t, testGraph.IncludesTransaction("tx2"))
	assert.True(t, testGraph.IncludesTransaction("tx3"))

}

func TestGetDispatchableTransactions(t *testing.T) {
	ctx := context.Background()

	// build the matrix by hand

	testTransactions := []*transaction{
		{
			id:       uuid.New().String(),
			endorsed: true,
		},
		{
			id:       uuid.New().String(),
			endorsed: true,
		},
		{
			id:       uuid.New().String(),
			endorsed: false,
		},
		{
			id:       uuid.New().String(),
			endorsed: true,
		},
		{
			id:       uuid.New().String(),
			endorsed: true,
		},
	}

	transactionsMatrix := make([][][]string, 5)
	for i := 0; i < 5; i++ {
		transactionsMatrix[i] = make([][]string, 5)
	}

	//state hash is not important for this test
	transactionsMatrix[0][3] = []string{"foo"}
	transactionsMatrix[1][3] = []string{"foo"}
	transactionsMatrix[1][4] = []string{"foo"}
	transactionsMatrix[2][4] = []string{"foo"}

	testGraph := graph{
		transactions:       testTransactions,
		transactionsMatrix: transactionsMatrix,
	}

	dispatchableTransactions, err := testGraph.GetDispatchableTransactions(ctx)
	assert.NoError(t, err)
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// stricly speaking, the absolute order does not matter so long as dependencies come before dependants.
	//However we also check here that the breadth first search is favouring older transactions as expected
	assert.Equal(t, testTransactions[0].id, dispatchableTransactions[0])
	assert.Equal(t, testTransactions[1].id, dispatchableTransactions[1])
	//transaction 2 is not endorsed so should not be in the dispatchable list
	assert.Equal(t, testTransactions[3].id, dispatchableTransactions[2])
	//transaction 4 has a dependency on 2 so should not be in the dispatchable list

}

func TestScenario1(t *testing.T) {
	// build the matrix by adding transactions
	ctx := context.Background()
	testGraph := NewGraph()
	err := testGraph.AddTransaction(ctx, "tx0", []string{}, []string{"S0"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx1", []string{}, []string{"S1A", "S1B"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx2", []string{}, []string{"S2"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx3", []string{"S0", "S1A"}, []string{"S3"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx4", []string{"S1B", "S2"}, []string{"S4"})
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx0")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx1")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx3")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx4")
	require.NoError(t, err)

	dispatchableTransactions, err := testGraph.GetDispatchableTransactions(ctx)
	assert.NoError(t, err)
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	assert.True(t, ((dispatchableTransactions[0] == "tx0" && dispatchableTransactions[1] == "tx1") ||
		(dispatchableTransactions[0] == "tx1" && dispatchableTransactions[1] == "tx0")))

	//transaction 2 is not endorsed so should not be in the dispatchable list
	assert.Equal(t, "tx3", dispatchableTransactions[2])

	// GetDispatchableTransactions is a read only operation so we can call it again and get the same result
	dispatchableTransactions, err = testGraph.GetDispatchableTransactions(ctx)
	assert.NoError(t, err)
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	assert.True(t, ((dispatchableTransactions[0] == "tx0" && dispatchableTransactions[1] == "tx1") ||
		(dispatchableTransactions[0] == "tx1" && dispatchableTransactions[1] == "tx0")))
	assert.Equal(t, "tx3", dispatchableTransactions[2])

	err = testGraph.RemoveTransactions(ctx, dispatchableTransactions)
	assert.NoError(t, err)

	dispatchableTransactions, err = testGraph.GetDispatchableTransactions(ctx)
	assert.NoError(t, err)
	assert.Len(t, dispatchableTransactions, 0)

}
