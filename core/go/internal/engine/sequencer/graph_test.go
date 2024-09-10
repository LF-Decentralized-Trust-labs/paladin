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
	require.NoError(t, err)
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
	require.NoError(t, err)
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
	require.NoError(t, err)
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
	require.NoError(t, err)
	require.Len(t, dispatchableTransactions, 3)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	assert.True(t, ((dispatchableTransactions[0] == "tx0" && dispatchableTransactions[1] == "tx1") ||
		(dispatchableTransactions[0] == "tx1" && dispatchableTransactions[1] == "tx0")))
	assert.Equal(t, "tx3", dispatchableTransactions[2])

	err = testGraph.RemoveTransactions(ctx, dispatchableTransactions)
	require.NoError(t, err)

	dispatchableTransactions, err = testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, dispatchableTransactions, 0)

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
	err := testGraph.AddTransaction(ctx, "tx0", []string{}, []string{"S0"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx1", []string{}, []string{"S1"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx2", []string{"S0"}, []string{"S2"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx3", []string{"S1"}, []string{"S3"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx4", []string{"S2"}, []string{"S4"})
	require.NoError(t, err)

	err = testGraph.AddTransaction(ctx, "tx5", []string{"S3"}, []string{"S5"})
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx0")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx1")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx2")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx3")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx4")
	require.NoError(t, err)

	err = testGraph.RecordEndorsement(ctx, "tx5")
	require.NoError(t, err)

	dispatchableTransactions, err := testGraph.GetDispatchableTransactions(ctx)
	require.NoError(t, err)
	require.Len(t, dispatchableTransactions, 6)

	//make sure they come out in the expected order
	// the absolute order does not matter so long as dependencies come before dependants.
	// so we expect either 0,1,3 or 1,0,3.
	isBefore := func(tx1, tx2 string) bool {
		foundTx1 := false
		for _, tx := range dispatchableTransactions {
			switch tx {
			case tx1:
				foundTx1 = true
			case tx2:
				return foundTx1
			}
		}
		assert.Failf(t, "%s not found", tx2)
		return false
	}
	assert.True(t, isBefore("tx0", "tx2"))
	assert.True(t, isBefore("tx0", "tx3"))
	assert.True(t, isBefore("tx1", "tx2"))
	assert.True(t, isBefore("tx1", "tx3"))
	assert.True(t, isBefore("tx2", "tx4"))
	assert.True(t, isBefore("tx2", "tx5"))
	assert.True(t, isBefore("tx3", "tx4"))
	assert.True(t, isBefore("tx3", "tx5"))

}
