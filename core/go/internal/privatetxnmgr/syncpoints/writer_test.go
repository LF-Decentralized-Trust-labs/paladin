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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestDomainContextWithFlush(t *testing.T) (dc *componentmocks.DomainContext, flushResult chan error) {
	dc = componentmocks.NewDomainContext(t)
	dc.On("Info").Return(components.DomainContextInfo{
		ID: uuid.New(),
	}).Maybe()

	flushResult = make(chan error, 1)
	dc.On("Flush", mock.Anything).Return(func(err error) {
		flushResult <- err
	}, nil)
	return
}

func TestRunBatchFinalizeOperations(t *testing.T) {
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)

	dc, flushResult := newTestDomainContextWithFlush(t)

	testRevertReason := "test error"
	testTxnID := uuid.New()
	testContractAddress := tktypes.RandAddress()
	dbTX := m.persistence.P.DB()
	testSyncPointOperations := []*syncPointOperation{
		{
			domainContext:   dc,
			contractAddress: *testContractAddress,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID,
				FailureMessage: testRevertReason,
			},
		},
	}

	expectedReceipts := []*components.ReceiptInput{
		{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  testTxnID,
			FailureMessage: testRevertReason,
		},
	}

	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts).Return(func() {}, nil)

	dbResultCB, res, err := s.runBatch(ctx, dbTX, testSyncPointOperations)
	assert.NoError(t, err)
	require.Len(t, res, 1)
	dbResultCB(nil)
	require.NoError(t, <-flushResult)

}

func TestRunBatchFinalizeOperationsMixedContractAddresses(t *testing.T) {

	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)

	dc, flushResult := newTestDomainContextWithFlush(t)

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
			domainContext:   dc,
			contractAddress: *testContractAddress1,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID1,
				FailureMessage: testRevertReason1,
			},
		},
		{
			domainContext:   dc,
			contractAddress: *testContractAddress2,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID2a,
				FailureMessage: testRevertReason2a,
			},
		},
		{
			domainContext:   dc,
			contractAddress: *testContractAddress2,
			finalizeOperation: &finalizeOperation{
				TransactionID:  testTxnID2b,
				FailureMessage: testRevertReason2b,
			},
		},
		{
			// This one is a success - which does NOT get passed to FinalizeTransactions as
			// the receipt is written by the Domain event indexer.
			domainContext:   dc,
			contractAddress: *testContractAddress2,
			finalizeOperation: &finalizeOperation{
				TransactionID: testTxnID2a,
			},
		},
	}

	expectedReceipts := []*components.ReceiptInput{
		{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  testTxnID1,
			FailureMessage: testRevertReason1,
		},
		{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  testTxnID2a,
			FailureMessage: testRevertReason2a,
		},
		{
			ReceiptType:    components.RT_FailedWithMessage,
			TransactionID:  testTxnID2b,
			FailureMessage: testRevertReason2b,
		},
	}

	m.txMgr.On("FinalizeTransactions", ctx, dbTX, expectedReceipts).Return(func() {}, nil)

	dbResultCB, res, err := s.runBatch(ctx, dbTX, testSyncPointOperations)
	assert.NoError(t, err)
	require.Len(t, res, 4)
	dbResultCB(nil)
	require.NoError(t, <-flushResult)

}
