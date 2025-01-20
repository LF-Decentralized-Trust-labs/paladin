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

package statemgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	domainManager *componentmocks.DomainManager
	txManager     *componentmocks.TXManager
	allComponents *componentmocks.AllComponents
}

func newMockComponents(t *testing.T) *mockComponents {
	m := &mockComponents{}
	m.domainManager = componentmocks.NewDomainManager(t)
	m.txManager = componentmocks.NewTXManager(t)
	m.allComponents = componentmocks.NewAllComponents(t)
	m.allComponents.On("DomainManager").Return(m.domainManager)
	m.allComponents.On("TxManager").Return(m.txManager)
	return m
}

func newDBTestStateManager(t *testing.T) (context.Context, *stateManager, *mockComponents, func()) {
	ctx := context.Background()
	p, pDone, err := persistence.NewUnitTestPersistence(ctx, "statemgr")
	require.NoError(t, err)
	ss := NewStateManager(ctx, &pldconf.StateStoreConfig{}, p)

	m := newMockComponents(t)

	ir, err := ss.PreInit(m.allComponents)
	require.NoError(t, err)
	require.NotNil(t, ir)
	require.NotEmpty(t, ir.RPCModules)

	err = ss.PostInit(m.allComponents)
	require.NoError(t, err)

	err = ss.Start()
	require.NoError(t, err)

	return ctx, ss.(*stateManager), m, func() {
		ss.Stop()
		pDone()
	}
}

func newDBMockStateManager(t *testing.T) (context.Context, *stateManager, sqlmock.Sqlmock, *mockComponents, func()) {
	ctx := context.Background()
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	ss := NewStateManager(ctx, &pldconf.StateStoreConfig{}, p.P)

	m := newMockComponents(t)

	_, err = ss.PreInit(m.allComponents)
	require.NoError(t, err)

	err = ss.PostInit(m.allComponents)
	require.NoError(t, err)

	err = ss.Start()
	require.NoError(t, err)

	return ctx, ss.(*stateManager), p.Mock, m, func() {
		ss.Stop()
		require.NoError(t, p.Mock.ExpectationsWereMet())
	}
}

func TestGetTransactionStatesUnavailable(t *testing.T) {

	ctx, ss, _, done := newDBTestStateManager(t)
	defer done()

	txID := uuid.New()
	stateID1 := tktypes.HexBytes(tktypes.RandBytes(32))
	stateID2 := tktypes.HexBytes(tktypes.RandBytes(32))
	stateID3 := tktypes.HexBytes(tktypes.RandBytes(32))
	stateID4 := tktypes.HexBytes(tktypes.RandBytes(32))

	err := ss.WriteStateFinalizations(ctx, ss.p.DB(),
		[]*pldapi.StateSpendRecord{
			{DomainName: "domain1", State: stateID1, Transaction: txID},
		},
		[]*pldapi.StateReadRecord{
			{DomainName: "domain1", State: stateID2, Transaction: txID},
		},
		[]*pldapi.StateConfirmRecord{
			{DomainName: "domain1", State: stateID3, Transaction: txID},
		},
		[]*pldapi.StateInfoRecord{
			{DomainName: "domain1", State: stateID4, Transaction: txID},
		})
	require.NoError(t, err)

	txStates, err := ss.GetTransactionStates(ctx, ss.p.DB(), txID)
	require.NoError(t, err)
	require.Empty(t, txStates.Spent)
	require.Empty(t, txStates.Read)
	require.Empty(t, txStates.Confirmed)
	require.Equal(t, []tktypes.HexBytes{stateID1}, txStates.Unavailable.Spent)
	require.Equal(t, []tktypes.HexBytes{stateID2}, txStates.Unavailable.Read)
	require.Equal(t, []tktypes.HexBytes{stateID3}, txStates.Unavailable.Confirmed)
	require.Equal(t, []tktypes.HexBytes{stateID4}, txStates.Unavailable.Info)
}

func TestGetTransactionStatesFail(t *testing.T) {

	ctx, ss, db, _, done := newDBMockStateManager(t)
	defer done()

	db.ExpectQuery("SELECT.*states").WillReturnError(fmt.Errorf("pop"))

	_, err := ss.GetTransactionStates(ctx, ss.p.DB(), uuid.New())
	assert.Regexp(t, "pop", err)
}
