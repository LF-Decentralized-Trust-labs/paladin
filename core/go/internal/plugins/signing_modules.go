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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

// The gRPC stream connected to by signing module plugins
func (pm *pluginManager) ConnectSigningModule(stream prototk.PluginController_ConnectSigningModuleServer) error {
	handler := newPluginHandler(pm, prototk.PluginInfo_SIGNING_MODULE, pm.signingModulePlugins, stream,
		&plugintk.SigningModuleMessageWrapper{},
		func(plugin *plugin[prototk.SigningModuleMessage], toPlugin managerToPlugin[prototk.SigningModuleMessage]) (pluginToManager pluginToManager[prototk.SigningModuleMessage], err error) {
			br := &SigningModuleBridge{
				plugin:     plugin,
				pluginType: plugin.def.Plugin.PluginType.String(),
				pluginName: plugin.name,
				pluginId:   plugin.id.String(),
				toPlugin:   toPlugin,
			}
			br.manager, err = pm.signingModuleManager.SigningModuleRegistered(plugin.name, plugin.id, br)
			if err != nil {
				return nil, err
			}
			return br, nil
		})
	return handler.serve()
}

type SigningModuleBridge struct {
	plugin     *plugin[prototk.SigningModuleMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.SigningModuleMessage]
	manager    plugintk.SigningModuleCallbacks
}

// SigningModuleManager calls this when it is satisfied the SigningModule is fully initialized.
// WaitForStart will block until this is done.
func (br *SigningModuleBridge) Initialized() {
	br.plugin.notifyInitialized()
}

// requests to callbacks in the signing module manager
// for now there are no callbacks supported
func (br *SigningModuleBridge) RequestReply(ctx context.Context, reqMsg plugintk.PluginMessage[prototk.SigningModuleMessage]) (resFn func(plugintk.PluginMessage[prototk.SigningModuleMessage]), err error) {
	switch req := reqMsg.Message().RequestFromSigningModule.(type) {
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPluginBadRequestBody, req)
	}
}

func (br *SigningModuleBridge) ConfigureSigningModule(ctx context.Context, req *prototk.ConfigureSigningModuleRequest) (res *prototk.ConfigureSigningModuleResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) {
			dm.Message().RequestToSigningModule = &prototk.SigningModuleMessage_ConfigureSigningModule{ConfigureSigningModule: req}
		},
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) bool {
			if r, ok := dm.Message().ResponseFromSigningModule.(*prototk.SigningModuleMessage_ConfigureSigningModuleRes); ok {
				res = r.ConfigureSigningModuleRes
			}
			return res != nil
		},
	)
	return
}

func (br *SigningModuleBridge) ResolveKey(ctx context.Context, req *prototk.ResolveKeyRequest) (res *prototk.ResolveKeyResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) {
			dm.Message().RequestToSigningModule = &prototk.SigningModuleMessage_ResolveKey{ResolveKey: req}
		},
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) bool {
			if r, ok := dm.Message().ResponseFromSigningModule.(*prototk.SigningModuleMessage_ResolveKeyRes); ok {
				res = r.ResolveKeyRes
			}
			return res != nil
		},
	)
	return
}

func (br *SigningModuleBridge) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (res *prototk.SignWithKeyResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) {
			dm.Message().RequestToSigningModule = &prototk.SigningModuleMessage_Sign{Sign: req}
		},
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) bool {
			if r, ok := dm.Message().ResponseFromSigningModule.(*prototk.SigningModuleMessage_SignRes); ok {
				res = r.SignRes
			}
			return res != nil
		},
	)
	return
}

func (br *SigningModuleBridge) ListKeys(ctx context.Context, req *prototk.ListKeysRequest) (res *prototk.ListKeysResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) {
			dm.Message().RequestToSigningModule = &prototk.SigningModuleMessage_ListKeys{ListKeys: req}
		},
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) bool {
			if r, ok := dm.Message().ResponseFromSigningModule.(*prototk.SigningModuleMessage_ListKeysRes); ok {
				res = r.ListKeysRes
			}
			return res != nil
		},
	)
	return
}

func (br *SigningModuleBridge) Close(ctx context.Context, req *prototk.CloseRequest) (res *prototk.CloseResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) {
			dm.Message().RequestToSigningModule = &prototk.SigningModuleMessage_Close{Close: req}
		},
		func(dm plugintk.PluginMessage[prototk.SigningModuleMessage]) bool {
			if r, ok := dm.Message().ResponseFromSigningModule.(*prototk.SigningModuleMessage_CloseRes); ok {
				res = r.CloseRes
			}
			return res != nil
		},
	)
	return
}
