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

package loader

import (
	"context"
	"plugin"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

// functions specific to loading plugins that are built as go plugin shared libraries
type goPluginLoader struct {
}

func (g *goProviderBinding) BuildInfo(ctx context.Context) (string, error) {
	if g.buildInfoFunc == nil {
		return "", i18n.NewError(ctx, msgs.MsgPluginLoadError, g.sharedLibrary)
	}
	buildInfo := g.buildInfoFunc()
	return buildInfo, nil
}

func (g *goProviderBinding) InitializeTransportProvider(ctx context.Context, socketAddress string, providerListenerDestination string) error {

	log.L(ctx).Infof("Initializing transport provider socketAddress=%s, destination=%s", socketAddress, providerListenerDestination)
	if g.initializeTransportProviderFunc == nil {
		return i18n.NewError(ctx, msgs.MsgPluginLoadError, g.name)
	}
	err := g.initializeTransportProviderFunc(socketAddress, providerListenerDestination)
	if err != nil {
		log.L(ctx).Errorf("Failed to initialize transport provider %v", err)
		return i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, g.sharedLibrary)
	}

	return nil
}

type goProviderBinding struct {
	name                            string
	sharedLibrary                   string
	buildInfoFunc                   func() string
	initializeTransportProviderFunc func(string, string) error
}

func (_ *goPluginLoader) Load(ctx context.Context, providerConfig Config) (ProviderBinding, error) {
	// Load shared library
	log.L(ctx).Info("Loading shared library")
	plug, err := plugin.Open(providerConfig.Path)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, providerConfig.Path)
	}

	binding := &goProviderBinding{}

	buildInfoSymbol, err := plug.Lookup("BuildInfo")
	if err != nil {
		log.L(ctx).Errorf("Failed to lookup BuildInfo symbol %v", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, providerConfig.Path)
	}
	buildInfoFunc, ok := buildInfoSymbol.(func() string)
	if !ok {
		log.L(ctx).Infof("BuildInfo symbol is not correct signature")
		return nil, i18n.NewError(ctx, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, providerConfig.Path)
	}
	binding.buildInfoFunc = buildInfoFunc

	initializeSymbol, err := plug.Lookup("InitializeTransportProvider")
	if err != nil {
		log.L(ctx).Errorf("Failed to lookup InitializeTransportProvider symbol %v", err)
		return nil, i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, providerConfig.Path)
	}

	initializeFunc, ok := initializeSymbol.(func(string, string) error)
	if !ok {
		log.L(ctx).Infof("InitializeTransportProvider symbol is not correct signature")
		return nil, i18n.NewError(ctx, msgs.MsgPluginLoadError, GO_SHARED_LIBRARY, providerConfig.Path)
	}
	binding.initializeTransportProviderFunc = initializeFunc

	return binding, nil
}
