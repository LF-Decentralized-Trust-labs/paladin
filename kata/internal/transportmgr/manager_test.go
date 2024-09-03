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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type mockComponents struct {
}

func newTestTransportManager(t *testing.T, conf *TransportManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *transportManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := &mockComponents{}

	for _, fn := range extraSetup {
		fn(mc)
	}

	tm := NewTransportManager(ctx, conf)

	err := tm.Start()
	assert.NoError(t, err)

	return ctx, tm.(*transportManager), mc, func() {
		cancelCtx()
		// pDone()
		tm.Stop()
	}
}

func yamlNode(t *testing.T, s string) (n yaml.Node) {
	err := yaml.Unmarshal([]byte(s), &n)
	assert.NoError(t, err)
	return
}

func TestConfiguredTransports(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		Transports: map[string]*TransportConfig{
			"test1": {
				Plugin: plugins.PluginConfig{
					Type:    plugins.LibraryTypeCShared.Enum(),
					Library: "some/where",
				},
			},
		},
	})
	defer done()

	assert.Equal(t, map[string]*plugins.PluginConfig{
		"test1": {
			Type:    plugins.LibraryTypeCShared.Enum(),
			Library: "some/where",
		},
	}, dm.ConfiguredTransports())
}

func TestTransportRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		Transports: map[string]*TransportConfig{},
	})
	defer done()

	_, err := dm.TransportRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD011600", err)
}

func TestGetTransportNotFound(t *testing.T) {
	ctx, dm, _, done := newTestTransportManager(t, &TransportManagerConfig{
		Transports: map[string]*TransportConfig{},
	})
	defer done()

	_, err := dm.GetTransportByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)
}
