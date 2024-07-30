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
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/mocks/commsbusmocks"
	pluginPB "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	testUtil "github.com/kaleido-io/paladin/kata/test/util"
)

func TestNewRegistryOK(t *testing.T) {
	//for most of the tests, we do not use this function so lets test it explicitly
	ctx := context.Background()
	commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	config := &Config{}
	registry, err := NewPluginRegistry(ctx, config, commsBus)
	assert.NoError(t, err)
	assert.NotNil(t, registry)
}

func TestNewRegistryFailMissingConfig(t *testing.T) {
	//for most of the tests, we do not use this function so lets test it explicitly
	ctx := context.Background()
	commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	registry, err := NewPluginRegistry(ctx, nil, commsBus)
	assert.Error(t, err)
	assert.Nil(t, registry)
	assert.Contains(t, err.Error(), "PD010503")
}

func TestPluginRegistry_LoadPluginFailErrorCreatingListener(t *testing.T) {
	ctx := context.Background()
	//commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	messageHandlerMock := commsbus.MessageHandler{
		Channel: make(chan commsbus.Message, 1),
	}
	mockCommsBus := commsbusmocks.NewCommsBus(t)
	mockBroker := commsbusmocks.NewBroker(t)
	mockBroker.On("Listen", ctx, mock.Anything).Return(messageHandlerMock, errors.New("listen error"))
	mockCommsBus.On("Broker").Return(mockBroker)

	config := &Config{}
	registry, err := NewPluginRegistry(ctx, config, mockCommsBus)
	assert.NoError(t, err)
	err = registry.Initialize(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listen error")
	assert.Contains(t, err.Error(), "PD011200")
}

func TestPluginRegistry_LoadPluginFailErrorSubscribingToTopic(t *testing.T) {
	ctx := context.Background()
	//commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	messageHandlerMock := commsbus.MessageHandler{
		Channel: make(chan commsbus.Message, 1),
	}
	mockCommsBus := commsbusmocks.NewCommsBus(t)
	mockBroker := commsbusmocks.NewBroker(t)
	mockBroker.On("Listen", ctx, mock.Anything).Return(messageHandlerMock, nil)
	mockBroker.On("SubscribeToTopic", ctx, mock.Anything, mock.Anything).Return(errors.New("subscribe error"))
	mockCommsBus.On("Broker").Return(mockBroker)

	config := &Config{}
	registry, err := NewPluginRegistry(ctx, config, mockCommsBus)
	assert.NoError(t, err)
	err = registry.Initialize(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscribe error")
	assert.Contains(t, err.Error(), "PD011200")
}

func TestPluginRegistry_LoadPluginFailBindingNotSupported(t *testing.T) {
	ctx := context.Background()
	messageHandlerMock := commsbus.MessageHandler{
		Channel: make(chan commsbus.Message, 1),
	}
	mockCommsBus := commsbusmocks.NewCommsBus(t)
	mockBroker := commsbusmocks.NewBroker(t)
	mockBroker.On("Listen", ctx, mock.Anything).Return(messageHandlerMock, nil)
	mockBroker.On("SubscribeToTopic", ctx, mock.Anything, mock.Anything).Return(nil)
	mockCommsBus.On("Broker").Return(mockBroker)

	config := &Config{
		Providers: []ProviderConfig{
			{
				loaderConfig: loaderConfig{
					Binding: "somerandombbinding",
				},
			},
		},
	}
	registry, err := NewPluginRegistry(ctx, config, mockCommsBus)
	assert.NoError(t, err)
	err = registry.(*pluginRegistry).Initialize(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PD011203")
	assert.Contains(t, err.Error(), "somerandombbinding")

}

func TestPluginRegistry_ListPlugins(t *testing.T) {
	ctx := context.Background()

	commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	registry, providerConfigs, _ := newPluginRegistryForTesting(ctx, t, commsBus)

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

	commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	registry, providerConfigs, _ := newPluginRegistryForTesting(ctx, t, commsBus)

	instanceName := "test-instance-1"
	instance, err := registry.CreateInstance(ctx, providerConfigs[0].Name, instanceName)
	require.NoError(t, err)
	assert.NotNil(t, instance)

}

func TestPluginRegistry_NewInstanceEvent(t *testing.T) {
	ctx := context.Background()

	commsBus := testUtil.NewCommsBusForTesting(ctx, t)
	registry, providerConfigs, messageHandler := newPluginRegistryForTesting(ctx, t, commsBus)

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
