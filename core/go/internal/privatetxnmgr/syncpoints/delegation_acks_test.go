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
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alecthomas/assert/v2"
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/require"
)

func TestWriteDelegationAcksOperations(t *testing.T) {
	ctx := context.Background()
	s, m := newSyncPointsForTesting(t)
	testDelegationID1 := uuid.New()
	testDelegationID2 := uuid.New()
	testContractAddress1 := tktypes.RandAddress()
	testContractAddress2 := tktypes.RandAddress()
	testSyncPointOperations := []*syncPointOperation{
		{
			contractAddress: *testContractAddress1,
			delegationAckOperation: &delegationAckOperation{
				DelegationID: testDelegationID1,
			},
		},
		{
			contractAddress: *testContractAddress2,
			delegationAckOperation: &delegationAckOperation{
				DelegationID: testDelegationID2,
			},
		},
	}
	dbTX := m.persistence.P.DB()
	m.persistence.Mock.ExpectExec("INSERT.*transaction_delegation_acknowledgements").WithArgs(
		sqlmock.AnyArg(), testDelegationID1,
		sqlmock.AnyArg(), testDelegationID2,
	).WillReturnResult(driver.ResultNoRows)

	res, err := s.runBatch(ctx, dbTX, testSyncPointOperations)
	assert.NoError(t, err)
	require.Len(t, res, 2)
	assert.NoError(t, m.persistence.Mock.ExpectationsWereMet())

}
