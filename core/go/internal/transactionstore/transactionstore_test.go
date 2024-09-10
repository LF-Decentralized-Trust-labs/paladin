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

package transactionstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
)

func newTestTransactionStore(t *testing.T) (context.Context, *transactionStore, func()) {
	ctx := context.Background()
	p, done, err := persistence.NewUnitTestPersistence(ctx)
	require.NoError(t, err)
	ts := NewTransactionStore(ctx, &Config{}, p)
	return ctx, ts.(*transactionStore), done
}

func createSomeRandomTransactions(t *testing.T, ctx context.Context, ts *transactionStore, count int) {
	for i := 0; i < count; i++ {
		from := uuid.New().String()
		contract := uuid.New().String()
		payloadJSON := fmt.Sprintf(`{"foo_%d":"bar"}`, count)
		txn := Transaction{
			From:        from,
			Contract:    contract,
			PayloadJSON: &payloadJSON,
		}

		createdTransaction, err := ts.InsertTransaction(ctx, txn)
		require.NoError(t, err)
		require.NotNil(t, createdTransaction)
	}
}

func TestRetrieveAllTransactionOK(t *testing.T) {

	ctx, ts, done := newTestTransactionStore(t)
	defer done()

	createSomeRandomTransactions(t, ctx, ts, 10)

	retreivedTxns, err := ts.GetAllTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, retreivedTxns, 10)
}

func TestStoreRetrieveTransaction(t *testing.T) {

	ctx, ts, done := newTestTransactionStore(t)
	defer done()

	createSomeRandomTransactions(t, ctx, ts, 10)

	from := uuid.New().String()
	contract := uuid.New().String()
	payloadJSON := `{"foo":"bar"}`
	txn := Transaction{
		From:        from,
		Contract:    contract,
		PayloadJSON: &payloadJSON,
	}

	createdTransaction, err := ts.InsertTransaction(ctx, txn)
	require.NoError(t, err)
	require.NotNil(t, createdTransaction)
	assert.NotEqual(t, uuid.Nil, createdTransaction.ID)
	txnID := createdTransaction.ID

	createSomeRandomTransactions(t, ctx, ts, 10)

	retreivedTxn, err := ts.GetTransactionByID(ctx, txnID)
	require.NoError(t, err)
	assert.Equal(t, txnID, retreivedTxn.ID)
	assert.Equal(t, from, retreivedTxn.From)
	assert.Equal(t, contract, retreivedTxn.Contract)
	assert.Equal(t, payloadJSON, *retreivedTxn.PayloadJSON)
}

func TestStoreDeleteTransaction(t *testing.T) {

	ctx, ts, done := newTestTransactionStore(t)
	defer done()
	createSomeRandomTransactions(t, ctx, ts, 10)

	from := uuid.New().String()
	contract := uuid.New().String()
	payloadJSON := `{"foo":"bar"}`
	txn := Transaction{
		From:        from,
		Contract:    contract,
		PayloadJSON: &payloadJSON,
	}

	createdTransaction, err := ts.InsertTransaction(ctx, txn)
	require.NoError(t, err)
	require.NotNil(t, createdTransaction)
	assert.NotEqual(t, uuid.Nil, createdTransaction.ID)

	createSomeRandomTransactions(t, ctx, ts, 10)

	txnID := createdTransaction.ID
	retreivedTxn, err := ts.GetTransactionByID(ctx, txnID)
	require.NoError(t, err)
	assert.Equal(t, txnID, retreivedTxn.ID)

	err = ts.DeleteTransaction(ctx, *retreivedTxn)

	require.NoError(t, err)

	retreivedTxn, err = ts.GetTransactionByID(ctx, txnID)
	assert.Error(t, err)
	assert.Nil(t, retreivedTxn)
	assert.Contains(t, err.Error(), "record not found")

}

func TestStoreUpdateTransaction(t *testing.T) {
	ctx, ts, done := newTestTransactionStore(t)
	defer done()
	createSomeRandomTransactions(t, ctx, ts, 10)

	from := uuid.New().String()
	contract := uuid.New().String()
	payloadJSON := `{"foo":"bar"}`
	txn := Transaction{
		From:        from,
		Contract:    contract,
		PayloadJSON: &payloadJSON,
	}

	createdTransaction, err := ts.InsertTransaction(ctx, txn)
	require.NoError(t, err)
	require.NotNil(t, createdTransaction)
	assert.NotEqual(t, uuid.Nil, createdTransaction.ID)

	createSomeRandomTransactions(t, ctx, ts, 10)

	txnID := createdTransaction.ID
	retreivedTxn, err := ts.GetTransactionByID(ctx, txnID)
	require.NoError(t, err)
	assert.Equal(t, txnID, retreivedTxn.ID)

	sequenceID := uuid.New()
	txnUpdate := Transaction{
		ID:          createdTransaction.ID,
		From:        from,
		Contract:    contract,
		PayloadJSON: &payloadJSON,
		SequenceID:  &sequenceID,
	}
	updatedTxn, err := ts.UpdateTransaction(ctx, txnUpdate)

	require.NoError(t, err)
	assert.NotNil(t, updatedTxn)

	retreivedTxn, err = ts.GetTransactionByID(ctx, txnID)
	require.NoError(t, err)
	assert.Equal(t, txnID, retreivedTxn.ID)
	assert.Equal(t, from, retreivedTxn.From)
	assert.Equal(t, contract, retreivedTxn.Contract)
	assert.Equal(t, payloadJSON, *retreivedTxn.PayloadJSON)
	assert.Equal(t, sequenceID, *retreivedTxn.SequenceID)
}
