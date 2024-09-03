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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type mockComponents struct {
}

func newTestRegistryManager(t *testing.T, conf *RegistryManagerConfig, extraSetup ...func(mc *mockComponents)) (context.Context, *registryManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	mc := &mockComponents{}

	for _, fn := range extraSetup {
		fn(mc)
	}

	tm := NewRegistryManager(ctx, conf)

	err := tm.Start()
	assert.NoError(t, err)

	return ctx, tm.(*registryManager), mc, func() {
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

func TestConfiguredRegistries(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{
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
	}, dm.ConfiguredRegistries())
}

func TestRegistryRegisteredNotFound(t *testing.T) {
	_, dm, _, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{},
	})
	defer done()

	_, err := dm.RegistryRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD011600", err)
}

func TestGetRegistryNotFound(t *testing.T) {
	ctx, dm, _, done := newTestRegistryManager(t, &RegistryManagerConfig{
		Registries: map[string]*RegistryConfig{},
	})
	defer done()

	_, err := dm.GetRegistryByName(ctx, "wrong")
	assert.Regexp(t, "PD011600", err)
}
