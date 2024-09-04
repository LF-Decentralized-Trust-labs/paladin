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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func newTestRegistryManager(t *testing.T, conf *RegistryManagerConfig, extraSetup ...func(mc *componentmocks.AllComponents)) (context.Context, *registryManager, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := componentmocks.NewAllComponents(t)

	for _, fn := range extraSetup {
		fn(mc)
	}

	tm := NewRegistryManager(ctx, conf)

	initData, err := tm.PreInit(mc)
	assert.NoError(t, err)
	assert.NotNil(t, initData)

	err = tm.PostInit(mc)
	assert.NoError(t, err)

	err = tm.Start()
	assert.NoError(t, err)

	return ctx, tm.(*registryManager), func() {
		cancelCtx()
		// pDone()
		tm.Stop()
	}
}

func TestConfiguredRegistries(t *testing.T) {
	_, dm, done := newTestRegistryManager(t, &RegistryManagerConfig{
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
	_, dm, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{},
	})
	defer done()

	_, err := dm.RegistryRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012002", err)
}

func TestConfigureRegistryFail(t *testing.T) {
	_, tm, done := newTestRegistryManager(t, &RegistryManagerConfig{
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
