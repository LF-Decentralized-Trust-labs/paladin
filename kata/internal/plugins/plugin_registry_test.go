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
package plugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginRegistry_ListPlugins(t *testing.T) {
	ctx := context.Background()

	// Create a sample plugin registry with provider configs
	providerConfigs := []ProviderConfig{
		{
			Name:    "transportA",
			Type:    TRANSPORT,
			Binding: GO_SHARED_LIBRARY,
			//this path is relative to the package under test
			Path: "../../transportA.so",
		},
	}
	registry, err := NewPluginRegistry(ctx, &Config{
		Providers: providerConfigs,
	})
	require.NoError(t, err)

	// Call the ListPlugins method
	plugins, err := registry.ListPlugins(ctx)

	require.NoError(t, err)

	// Assert that the returned plugins match the provider configs
	assert.Len(t, plugins, len(providerConfigs))
	for i, plugin := range plugins {
		assert.Equal(t, providerConfigs[i].Name, plugin.GetName())
		assert.Equal(t, providerConfigs[i].Type, plugin.GetType())
		assert.Equal(t, providerConfigs[i].Binding, plugin.GetBinding())
	}

	assert.Equal(t, "Hello I am transportAProvider", plugins[0].GetBuildInfo())

}
