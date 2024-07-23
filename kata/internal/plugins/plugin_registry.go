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
	"plugin"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type Config struct {
	Providers []ProviderConfig `yaml:"providers"`
}

type PluginType string

const (
	TRANSPORT PluginType = "TRANSPORT"
	DOMAIN    PluginType = "DOMAIN"
)

type PluginBinding string

const (
	GO_SHARED_LIBRARY PluginBinding = "GO_SHARED_LIBRARY"
	JAVA              PluginBinding = "JAVA"
	C_SHARED_LIBRARY  PluginBinding = "C_SHARED_LIBRARY"
)

type ProviderConfig struct {
	Name    string        `yaml:"name"`
	Type    PluginType    `yaml:"type"`
	Binding PluginBinding `yaml:"binding"`
	Path    string        `yaml:"path"`
}

type Plugin interface {
	GetName() string
	GetType() PluginType
	GetBinding() PluginBinding
	GetBuildInfo() string
}

type PluginRegistry interface {
	ListPlugins(ctx context.Context) ([]Plugin, error)
}

type loadedPlugin struct {
	buildInfo  string
	name       string
	pluginType PluginType
	binding    PluginBinding
}

func (lp loadedPlugin) GetName() string {
	return lp.name
}

func (lp loadedPlugin) GetType() PluginType {
	return lp.pluginType
}

func (lp loadedPlugin) GetBinding() PluginBinding {
	return lp.binding
}

func (lp loadedPlugin) GetBuildInfo() string {
	return lp.buildInfo
}

type pluginRegistry struct {
	providerConfigs []ProviderConfig
	loadedPlugins   []loadedPlugin
}

// ListPlugins implements PluginRegistry.
func (p *pluginRegistry) ListPlugins(ctx context.Context) ([]Plugin, error) {
	log.L(ctx).Info("Listing plugins")

	plugins := make([]Plugin, len(p.loadedPlugins))
	for i, loadedPlugin := range p.loadedPlugins {
		plugins[i] = loadedPlugin

	}
	return plugins, nil
}

// ListPlugins implements PluginRegistry.
func (p *pluginRegistry) loadAllPlugins(ctx context.Context) error {
	log.L(ctx).Info("Loading all plugins")
	for _, providerConfig := range p.providerConfigs {
		log.L(ctx).Infof("Loading plugin %s from %s", providerConfig.Name, providerConfig.Path)
		switch providerConfig.Binding {
		case GO_SHARED_LIBRARY:
			// Load shared library
			log.L(ctx).Info("Loading shared library")
			plug, err := plugin.Open(providerConfig.Path)
			if err != nil {
				return i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, providerConfig.Name, GO_SHARED_LIBRARY, providerConfig.Path)
			}

			buildInfoSymbol, err := plug.Lookup("BuildInfo")
			if err != nil {
				log.L(ctx).Errorf("Failed to lookup BuildInfo symbol %v", err)
				return i18n.WrapError(ctx, err, msgs.MsgPluginLoadError, providerConfig.Name, GO_SHARED_LIBRARY, providerConfig.Path)
			}
			buildInfoFunc, ok := buildInfoSymbol.(func() string)
			if !ok {
				log.L(ctx).Infof("BuildInfo symbol is not of type func")
				return i18n.NewError(ctx, msgs.MsgPluginLoadError, providerConfig.Name, GO_SHARED_LIBRARY, providerConfig.Path)
			}
			buildInfo := buildInfoFunc()
			p.loadedPlugins = append(p.loadedPlugins, loadedPlugin{
				buildInfo:  buildInfo,
				name:       providerConfig.Name,
				pluginType: providerConfig.Type,
				binding:    providerConfig.Binding,
			})

		case JAVA:
			//TODO: this will be a case of sending a message to Potara asking it to load the jar
			log.L(ctx).Errorf("Java plugins not implemented yet")
		case C_SHARED_LIBRARY:
			//TODO: this will be similar to GO_SHARED_LIBRARY but we will use dlopen from from the "C" golang package and
			// we need to be super careful about memory management so that we can pass strings across the
			log.L(ctx).Errorf("C shared library plugins not implemented yet")
		default:
			log.L(ctx).Errorf("Unsupported plugin binding %s", providerConfig.Binding)
		}
	}
	return nil
}

func NewPluginRegistry(ctx context.Context, conf *Config) (PluginRegistry, error) {

	if conf == nil {
		log.L(ctx).Error("Missing plugin registry config")
		return nil, i18n.NewError(ctx, msgs.MsgConfigFileMissingMandatoryValue, "plugins")
	}
	p := &pluginRegistry{
		providerConfigs: conf.Providers,
	}

	//TODO should this be a separate method that gets called after the factory function is called?
	err := p.loadAllPlugins(ctx)
	if err != nil {
		return nil, err
	}
	return p, nil
}
