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
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	pluginPB "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	testUtil "github.com/kaleido-io/paladin/kata/test/util"
)

func newPluginRegistryForTesting(ctx context.Context, t *testing.T) (PluginRegistry, []ProviderConfig, commsbus.CommsBus, *commsbus.MessageHandler) {

	commsBus := testUtil.NewCommsBusForTesting(ctx, t)

	testDestination := "test-destination-1"

	messageHandler, err := commsBus.Broker().Listen(ctx, testDestination)
	require.NoError(t, err)

	err = commsBus.Broker().SubscribeToTopic(ctx, TOPIC_PROVIDER_READY, testDestination)
	require.NoError(t, err)

	err = commsBus.Broker().SubscribeToTopic(ctx, TOPIC_INSTANCE_READY, testDestination)
	require.NoError(t, err)

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
	}, commsBus)
	require.NoError(t, err)

	var readyEventBody *pluginPB.PluginProviderReadyEvent
	select {
	case readyEvent := <-messageHandler.Channel:
		require.Equal(t, reflect.TypeFor[*pluginPB.PluginProviderReadyEvent](), reflect.TypeOf(readyEvent.Body), "unexepected event type: "+reflect.TypeOf(readyEvent.Body).String())
		readyEventBody = readyEvent.Body.(*pluginPB.PluginProviderReadyEvent)
	case <-time.After(time.Second): // Timeout after 1 second
		require.Fail(t, "Timed out waiting for message")
	}
	require.NotNil(t, readyEventBody)
	return registry, providerConfigs, commsBus, &messageHandler
}

func TestPluginRegistry_LoadPlugin(t *testing.T) {
	ctx := context.Background()
	newPluginRegistryForTesting(ctx, t)

}

func TestPluginRegistry_ListPlugins(t *testing.T) {
	ctx := context.Background()

	registry, providerConfigs, _, _ := newPluginRegistryForTesting(ctx, t)

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

	assert.Regexp(t, "github.com/kaleido-io/paladin/kata/test/plugins/transport/A", plugins[0].GetBuildInfo())

}

func TestPluginRegistry_CreateInstance(t *testing.T) {
	ctx := context.Background()

	registry, providerConfigs, _, _ := newPluginRegistryForTesting(ctx, t)

	instanceName := "test-instance-1"
	instance, err := registry.CreateInstance(ctx, providerConfigs[0].Name, instanceName)
	require.NoError(t, err)
	assert.NotNil(t, instance)

}

func TestPluginRegistry_NewInstanceEvent(t *testing.T) {
	ctx := context.Background()

	registry, providerConfigs, _, messageHandler := newPluginRegistryForTesting(ctx, t)
	instanceName := "test-instance-1"
	_, err := registry.CreateInstance(ctx, providerConfigs[0].Name, instanceName)
	require.NoError(t, err)

	var readyEventBody *pluginPB.PluginInstanceReadyEvent
	select {
	case readyEvent := <-messageHandler.Channel:
		require.Equal(
			t,
			reflect.TypeFor[*pluginPB.PluginInstanceReadyEvent](),
			reflect.TypeOf(readyEvent.Body),
			"unexepected event type: "+reflect.TypeOf(readyEvent.Body).String())
		readyEventBody = readyEvent.Body.(*pluginPB.PluginInstanceReadyEvent)
	case <-time.After(time.Second): // Timeout after 1 second
		require.Fail(t, "Timed out waiting for message")
	}
	require.NotNil(t, readyEventBody)
	assert.Equal(t, instanceName, readyEventBody.InstanceName)

}
