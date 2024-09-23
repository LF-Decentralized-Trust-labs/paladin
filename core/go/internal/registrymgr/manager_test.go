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
	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/core/pkg/persistence/mockpersistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	db sqlmock.Sqlmock
}

func newTestRegistryManager(t *testing.T, realDB bool, conf *RegistryManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *registryManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := &mockComponents{}
	componentMocks := componentmocks.NewAllComponents(t)

	var p persistence.Persistence
	var err error
	var pDone func()
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx)
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
	componentMocks.On("Persistence").Return(p)

	for _, fn := range extraSetup {
		fn(mc)
	}

	rm := NewRegistryManager(ctx, conf)

	initData, err := rm.PreInit(componentMocks)
	require.NoError(t, err)
	assert.NotNil(t, initData)

	err = rm.PostInit(componentMocks)
	require.NoError(t, err)

	err = rm.Start()
	require.NoError(t, err)

	return ctx, rm.(*registryManager), mc, func() {
		cancelCtx()
		pDone()
		rm.Stop()
	}
}

func TestConfiguredRegistries(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, false, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{
			"test1": {
				Plugin: components.PluginConfig{
					Type:    components.LibraryTypeCShared.Enum(),
					Library: "some/where",
				},
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*components.PluginConfig{
		"test1": {
			Type:    components.LibraryTypeCShared.Enum(),
			Library: "some/where",
		},
	}, dm.ConfiguredRegistries())
}

func TestRegistryRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, false, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{},
	})
	defer done()

	_, err := dm.RegistryRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012102", err)
}

func TestConfigureRegistryFail(t *testing.T) {
	_, tm, _, done := newTestRegistryManager(t, false, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{
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
