// Copyright © 2024 Kaleido, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package plugins

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/plugins/loader"
	pluginPB "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	aProvider "github.com/kaleido-io/paladin/kata/test/plugins/transport/A/pkg/provider"
	testUtil "github.com/kaleido-io/paladin/kata/test/util"
)

func newPluginRegistryForTesting(ctx context.Context, t *testing.T) (PluginRegistry, []ProviderConfig, commsbus.CommsBus, *commsbus.MessageHandler) {

	// Creates an actual comms bus, a message handler to listen for messages and subscribes to the provider ready and instance ready topics
	// creates a registry, with mock go lang loader to workaround the issue where the go plugin loader failes to load the shared library when run with `-cover`
	// I tried building the lib using -cover but that did not help.

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
			Name: "transportA",
			Type: TRANSPORT,
			loaderConfig: loader.Config{
				Binding: loader.GO_SHARED_LIBRARY,
				//this path is relative to the package under test
				Path: "../../transportA.so",
			},
		},
	}
	registry := &pluginRegistry{
		providerConfigs: providerConfigs,
		commsBus:        commsBus,
		golangLoader:    &goDirectLoader{},
	}

	require.NoError(t, err)

	err = registry.Initialize(ctx)
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

// functions specific to loading plugins that are built as go plugin shared libraries
type goDirectLoader struct {
}

func (g *goDirectBinding) BuildInfo(ctx context.Context) (string, error) {

	return aProvider.BuildInfo(), nil
}

func (g *goDirectBinding) InitializeTransportProvider(ctx context.Context, socketAddress string, providerListenerDestination string) error {
	return aProvider.InitializeTransportProvider(socketAddress, providerListenerDestination)
}

type goDirectBinding struct {
}

func (_ *goDirectLoader) Load(ctx context.Context, providerConfig loader.Config) (loader.ProviderBinding, error) {
	return &goDirectBinding{}, nil

}
