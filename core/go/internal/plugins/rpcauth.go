/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

// The gRPC stream connected to by RPC auth plugins
func (pm *pluginManager) ConnectRPCAuthPlugin(stream prototk.PluginController_ConnectRPCAuthPluginServer) error {
	handler := newPluginHandler(pm, prototk.PluginInfo_RPC_AUTH, pm.authPlugins, stream,
		&plugintk.RPCAuthMessageWrapper{},
		func(pluginInst *plugin[prototk.RPCAuthMessage], toPlugin managerToPlugin[prototk.RPCAuthMessage]) (pluginToManager pluginToManager[prototk.RPCAuthMessage], err error) {
			br := &RPCAuthBridge{
				plugin:     pluginInst,
				pluginType: pluginInst.def.Plugin.PluginType.String(),
				pluginName: pluginInst.name,
				pluginId:   pluginInst.id.String(),
				toPlugin:   toPlugin,
				manager:    nil, // Auth is unidirectional, no callbacks needed
			}

			// Register bridge with manager using standard pattern
			if pm.rpcAuthManager != nil {
				br.manager, err = pm.rpcAuthManager.RPCAuthorizerRegistered(pluginInst.name, pluginInst.id, br)
				if err != nil {
					return nil, err
				}
			}

			// Notify that plugin is initialized - config will be sent after handler is ready
			br.Initialized()

			return br, nil
		})

	return handler.serve()
}

type RPCAuthBridge struct {
	plugin     *plugin[prototk.RPCAuthMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.RPCAuthMessage]
	manager    plugintk.RPCAuthCallbacks // Empty interface for pattern consistency
}

// Initialized is called when the RPC authorizer plugin is fully initialized.
// WaitForStart will block until this is done.
func (ab *RPCAuthBridge) Initialized() {
	ab.plugin.notifyInitialized()
}

// ConfigureRPCAuthorizer implements plugintk.RPCAuthAPI
func (ab *RPCAuthBridge) ConfigureRPCAuthorizer(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
	var res *prototk.ConfigureRPCAuthorizerResponse
	err := ab.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) {
			dm.Message().RequestToAuthplugin = &prototk.RPCAuthMessage_ConfigureRpcAuthorizer{ConfigureRpcAuthorizer: req}
		},
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) bool {
			if r, ok := dm.Message().ResponseFromAuthplugin.(*prototk.RPCAuthMessage_ConfigureRpcAuthorizerRes); ok {
				res = r.ConfigureRpcAuthorizerRes
			}
			return res != nil
		},
	)
	return res, err
}

// Authenticate implements plugintk.RPCAuthAPI
func (ab *RPCAuthBridge) Authenticate(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
	var res *prototk.AuthenticateResponse
	err := ab.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) {
			dm.Message().RequestToAuthplugin = &prototk.RPCAuthMessage_Authenticate{Authenticate: req}
		},
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) bool {
			if r, ok := dm.Message().ResponseFromAuthplugin.(*prototk.RPCAuthMessage_AuthenticateRes); ok {
				res = r.AuthenticateRes
			}
			return res != nil
		},
	)
	return res, err
}

// Authorize implements plugintk.RPCAuthAPI
func (ab *RPCAuthBridge) Authorize(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
	var res *prototk.AuthorizeResponse
	err := ab.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) {
			dm.Message().RequestToAuthplugin = &prototk.RPCAuthMessage_Authorize{Authorize: req}
		},
		func(dm plugintk.PluginMessage[prototk.RPCAuthMessage]) bool {
			if r, ok := dm.Message().ResponseFromAuthplugin.(*prototk.RPCAuthMessage_AuthorizeRes); ok {
				res = r.AuthorizeRes
			}
			return res != nil
		},
	)
	return res, err
}

func (ab *RPCAuthBridge) RequestReply(ctx context.Context, req plugintk.PluginMessage[prototk.RPCAuthMessage]) (resFn func(plugintk.PluginMessage[prototk.RPCAuthMessage]), err error) {
	// Auth plugin doesn't send requests to the manager
	return func(plugintk.PluginMessage[prototk.RPCAuthMessage]) {}, nil
}

func (ab *RPCAuthBridge) ConfigureRPCAuthorizerByName(ctx context.Context, configJson string) error {
	req := &prototk.ConfigureRPCAuthorizerRequest{
		ConfigJson: configJson,
	}
	_, err := ab.ConfigureRPCAuthorizer(ctx, req)
	return err
}
