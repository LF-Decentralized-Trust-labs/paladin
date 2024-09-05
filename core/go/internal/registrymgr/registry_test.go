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
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

type testPlugin struct {
	plugintk.RegistryAPIBase
	initialized  atomic.Bool
	r            *registry
	sendMessages chan *prototk.SendMessageRequest
}

func (tp *testPlugin) Initialized() {
	tp.initialized.Store(true)
}

func newTestPlugin(registryFuncs *plugintk.RegistryAPIFunctions) *testPlugin {
	return &testPlugin{
		RegistryAPIBase: plugintk.RegistryAPIBase{
			Functions: registryFuncs,
		},
		sendMessages: make(chan *prototk.SendMessageRequest, 1),
	}
}

func newTestRegistry(t *testing.T, extraSetup ...func(mc *componentmocks.AllComponents)) (context.Context, *registryManager, *testPlugin, func()) {
	ctx, tm, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	}, extraSetup...)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.RegistryAPIFunctions{
		ConfigureRegistry: func(ctx context.Context, ctr *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
			assert.Equal(t, "test1", ctr.Name)
			assert.JSONEq(t, `{"some":"conf"}`, ctr.ConfigJson)
			return &prototk.ConfigureRegistryResponse{}, nil
		},
	}

	registerTestRegistry(t, tm, tp)
	return ctx, tm, tp, done
}

func registerTestRegistry(t *testing.T, rm *registryManager, tp *testPlugin) {
	registryID := uuid.New()
	_, err := rm.RegistryRegistered("test1", registryID, tp)
	assert.NoError(t, err)

	ra := rm.registriesByName["test1"]
	assert.NotNil(t, ra)
	tp.r = ra
	tp.r.initRetry.UTSetMaxAttempts(1)
	<-tp.r.initDone
}

func TestDoubleRegisterReplaces(t *testing.T) {

	_, rm, tp0, done := newTestRegistry(t)
	defer done()
	assert.Nil(t, tp0.r.initError.Load())
	assert.True(t, tp0.initialized.Load())

	// Register again
	tp1 := newTestPlugin(nil)
	tp1.Functions = tp0.Functions
	registerTestRegistry(t, rm, tp1)
	assert.Nil(t, tp1.r.initError.Load())
	assert.True(t, tp1.initialized.Load())

	// Check we get the second from all the maps
	byName := rm.registriesByName[tp1.r.name]
	assert.Same(t, tp1.r, byName)
	byUUID := rm.registriesByID[tp1.r.id]
	assert.Same(t, tp1.r, byUUID)

}

func TestRecordAndResolveInformation(t *testing.T) {
	ctx, rm, tp, done := newTestRegistry(t)
	defer done()

	_, err := rm.GetNodeTransports(ctx, "node1")
	assert.Regexp(t, "PD012100", err)

	// Upsert bad entry
	_, err = tp.r.UpsertTransportDetails(ctx, &prototk.UpsertTransportDetails{})
	assert.Regexp(t, "PD012101", err)

	entry1 := &prototk.UpsertTransportDetails{
		Node:             "node1",
		Transport:        "grpc",
		TransportDetails: "things and stuff",
	}

	// Upsert first entry
	res, err := tp.r.UpsertTransportDetails(ctx, entry1)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Check we get it
	transports, err := rm.GetNodeTransports(ctx, "node1")
	assert.NoError(t, err)
	assert.Len(t, transports, 1)
	assert.Equal(t, components.RegistryNodeTransportEntry{
		Node:             "node1",
		Transport:        "grpc",
		TransportDetails: "things and stuff",
	}, *transports[0])

	// Upsert second entry
	entry2 := &prototk.UpsertTransportDetails{
		Node:             "node1",
		Transport:        "websockets",
		TransportDetails: "more things and stuff",
	}

	// Upsert second entry
	res, err = tp.r.UpsertTransportDetails(ctx, entry2)
	assert.NoError(t, err)
	transports, err = rm.GetNodeTransports(ctx, "node1")
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Len(t, transports, 2)

	// Upsert first entry again
	res, err = tp.r.UpsertTransportDetails(ctx, entry1)
	assert.NoError(t, err)
	transports, err = rm.GetNodeTransports(ctx, "node1")
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Len(t, transports, 2)

}
