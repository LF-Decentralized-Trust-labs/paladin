/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package groupmgr

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/statemgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	c                *componentsmocks.AllComponents
	db               *mockpersistence.SQLMockProvider
	p                persistence.Persistence
	stateManager     *componentsmocks.StateManager
	txManager        *componentsmocks.TXManager
	domainManager    *componentsmocks.DomainManager
	domain           *componentsmocks.Domain
	registryManager  *componentsmocks.RegistryManager
	transportManager *componentsmocks.TransportManager
}

func newMockComponents(t *testing.T, realDB bool) *mockComponents {
	mc := &mockComponents{c: componentsmocks.NewAllComponents(t)}
	mc.domainManager = componentsmocks.NewDomainManager(t)
	mc.domain = componentsmocks.NewDomain(t)
	mc.registryManager = componentsmocks.NewRegistryManager(t)
	mc.transportManager = componentsmocks.NewTransportManager(t)
	mc.txManager = componentsmocks.NewTXManager(t)

	mc.c.On("DomainManager").Return(mc.domainManager).Maybe()
	mc.c.On("TransportManager").Return(mc.transportManager).Maybe()
	mc.c.On("RegistryManager").Return(mc.registryManager).Maybe()
	mc.c.On("TxManager").Return(mc.txManager).Maybe()

	if realDB {
		p, cleanup, err := persistence.NewUnitTestPersistence(context.Background(), "groupmgr")
		require.NoError(t, err)
		t.Cleanup(cleanup)
		mc.p = p
		mc.c.On("Persistence").Return(p).Maybe()

		stateManager := statemgr.NewStateManager(context.Background(), &pldconf.StateStoreConfig{}, p)
		_, err = stateManager.PreInit(mc.c)
		require.NoError(t, err)
		err = stateManager.PostInit(mc.c)
		require.NoError(t, err)
		err = stateManager.Start()
		require.NoError(t, err)
		mc.c.On("StateManager").Return(stateManager).Maybe()
		t.Cleanup(stateManager.Stop)

	} else {
		mdb, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		mc.db = mdb
		mc.p = mdb.P
		mc.c.On("Persistence").Return(mc.p).Maybe()

		mc.stateManager = componentsmocks.NewStateManager(t)
		mc.c.On("StateManager").Return(mc.stateManager).Maybe()
	}

	mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(mc.domain, nil).Maybe()
	mc.domain.On("CustomHashFunction").Return(false).Maybe()
	mc.domain.On("Name").Return("domain1").Maybe()
	mc.txManager.On("NotifyStatesDBChanged", mock.Anything).Return().Maybe()
	mc.transportManager.On("LocalNodeName").Return("node1").Maybe()

	return mc
}

func newTestGroupManager(t *testing.T, realDB bool, conf *pldconf.GroupManagerConfig, extraSetup ...func(mc *mockComponents, conf *pldconf.GroupManagerConfig)) (context.Context, *groupManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.TraceLevel)

	mc := newMockComponents(t, realDB)
	for _, fn := range extraSetup {
		fn(mc, conf)
	}

	gm := NewGroupManager(ctx, conf)

	ir, err := gm.PreInit(mc.c)
	require.NoError(t, err)
	assert.NotNil(t, ir)

	err = gm.PostInit(mc.c)
	require.NoError(t, err)

	err = gm.Start()
	require.NoError(t, err)

	return ctx, gm.(*groupManager), mc, func() {
		if !t.Failed() {
			logrus.SetLevel(oldLevel)
			cancelCtx()
			gm.Stop()
		}
	}
}

func mockBeginRollback(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectRollback()
}

func TestPrivacyGroupNoMembers(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners, mockBeginRollback)
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain: "domain1",
		})
		return err
	})
	require.Regexp(t, "PD012501", err)
}

func TestPrivacyGroupNonQualfiedMembers(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners, mockBeginRollback)
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me", "you"},
		})
		return err
	})
	require.Regexp(t, "PD020017", err)
}

func TestPrivacyGroupNoTransportsForNode(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
				Return(nil, fmt.Errorf("nope"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "nope", err)
}

func TestPrivacyGroupInvalidDomain(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain2").Return(nil, fmt.Errorf("nope"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain2",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "nope", err)
}

func TestPrivacyGroupDomainConfigureFail(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, nil)
			mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestPrivacyGroupDomainEnsureSchemasFail(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, nil)
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, fmt.Errorf("pop"))
			mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(map[string]string{}, nil)
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestPrivacyGroupDomainInitFail(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, nil)
			ms := componentsmocks.NewSchema(t)
			ms.On("ID").Return(pldtypes.RandBytes32())
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return([]*pldapi.State{
					{StateBase: pldapi.StateBase{
						ID: pldtypes.RandBytes(32),
					}},
				}, nil)
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return([]components.Schema{ms}, nil)
			mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(map[string]string{}, nil)
			mc.domain.On("InitPrivacyGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestPrivacyGroupWriteStateFail(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, nil)
			mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(map[string]string{}, nil)
			ms := componentsmocks.NewSchema(t)
			ms.On("ID").Return(pldtypes.RandBytes32())
			mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return([]components.Schema{ms}, nil)
			mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
				Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func TestPrivacyGroupSendTransactionFail(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockBeginRollback,
		mockReadyToSendTransaction(t),
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)
}

func mockReadyToSendTransaction(t *testing.T) func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
	return func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
		mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").Return(nil, nil)
		mc.domain.On("ConfigurePrivacyGroup", mock.Anything, mock.Anything).Return(map[string]string{}, nil)
		mc.domain.On("InitPrivacyGroup", mock.Anything, mock.Anything, mock.Anything).
			Return(&pldapi.TransactionInput{
				TransactionBase: pldapi.TransactionBase{
					Domain: "domain1",
					Type:   pldapi.TransactionTypePrivate.Enum(),
				},
			}, nil)
		ms := componentsmocks.NewSchema(t)
		ms.On("ID").Return(pldtypes.RandBytes32())
		ms.On("Signature").Return("").Maybe()
		mc.stateManager.On("EnsureABISchemas", mock.Anything, mock.Anything, "domain1", mock.Anything).
			Return([]components.Schema{ms}, nil)
		mc.stateManager.On("WriteReceivedStates", mock.Anything, mock.Anything, "domain1", mock.Anything).
			Return([]*pldapi.State{
				{StateBase: pldapi.StateBase{
					ID: pldtypes.RandBytes(32),
				}},
			}, nil)
	}
}

func mockReadyToInsertGroup(t *testing.T) func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
	return func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
		mockReadyToSendTransaction(t)(mc, conf)
		mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return([]uuid.UUID{uuid.New()}, nil)
	}
}

func TestPrivacyGroupWriteGroupFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockReadyToInsertGroup(t),
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectExec("INSERT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))
			mc.db.Mock.ExpectRollback()
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)

	require.NoError(t, mc.db.Mock.ExpectationsWereMet())

}

func TestPrivacyGroupMembersWriteFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockReadyToInsertGroup(t),
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.db.Mock.ExpectBegin()
			mc.db.Mock.ExpectExec("INSERT.*privacy_groups").WillReturnResult(driver.ResultNoRows)
			mc.db.Mock.ExpectExec("INSERT.*privacy_group_members").WillReturnError(fmt.Errorf("pop"))
			mc.db.Mock.ExpectRollback()
		})
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)

	require.NoError(t, mc.db.Mock.ExpectationsWereMet())
}

func mockInsertPrivacyGroupOK(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectExec("INSERT.*privacy_groups").WillReturnResult(driver.ResultNoRows)
	mc.db.Mock.ExpectExec("INSERT.*privacy_group_members").WillReturnResult(driver.ResultNoRows)
	mc.db.Mock.ExpectRollback()
}

func TestPrivacyGroupSendReliableFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{},
		mockEmptyMessageListeners,
		mockReadyToInsertGroup(t),
		mockInsertPrivacyGroupOK,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("pop"))
		},
	)
	defer done()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := gm.CreateGroup(ctx, dbTX, &pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		})
		return err
	})
	require.Regexp(t, "pop", err)

	require.NoError(t, mc.db.Mock.ExpectationsWereMet())
}

func TestQueryGroupsFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.QueryGroups(ctx, gm.p.NOTX(), query.NewQueryBuilder().Limit(1).Query())
	require.Regexp(t, "pop", err)
}

func TestQueryGroupsEnrichMembersFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnRows(sqlmock.NewRows([]string{
		"domain",
		"id",
	}).AddRow(
		"domain1",
		pldtypes.RandBytes(32),
	))
	mc.db.Mock.ExpectQuery("SELECT.*privacy_group_members").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.QueryGroups(ctx, gm.p.NOTX(), query.NewQueryBuilder().Limit(1).Query())
	require.Regexp(t, "pop", err)
}

func TestGetGroupByIDFailDB(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.GetGroupByID(ctx, gm.p.NOTX(), "domain1", pldtypes.RandBytes(32))
	assert.Regexp(t, "pop", err)
}

func TestGetGroupByAddressFailDB(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.GetGroupByAddress(ctx, gm.p.NOTX(), pldtypes.RandAddress())
	assert.Regexp(t, "pop", err)
}

func mockDBPrivacyGroup(mc *mockComponents, schemaID pldtypes.Bytes32, stateID pldtypes.HexBytes, contractAddr *pldtypes.EthAddress, members ...string) {
	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnRows(sqlmock.NewRows([]string{
		"domain",
		"id",
		"schema_id",
		`Receipt__contract_address`,
	}).AddRow(
		"domain1",
		stateID,
		schemaID,
		contractAddr,
	))
	memberRows := sqlmock.NewRows([]string{"group", "domain", "idx", "identity"})
	for i, m := range members {
		memberRows.AddRow(stateID, "domain1", i, m)
	}
	mc.db.Mock.ExpectQuery("SELECT.*privacy_group_members").WillReturnRows(memberRows)
}

func TestSendTransactionNoDomain(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{})
	require.Regexp(t, "PD012505", err)

}

func TestSendTransactionNoGroup(t *testing.T) {

	ctx, gm, _, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{
		Domain: "domain1",
	})
	require.Regexp(t, "PD012504", err)

}

func TestSendTransactionGroupNotFound(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnRows(sqlmock.NewRows([]string{}))

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{
		Domain: "domain1",
		Group:  pldtypes.RandBytes(32),
	})
	require.Regexp(t, "PD012502", err)

}

func TestSendTransactionGroupFailQuery(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	mc.db.Mock.ExpectQuery("SELECT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{
		Domain: "domain1",
		Group:  pldtypes.RandBytes(32),
	})
	require.Regexp(t, "pop", err)

}

func TestSendTransactionGroupNotReady(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	mockDBPrivacyGroup(mc, schemaID, groupID, nil)

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{
		Domain: "domain1",
		Group:  groupID,
	})
	require.Regexp(t, "PD012503", err)

}

func TestSendTransactionGroupGetContractFail(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	contractAddr := pldtypes.RandAddress()
	mockDBPrivacyGroup(mc, schemaID, groupID, contractAddr)

	mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *contractAddr).Return(nil, fmt.Errorf("pop"))

	var res any
	err := gm.Call(ctx, gm.p.NOTX(), &res, &pldapi.PrivacyGroupEVMCall{
		Domain: "domain1",
		Group:  groupID,
	})
	require.Regexp(t, "pop", err)

}

func TestSendTransactionSendPreparedTx(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	schemaID := pldtypes.RandBytes32()
	groupID := pldtypes.RandBytes(32)
	contractAddr := pldtypes.RandAddress()
	mockDBPrivacyGroup(mc, schemaID, groupID, contractAddr)

	psc := componentsmocks.NewDomainSmartContract(t)
	mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *contractAddr).Return(psc, nil)

	psc.On("WrapPrivacyGroupEVMTX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&pldapi.TransactionInput{}, nil)
	mc.txManager.On("SendTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := gm.SendTransaction(ctx, gm.p.NOTX(), &pldapi.PrivacyGroupEVMTXInput{
		Domain: "domain1",
		Group:  groupID,
	})
	require.Regexp(t, "pop", err)

}

func newValidPGState() *pldapi.State {
	return &pldapi.State{
		StateBase: pldapi.StateBase{
			ID:         pldtypes.RandBytes(32),
			Schema:     pldtypes.RandBytes32(),
			DomainName: "domain1",
			Data: pldtypes.JSONString(&pldapi.PrivacyGroupGenesisState{
				Name:          "pg1",
				Members:       []string{"me@node1", "you@node2"},
				Properties:    pldapi.NewKeyValueStringProperties(map[string]string{"prop1": "value1"}),
				Configuration: pldapi.NewKeyValueStringProperties(map[string]string{"conf2": "value2"}),
				GenesisSalt:   pldtypes.RandBytes32(),
			}),
		},
	}
}

func TestStoreReceivedGroupOk(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := newValidPGState()

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectExec("INSERT.*privacy_groups").WillReturnResult(driver.ResultNoRows)
	mc.db.Mock.ExpectExec("INSERT.*privacy_group_members").WillReturnResult(driver.ResultNoRows)
	mc.db.Mock.ExpectCommit()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.NoError(t, validationErr)
		return err
	})
	require.NoError(t, err)

}

func TestStoreReceivedGroupInvalidState(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := &pldapi.State{}

	mc.db.Mock.ExpectBegin()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.NoError(t, validationErr)
		return err
	})
	require.Regexp(t, "PD012523", err)

}
func TestStoreReceivedGroupFailInsert(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := newValidPGState()

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectExec("INSERT.*privacy_groups").WillReturnError(fmt.Errorf("pop"))

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.NoError(t, validationErr)
		return err
	})
	require.Regexp(t, "pop", err)

}

func TestStoreReceivedGroupFailUnknownDomain(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners,
		func(mc *mockComponents, conf *pldconf.GroupManagerConfig) {
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain2").Return(nil, fmt.Errorf("domain not found"))
		})
	defer done()

	txID := uuid.New()
	state := newValidPGState()

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectCommit()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain2", txID, state)
		require.Regexp(t, "domain not found", validationErr)
		return err
	})
	require.NoError(t, err)

}

func TestStoreReceivedGroupFailValidationNoMembers(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := &pldapi.State{
		StateBase: pldapi.StateBase{
			ID:         pldtypes.RandBytes(32),
			Schema:     pldtypes.RandBytes32(),
			DomainName: "domain1",
			Data: pldtypes.JSONString(&pldapi.PrivacyGroupGenesisState{
				Name:        "pg1",
				Members:     []string{ /* empty */ },
				GenesisSalt: pldtypes.RandBytes32(),
			}),
		},
	}

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectCommit()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.Regexp(t, "PD012501", validationErr)
		return err
	})
	require.NoError(t, err)

}

func TestStoreReceivedGroupFailValidationNoSalt(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := &pldapi.State{
		StateBase: pldapi.StateBase{
			ID:         pldtypes.RandBytes(32),
			Schema:     pldtypes.RandBytes32(),
			DomainName: "domain1",
			Data: pldtypes.JSONString(&pldapi.PrivacyGroupGenesisState{
				Name:    "pg1",
				Members: []string{"me@node1"},
			}),
		},
	}

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectCommit()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.Regexp(t, "PD012522", validationErr)
		return err
	})
	require.NoError(t, err)

}

func TestStoreReceivedGroupFailValidationBadName(t *testing.T) {

	ctx, gm, mc, done := newTestGroupManager(t, false, &pldconf.GroupManagerConfig{}, mockEmptyMessageListeners)
	defer done()

	txID := uuid.New()
	state := &pldapi.State{
		StateBase: pldapi.StateBase{
			ID:         pldtypes.RandBytes(32),
			Schema:     pldtypes.RandBytes32(),
			DomainName: "domain1",
			Data: pldtypes.JSONString(&pldapi.PrivacyGroupGenesisState{
				Name:        "      ",
				Members:     []string{"me@node1"},
				GenesisSalt: pldtypes.RandBytes32(),
			}),
		},
	}

	mc.db.Mock.ExpectBegin()
	mc.db.Mock.ExpectCommit()

	err := gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		validationErr, err := gm.StoreReceivedGroup(ctx, dbTX, "domain1", txID, state)
		require.Regexp(t, "PD020005", validationErr)
		return err
	})
	require.NoError(t, err)

}
