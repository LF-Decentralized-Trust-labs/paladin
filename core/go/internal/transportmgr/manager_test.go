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

package transportmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	c                *componentsmocks.AllComponents
	db               *mockpersistence.SQLMockProvider
	p                persistence.Persistence
	registryManager  *componentsmocks.RegistryManager
	stateManager     *componentsmocks.StateManager
	domainManager    *componentsmocks.DomainManager
	keyManager       *componentsmocks.KeyManager
	txManager        *componentsmocks.TXManager
	privateTxManager *componentsmocks.PrivateTxManager
	identityResolver *componentsmocks.IdentityResolver
	groupManager     *componentsmocks.GroupManager
}

func newMockComponents(t *testing.T, realDB bool) *mockComponents {
	mc := &mockComponents{c: componentsmocks.NewAllComponents(t)}
	mc.registryManager = componentsmocks.NewRegistryManager(t)
	mc.stateManager = componentsmocks.NewStateManager(t)
	mc.domainManager = componentsmocks.NewDomainManager(t)
	mc.keyManager = componentsmocks.NewKeyManager(t)
	mc.txManager = componentsmocks.NewTXManager(t)
	mc.privateTxManager = componentsmocks.NewPrivateTxManager(t)
	mc.identityResolver = componentsmocks.NewIdentityResolver(t)
	mc.groupManager = componentsmocks.NewGroupManager(t)
	if realDB {
		p, cleanup, err := persistence.NewUnitTestPersistence(context.Background(), "transportmgr")
		require.NoError(t, err)
		t.Cleanup(cleanup)
		mc.p = p
	} else {
		mdb, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		mc.db = mdb
		mc.p = mdb.P
	}
	mc.c.On("Persistence").Return(mc.p).Maybe()
	mc.c.On("RegistryManager").Return(mc.registryManager).Maybe()
	mc.c.On("StateManager").Return(mc.stateManager).Maybe()
	mc.c.On("DomainManager").Return(mc.domainManager).Maybe()
	mc.c.On("KeyManager").Return(mc.keyManager).Maybe()
	mc.c.On("TxManager").Return(mc.txManager).Maybe()
	mc.c.On("PrivateTxManager").Return(mc.privateTxManager).Maybe()
	mc.c.On("IdentityResolver").Return(mc.identityResolver).Maybe()
	mc.c.On("GroupManager").Return(mc.groupManager).Maybe()
	return mc
}

func newTestTransportManager(t *testing.T, realDB bool, conf *pldconf.TransportManagerConfig, extraSetup ...func(mc *mockComponents, conf *pldconf.TransportManagerConfig)) (context.Context, *transportManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.TraceLevel)

	mc := newMockComponents(t, realDB)
	for _, fn := range extraSetup {
		fn(mc, conf)
	}

	tm := NewTransportManager(ctx, conf)

	ir, err := tm.PreInit(mc.c)
	require.NoError(t, err)
	assert.NotNil(t, ir)

	err = tm.PostInit(mc.c)
	require.NoError(t, err)

	err = tm.Start()
	require.NoError(t, err)

	assert.Equal(t, conf.NodeName, tm.LocalNodeName())

	return ctx, tm.(*transportManager), mc, func() {
		if !t.Failed() {
			logrus.SetLevel(oldLevel)
			cancelCtx()
			tm.Stop()
		}
	}
}

func TestMissingName(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{})
	_, err := tm.PreInit(newMockComponents(t, false).c)
	assert.Regexp(t, "PD012002", err)
}

func TestConfiguredTransports(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, false, &pldconf.TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*pldconf.TransportConfig{
			"test1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, dm.ConfiguredTransports())
}

func TestTransportRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, false, &pldconf.TransportManagerConfig{
		NodeName:   "node1",
		Transports: map[string]*pldconf.TransportConfig{},
	})
	defer done()

	_, err := dm.TransportRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012001", err)
}

func TestConfigureTransportFail(t *testing.T) {
	_, tm, _, done := newTestTransportManager(t, false, &pldconf.TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*pldconf.TransportConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	})
	defer done()

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.TransportAPIFunctions{
		ConfigureTransport: func(ctx context.Context, ctr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}

	registerTestTransport(t, tm, tp)
	assert.Regexp(t, "pop", *tp.t.initError.Load())
}

func TestGetLocalTransportDetailsNotFound(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{}).(*transportManager)

	_, err := tm.getLocalTransportDetails(context.Background(), "nope")
	assert.Regexp(t, "PD012001", err)
}

func TestGetLocalTransportDetailsNotFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t, false)
	defer done()

	tp.Functions.GetLocalDetails = func(ctx context.Context, gldr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getLocalTransportDetails(ctx, tp.t.name)
	assert.Regexp(t, "pop", err)
}

func TestSendReliableBadMsg(t *testing.T) {
	ctx, tm, _, done := newTestTransport(t, false)
	defer done()

	err := tm.SendReliable(ctx, tm.persistence.NOTX(), &pldapi.ReliableMessage{
		MessageType: pldapi.RMTReceipt.Enum(),
	})
	assert.Regexp(t, "PD012015", err)
}
