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

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type mockComponents struct {
	c               *componentmocks.AllComponents
	registryManager *componentmocks.RegistryManager
	engine          *componentmocks.Engine
}

func newMockComponents(t *testing.T) *mockComponents {
	mc := &mockComponents{c: componentmocks.NewAllComponents(t)}
	mc.registryManager = componentmocks.NewRegistryManager(t)
	mc.c.On("RegistryManager").Return(mc.registryManager).Maybe()
	mc.engine = componentmocks.NewEngine(t)
	mc.c.On("Engine").Return(mc.engine).Maybe()
	return mc
}

func newTestTransportManager(t *testing.T, conf *TransportManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := newMockComponents(t)
	for _, fn := range extraSetup {
		fn(mc)
	}

	tm := NewTransportManager(ctx, conf)

	ir, err := tm.PreInit(mc.c)
	assert.NoError(t, err)
	assert.NotNil(t, ir)

	err = tm.PostInit(mc.c)
	assert.NoError(t, err)

	err = tm.Start()
	assert.NoError(t, err)

	assert.Equal(t, conf.NodeName, tm.LocalNodeName())

	return ctx, tm.(*transportManager), mc, func() {
		cancelCtx()
		// pDone()
		tm.Stop()
	}
}

func TestMissingName(t *testing.T) {
	tm := NewTransportManager(context.Background(), &TransportManagerConfig{})
	_, err := tm.PreInit(newMockComponents(t).c)
	assert.Regexp(t, "PD012002", err)
}

func TestConfiguredTransports(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*TransportConfig{
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
	}, dm.ConfiguredTransports())
}

func TestTransportRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		NodeName:   "node1",
		Transports: map[string]*TransportConfig{},
	})
	defer done()

	_, err := dm.TransportRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012001", err)
}

func TestConfigureTransportFail(t *testing.T) {
	_, tm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*TransportConfig{
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
