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
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/sirupsen/logrus"

	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	c               *componentmocks.AllComponents
	registryManager *componentmocks.RegistryManager
}

func newMockComponents(t *testing.T) *mockComponents {
	mc := &mockComponents{c: componentmocks.NewAllComponents(t)}
	mc.registryManager = componentmocks.NewRegistryManager(t)
	mc.c.On("RegistryManager").Return(mc.registryManager).Maybe()
	return mc
}

func newTestTransportManager(t *testing.T, conf *pldconf.TransportManagerConfig, extraSetup ...func(mc *mockComponents) components.TransportClient) (context.Context, *transportManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.TraceLevel)

	mc := newMockComponents(t)
	var clients []components.TransportClient
	for _, fn := range extraSetup {
		client := fn(mc)
		if client != nil {
			clients = append(clients, client)
		}
	}

	tm := NewTransportManager(ctx, conf)

	ir, err := tm.PreInit(mc.c)
	require.NoError(t, err)
	assert.NotNil(t, ir)

	// registration happens during init
	for _, c := range clients {
		err := tm.RegisterClient(ctx, c)
		require.NoError(t, err)
	}

	err = tm.PostInit(mc.c)
	require.NoError(t, err)

	err = tm.Start()
	require.NoError(t, err)

	assert.Equal(t, conf.NodeName, tm.LocalNodeName())

	return ctx, tm.(*transportManager), mc, func() {
		logrus.SetLevel(oldLevel)
		cancelCtx()
		tm.Stop()
	}
}

func TestMissingName(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{})
	_, err := tm.PreInit(newMockComponents(t).c)
	assert.Regexp(t, "PD012002", err)
}

func TestConfiguredTransports(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &pldconf.TransportManagerConfig{
		NodeName: "node1",
		Transports: map[string]*pldconf.TransportConfig{
			"test1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(tktypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(tktypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, dm.ConfiguredTransports())
}

func TestTransportRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &pldconf.TransportManagerConfig{
		NodeName:   "node1",
		Transports: map[string]*pldconf.TransportConfig{},
	})
	defer done()

	_, err := dm.TransportRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD012001", err)
}

func TestConfigureTransportFail(t *testing.T) {
	_, tm, _, done := newTestTransportManager(t, &pldconf.TransportManagerConfig{
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

func TestDoubleRegisterClient(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{})

	receivingClient := componentmocks.NewTransportClient(t)
	receivingClient.On("Destination").Return("receivingClient1")

	err := tm.RegisterClient(context.Background(), receivingClient)
	require.NoError(t, err)

	err = tm.RegisterClient(context.Background(), receivingClient)
	assert.Regexp(t, "PD012010", err)
}

func TestDoubleRegisterAfterStart(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{})
	tm.(*transportManager).destinationsFixed = true

	receivingClient := componentmocks.NewTransportClient(t)
	receivingClient.On("Destination").Return("receivingClient1")

	err := tm.RegisterClient(context.Background(), receivingClient)
	assert.Regexp(t, "PD012012", err)
}

func TestGetLocalTransportDetailsNotFound(t *testing.T) {
	tm := NewTransportManager(context.Background(), &pldconf.TransportManagerConfig{}).(*transportManager)

	_, err := tm.getLocalTransportDetails(context.Background(), "nope")
	assert.Regexp(t, "PD012001", err)
}

func TestGetLocalTransportDetailsNotFail(t *testing.T) {
	ctx, tm, tp, done := newTestTransport(t)
	defer done()

	tp.Functions.GetLocalDetails = func(ctx context.Context, gldr *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
		return nil, fmt.Errorf("pop")
	}

	_, err := tm.getLocalTransportDetails(ctx, tp.t.name)
	assert.Regexp(t, "pop", err)
}
