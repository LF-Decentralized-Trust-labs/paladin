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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

// The gRPC stream connected to by Registry plugins
func (pm *pluginManager) ConnectRegistry(stream prototk.PluginController_ConnectRegistryServer) error {
	handler := newPluginHandler(pm, prototk.PluginInfo_REGISTRY, pm.registryPlugins, stream,
		&plugintk.RegistryMessageWrapper{},
		func(plugin *plugin[prototk.RegistryMessage], toPlugin managerToPlugin[prototk.RegistryMessage]) (pluginToManager pluginToManager[prototk.RegistryMessage], err error) {
			br := &RegistryBridge{
				plugin:     plugin,
				pluginType: plugin.def.Plugin.PluginType.String(),
				pluginName: plugin.name,
				pluginId:   plugin.id.String(),
				toPlugin:   toPlugin,
			}
			br.manager, err = pm.registryManager.RegistryRegistered(plugin.name, plugin.id, br)
			if err != nil {
				return nil, err
			}
			return br, nil
		})
	return handler.serve()
}

type RegistryBridge struct {
	plugin     *plugin[prototk.RegistryMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.RegistryMessage]
	manager    plugintk.RegistryCallbacks
}

// RegistryManager calls this when it is satisfied the Registry is fully initialized.
// WaitForStart will block until this is done.
func (br *RegistryBridge) Initialized() {
	br.plugin.notifyInitialized()
}

// requests to callbacks in the Registry manager
func (br *RegistryBridge) RequestReply(ctx context.Context, reqMsg plugintk.PluginMessage[prototk.RegistryMessage]) (resFn func(plugintk.PluginMessage[prototk.RegistryMessage]), err error) {
	switch req := reqMsg.Message().RequestFromRegistry.(type) {
	case *prototk.RegistryMessage_UpsertRegistryRecords:
		return callManagerImpl(ctx, req.UpsertRegistryRecords,
			br.manager.UpsertRegistryRecords,
			func(resMsg *prototk.RegistryMessage, res *prototk.UpsertRegistryRecordsResponse) {
				resMsg.ResponseToRegistry = &prototk.RegistryMessage_UpsertRegistryRecordsRes{
					UpsertRegistryRecordsRes: res,
				}
			},
		)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPluginBadRequestBody, req)
	}
}

func (br *RegistryBridge) ConfigureRegistry(ctx context.Context, req *prototk.ConfigureRegistryRequest) (res *prototk.ConfigureRegistryResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.RegistryMessage]) {
			dm.Message().RequestToRegistry = &prototk.RegistryMessage_ConfigureRegistry{ConfigureRegistry: req}
		},
		func(dm plugintk.PluginMessage[prototk.RegistryMessage]) bool {
			if r, ok := dm.Message().ResponseFromRegistry.(*prototk.RegistryMessage_ConfigureRegistryRes); ok {
				res = r.ConfigureRegistryRes
			}
			return res != nil
		},
	)
	return
}

func (br *RegistryBridge) HandleRegistryEvents(ctx context.Context, req *prototk.HandleRegistryEventsRequest) (res *prototk.HandleRegistryEventsResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.RegistryMessage]) {
			dm.Message().RequestToRegistry = &prototk.RegistryMessage_HandleRegistryEvents{HandleRegistryEvents: req}
		},
		func(dm plugintk.PluginMessage[prototk.RegistryMessage]) bool {
			if r, ok := dm.Message().ResponseFromRegistry.(*prototk.RegistryMessage_HandleRegistryEventsRes); ok {
				res = r.HandleRegistryEventsRes
			}
			return res != nil
		},
	)
	return
}
