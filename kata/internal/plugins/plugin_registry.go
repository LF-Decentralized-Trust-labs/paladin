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
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/commsbus"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins/loader"
	kataPB "github.com/kaleido-io/paladin/kata/pkg/proto"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
)

type Config struct {
	Providers []ProviderConfig `yaml:"providers"`
}

type PluginType string

const (
	TRANSPORT PluginType = "TRANSPORT"
	DOMAIN    PluginType = "DOMAIN"
)

type loaderConfig = loader.Config

type ProviderConfig struct {
	loaderConfig
	Name string     `yaml:"name"`
	Type PluginType `yaml:"type"`
}

type Provider interface {
	GetName() string
	GetDestination() string
	GetType() PluginType
	GetBinding() loader.Binding
	GetBuildInfo() string
	CreateInstance(ctx context.Context, instanceName string) (Instance, error)
}

type Instance interface {
	GetName() string
	GetProvider() Provider
	GetDestination() string
}

type instance struct {
	name        string
	provider    *loadedPlugin
	destination string
	status      PluginInstanceStatus
	registry    *pluginRegistry
}

// GetName implements Instance.
func (i instance) GetName() string {
	return i.name
}

// GetProvider implements Instance.
func (i instance) GetProvider() Provider {
	return i.provider
}

// GetDestination implements Instance.
func (i instance) GetDestination() string {
	return i.destination
}

type PluginRegistry interface {
	Initialize(ctx context.Context) error
	ListPlugins(ctx context.Context) ([]Provider, error)
	CreateInstance(ctx context.Context, providerName string, instanceName string) (Instance, error)
}
type PluginProviderStatus string
type PluginInstanceStatus string

const (
	PluginProviderStatusNotReady PluginProviderStatus = "NotReady"
	PluginProviderStatusReady    PluginProviderStatus = "Ready"
	PluginInstanceStatusNotReady PluginInstanceStatus = "NotReady"
	PluginInstanceStatusReady    PluginInstanceStatus = "Ready"
	TOPIC_PROVIDER_READY         string               = "paladin.kata.plugins.provider.status.ready"
	TOPIC_INSTANCE_READY         string               = "paladin.kata.plugins.instance.status.ready"
)

type loadedPlugin struct {
	buildInfo        string
	name             string
	pluginType       PluginType
	binding          loader.Binding
	status           PluginProviderStatus
	stopMonitoring   func()
	providerListener string
	registry         *pluginRegistry
	instances        []*instance
}

func (lp loadedPlugin) GetName() string {
	return lp.name
}

func (lp loadedPlugin) GetDestination() string {
	return lp.providerListener
}

func (lp loadedPlugin) GetType() PluginType {
	return lp.pluginType
}

func (lp loadedPlugin) GetBinding() loader.Binding {
	return lp.binding
}

func (lp loadedPlugin) GetBuildInfo() string {
	return lp.buildInfo
}

type pluginRegistry struct {
	providerConfigs []ProviderConfig
	loadedPlugins   []*loadedPlugin
	commsBus        commsbus.CommsBus
	golangLoader    loader.ProviderLoader //TODO should be a map of loaders by binding
	initialized     bool
}

// CreateInstance implements PluginRegistry.
func (p *pluginRegistry) CreateInstance(ctx context.Context, providerName string, instanceName string) (Instance, error) {
	if !p.initialized {
		log.L(ctx).Error("CreateInstance caled before registry is initialized")
		return nil, i18n.NewError(ctx, msgs.MsgPluginRegistryInternalError, "CreateInstance called before registry is initialized")
	}
	plugin, err := p.getPluginByName(ctx, providerName)
	if err != nil {
		log.L(ctx).Errorf("Failed to get plugin for provider name %s", providerName)
		return nil, err
	}
	if plugin == nil {
		log.L(ctx).Errorf("No plugin found for provider name %s", providerName)
		return nil, i18n.NewError(ctx, msgs.MsgPluginNotFound, providerName)
	}
	return plugin.CreateInstance(ctx, instanceName)
}

func (p *loadedPlugin) CreateInstance(ctx context.Context, instanceName string) (Instance, error) {
	log.L(ctx).Info("CreateInstance")
	instanceUUID := uuid.New().String()

	createInstanceMessage := &pb.CreateInstance{
		MessageDestination: instanceUUID,
		Name:               instanceName,
	}

	log.L(ctx).Infof("Sending create instance message %s", instanceUUID)
	messageID := uuid.New().String()
	busMessage := commsbus.Message{
		ID:          messageID,
		Destination: p.providerListener,
		Body:        createInstanceMessage,
	}
	newInstance := instance{
		name:        instanceName,
		provider:    p,
		destination: instanceUUID,
		status:      PluginInstanceStatusNotReady,
		registry:    p.registry,
	}

	// add to the list before sending the message because we have another thread listening for new listeners and updating the status
	p.instances = append(p.instances, &newInstance)

	err := p.registry.commsBus.Broker().SendMessage(ctx, busMessage)
	if err != nil {
		log.L(ctx).Errorf("Failed to send create instance message %v", err)
		return nil, err
	}
	return newInstance, nil
}

// ListPlugins implements PluginRegistry.
func (p *pluginRegistry) ListPlugins(ctx context.Context) ([]Provider, error) {
	log.L(ctx).Info("Listing plugins")

	plugins := make([]Provider, len(p.loadedPlugins))
	for i, loadedPlugin := range p.loadedPlugins {
		plugins[i] = loadedPlugin

	}
	return plugins, nil
}

func (p *pluginRegistry) getPluginByName(ctx context.Context, name string) (*loadedPlugin, error) {
	log.L(ctx).Debugf("Looking for plugin with name%s", name)

	//TODO this is a linear search, we should probably use a map
	for _, plugin := range p.loadedPlugins {
		if plugin.GetName() == name {
			return plugin, nil
		}
	}
	return nil, nil
}

func (p *pluginRegistry) getPluginByDestination(ctx context.Context, destination string) *loadedPlugin {
	log.L(ctx).Debugf("Looking for plugin with destination %s", destination)
	//TODO this is a linear search, we should probably use a map
	for _, plugin := range p.loadedPlugins {
		if plugin.GetDestination() == destination {
			return plugin
		}
	}
	return nil
}

func (p *pluginRegistry) getPluginInstanceByDestination(ctx context.Context, destination string) *instance {
	log.L(ctx).Debugf("Looking for plugin instance with destination %s", destination)
	//TODO this is a linear search, we should probably use a map
	for _, plugin := range p.loadedPlugins {
		for _, inst := range plugin.instances {
			if inst.GetDestination() == destination {
				return inst
			}
		}
	}
	return nil
}

func (p *pluginRegistry) subscribeToNewListenerEvents(ctx context.Context) error {

	// Subscribe to the topic that the comms bus will publish to when new listeners start
	// create a new channel for these events so that they don't get confused with messages from the plugins
	newListenersDestination := uuid.New().String()
	newListenersListener, err := p.commsBus.Broker().Listen(ctx, newListenersDestination)
	if err != nil {
		log.L(ctx).Errorf("Failed to create new listener listener %s, %v", newListenersDestination, err)
		return i18n.WrapError(ctx, err, msgs.MsgPluginRegistryInternalError, "Failed to create new listener")
	}
	err = p.commsBus.Broker().SubscribeToTopic(ctx, commsbus.TOPIC_NEW_LISTENER, newListenersDestination)
	if err != nil {
		log.L(ctx).Errorf("Failed to subscribe to new listener topic %v", err)
		return i18n.WrapError(ctx, err, msgs.MsgPluginRegistryInternalError, "Failed to subscribe to new listener topic")
	}
	go func() {
		for newListenerEvent := range newListenersListener.Channel {
			log.L(ctx).Infof("Received new listener event %v", newListenerEvent)
			newListenerEventBody, ok := newListenerEvent.Body.(*kataPB.NewListenerEvent)
			if !ok {
				//this should never happen because we created this channel specifically for these events
				// but if it does, there is nothing we can do other than log it and move on
				log.L(ctx).Errorf("Failed to cast new listener event body %v", newListenerEvent)
				continue
			}

			log.L(ctx).Infof("New listener started for destination %s", newListenerEventBody.Destination)
			//if the listener corresponds to one of the plugin providers that we have loaded, we should update the status
			plugin := p.getPluginByDestination(ctx, newListenerEventBody.Destination)

			if plugin != nil {
				log.L(ctx).Infof("Plugin %s is now ready", plugin.GetName())

				//TODO should really move the following inc. the event publication into a SetStatus method on the plugin interface
				plugin.status = PluginProviderStatusReady
				//publish an event to notify that the plugin is ready
				eventId := uuid.New().String()
				eventPayload := pb.PluginProviderReadyEvent{
					ProviderName: plugin.GetName(),
				}
				err = p.commsBus.Broker().PublishEvent(ctx, commsbus.Event{
					Topic: TOPIC_PROVIDER_READY,
					Body:  &eventPayload,
					ID:    eventId,
				})
				if err != nil {
					log.L(ctx).Error("Error publishing event", err)
					//not much more we can do here until we have retries and dead letter queues
				}
				continue
			}

			log.L(ctx).Debugf("No plugin found for destination %s checking if it matches an instance", newListenerEventBody.Destination)
			//if the listener corresponds to one of the plugin instances that we have created, we should update the status
			instance := p.getPluginInstanceByDestination(ctx, newListenerEventBody.Destination)

			if instance != nil {
				log.L(ctx).Infof("Instance %s is now ready", instance.GetName())

				//TODO should really move the following inc. the event publication into a SetStatus method on the plugin interface
				instance.status = PluginInstanceStatusReady
				//publish an event to notify that the plugin instance is ready
				eventId := uuid.New().String()
				eventPayload := pb.PluginInstanceReadyEvent{
					InstanceName: instance.GetName(),
					ProviderName: instance.GetProvider().GetName(),
				}
				err = p.commsBus.Broker().PublishEvent(ctx, commsbus.Event{
					Topic: TOPIC_INSTANCE_READY,
					Body:  &eventPayload,
					ID:    eventId,
				})
				if err != nil {
					log.L(ctx).Error("Error publishing event", err)
					//not much more we can do here until we have retries and dead letter queues
				}
				continue
			}
			log.L(ctx).Debugf("No plugin instance found for destination %s.  Ignoring.", newListenerEventBody.Destination)

		}
		log.L(ctx).Info("Stopped newListenersListener")
	}()
	return nil
}

// ListPlugins implements PluginRegistry.
func (p *pluginRegistry) loadAllPlugins(ctx context.Context) error {
	log.L(ctx).Info("Loading all plugins")
	for _, providerConfig := range p.providerConfigs {
		log.L(ctx).Infof("Loading plugin %s from %s", providerConfig.Name, providerConfig.Path)
		switch providerConfig.Binding {
		case loader.GO_SHARED_LIBRARY:
			providerBinding, err := p.golangLoader.Load(ctx, providerConfig.loaderConfig)
			if err != nil {
				log.L(ctx).Errorf("Failed to load plugin %s", providerConfig.Name)
				return err
			}

			buildInfo, err := providerBinding.BuildInfo(ctx)
			if err != nil {
				log.L(ctx).Errorf("Failed to get build info for plugin %s", providerConfig.Name)
				return err
			}
			// add to the list before loading it because we have another thread listening for new listeners and updating the status
			newlyLoadedPlugin := loadedPlugin{
				buildInfo:        buildInfo,
				name:             providerConfig.Name,
				pluginType:       providerConfig.Type,
				binding:          providerConfig.Binding,
				status:           PluginProviderStatusNotReady,
				providerListener: uuid.New().String(),
				registry:         p,
			}
			p.loadedPlugins = append(p.loadedPlugins, &newlyLoadedPlugin)

			err = providerBinding.InitializeTransportProvider(
				ctx,
				p.commsBus.GRPCServer().GetSocketAddress(),
				newlyLoadedPlugin.providerListener)
			if err != nil {
				log.L(ctx).Errorf("Failed to initialize transport provider for plugin %s", providerConfig.Name)
				return err
			}

			stopMonitoring, err := p.monitorPlugin(ctx, providerConfig.Name)
			if err != nil {
				log.L(ctx).Errorf("Failed to start monitoring plugin %s", providerConfig.Name)
			}
			newlyLoadedPlugin.stopMonitoring = stopMonitoring

		//case loader.JAVA:
		//TODO: this will be a case of sending a message to Portara asking it to load the jar
		//log.L(ctx).Errorf("Java plugins not implemented yet")
		//case loader.C_SHARED_LIBRARY:
		//TODO: this will be similar to GO_SHARED_LIBRARY but we will use dlopen from from the "C" golang package and
		// we need to be super careful about memory management so that we can pass strings across the
		//log.L(ctx).Errorf("C shared library plugins not implemented yet")
		default:
			log.L(ctx).Errorf("Unsupported plugin binding %s", providerConfig.Binding)
			return i18n.NewError(ctx, msgs.MsgPluginBindingNotSupported, providerConfig.Binding)
		}
	}
	return nil
}

func (p *pluginRegistry) monitorPlugin(ctx context.Context, providerName string) (func(), error) {
	log.L(ctx).Infof("Starting monitoring for plugin %s", providerName)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	stopMonitoring := make(chan bool)
	go func() {

		for {
			select {
			case <-ticker.C:
				log.L(ctx).Info("Checking plugin status")
			case <-stopMonitoring:
				log.L(ctx).Info("Stopping monitoring")
				return
			}
		}
	}()

	return func() {
		stopMonitoring <- true
	}, nil

}

func NewPluginRegistry(ctx context.Context, conf *Config, commsBus commsbus.CommsBus) (PluginRegistry, error) {

	//minimal creation code happens here.  Most of the real initialization logic happens in `Initialize`
	//keeping this minimal allows tests to inject dependencies (e.g. mock versions of the loaders) by
	//using a ...ForTesting version of this function
	if conf == nil {
		log.L(ctx).Error("Missing plugin registry config")
		return nil, i18n.NewError(ctx, msgs.MsgConfigFileMissingMandatoryValue, "plugins")
	}
	p := &pluginRegistry{
		providerConfigs: conf.Providers,
		commsBus:        commsBus,
		golangLoader:    loader.NewPluginLoader(ctx, "GO_SHARED_LIBRARY"),
	}
	return p, nil

}

func (p *pluginRegistry) Initialize(ctx context.Context) error {

	//before loading plugins, lets make sure we will know when they start listenting for messages
	err := p.subscribeToNewListenerEvents(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to subscribe to new listener events %v", err)
		return err
	}

	err = p.loadAllPlugins(ctx)
	if err != nil {
		log.L(ctx).Errorf("Failed to load all plugins %v", err)
		return err
	}
	p.initialized = true
	return nil
}
