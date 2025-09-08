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

package registrymgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/blockindexermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	noInit        bool
	db            sqlmock.Sqlmock
	allComponents *componentsmocks.AllComponents
	blockIndexer  *blockindexermocks.BlockIndexer
}

func newTestRegistryManager(t *testing.T, realDB bool, conf *pldconf.RegistryManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *registryManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	mm := metrics.NewMetricsManager(ctx)

	mc := &mockComponents{
		blockIndexer:  blockindexermocks.NewBlockIndexer(t),
		allComponents: componentsmocks.NewAllComponents(t),
	}
	mc.allComponents.On("BlockIndexer").Return(mc.blockIndexer).Maybe()
	mc.allComponents.On("MetricsManager").Return(mm).Maybe()

	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx, "registry")
		require.NoError(t, err)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
	}
	mc.allComponents.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	rm := NewRegistryManager(ctx, conf)

	if !mc.noInit {
		initData, err := rm.PreInit(mc.allComponents)
		require.NoError(t, err)
		assert.NotNil(t, initData)

		err = rm.PostInit(mc.allComponents)
		require.NoError(t, err)

		err = rm.Start()
		require.NoError(t, err)
	}

	return ctx, rm.(*registryManager), mc, func() {
		cancelCtx()
		pDone()
		rm.Stop()
	}
}

func TestConfiguredRegistries(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, false, &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{
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
	}, dm.ConfiguredRegistries())
}

func TestRegistryRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, false, &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{},
	})
	defer done()

	_, err := dm.RegistryRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012101", err)
}

func TestConfigureRegistryFail(t *testing.T) {
	_, tm, _, done := newTestRegistryManager(t, false, &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	})
	defer done()

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.RegistryAPIFunctions{
		ConfigureRegistry: func(ctx context.Context, ctr *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}

	registerTestRegistry(t, tm, tp)
	assert.Regexp(t, "pop", *tp.r.initError.Load())
}

func TestGetRegistryNotFound(t *testing.T) {
	ctx, dm, _, done := newTestRegistryManager(t, false, &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{},
	})
	defer done()

	_, err := dm.GetRegistry(ctx, "unknown")
	assert.Regexp(t, "PD012101", err)
}
