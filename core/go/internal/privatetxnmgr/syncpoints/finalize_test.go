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

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestWriteFinalizeOperations(t *testing.T) {
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)
	testRevertReason := "test error"
	testTxnID := uuid.New()

	finalizeOperations := []*finalizeOperation{
		{
			TransactionID:  testTxnID,
			FailureMessage: testRevertReason,
		},
	}

	expectedReceipts := []*components.ReceiptInput{
		{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  testTxnID,
			FailureMessage: testRevertReason,
		},
	}

	m.txMgr.On("FinalizeTransactions", mock.Anything, mock.Anything, expectedReceipts).Return(nil)
	m.persistence.Mock.ExpectBegin()
	m.persistence.Mock.ExpectCommit()

	err := m.persistence.P.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return s.writeFailureOperations(ctx, dbTX, finalizeOperations)
	})
	assert.NoError(t, err)
}
