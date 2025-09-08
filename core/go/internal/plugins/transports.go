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

// The gRPC stream connected to by Transport plugins
func (pm *pluginManager) ConnectTransport(stream prototk.PluginController_ConnectTransportServer) error {
	handler := newPluginHandler(pm, prototk.PluginInfo_TRANSPORT, pm.transportPlugins, stream,
		&plugintk.TransportMessageWrapper{},
		func(plugin *plugin[prototk.TransportMessage], toPlugin managerToPlugin[prototk.TransportMessage]) (pluginToManager pluginToManager[prototk.TransportMessage], err error) {
			br := &TransportBridge{
				plugin:     plugin,
				pluginType: plugin.def.Plugin.PluginType.String(),
				pluginName: plugin.name,
				pluginId:   plugin.id.String(),
				toPlugin:   toPlugin,
			}
			br.manager, err = pm.transportManager.TransportRegistered(plugin.name, plugin.id, br)
			if err != nil {
				return nil, err
			}
			return br, nil
		})
	return handler.serve()
}

type TransportBridge struct {
	plugin     *plugin[prototk.TransportMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.TransportMessage]
	manager    plugintk.TransportCallbacks
}

// TransportManager calls this when it is satisfied the Transport is fully initialized.
// WaitForStart will block until this is done.
func (br *TransportBridge) Initialized() {
	br.plugin.notifyInitialized()
}

// requests to callbacks in the Transport manager
func (br *TransportBridge) RequestReply(ctx context.Context, reqMsg plugintk.PluginMessage[prototk.TransportMessage]) (resFn func(plugintk.PluginMessage[prototk.TransportMessage]), err error) {
	switch req := reqMsg.Message().RequestFromTransport.(type) {
	case *prototk.TransportMessage_GetTransportDetails:
		return callManagerImpl(ctx, req.GetTransportDetails,
			br.manager.GetTransportDetails,
			func(resMsg *prototk.TransportMessage, res *prototk.GetTransportDetailsResponse) {
				resMsg.ResponseToTransport = &prototk.TransportMessage_GetTransportDetailsRes{
					GetTransportDetailsRes: res,
				}
			},
		)
	case *prototk.TransportMessage_ReceiveMessage:
		return callManagerImpl(ctx, req.ReceiveMessage,
			br.manager.ReceiveMessage,
			func(resMsg *prototk.TransportMessage, res *prototk.ReceiveMessageResponse) {
				resMsg.ResponseToTransport = &prototk.TransportMessage_ReceiveMessageRes{
					ReceiveMessageRes: res,
				}
			},
		)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPluginBadRequestBody, req)
	}
}

func (br *TransportBridge) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (res *prototk.ConfigureTransportResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) {
			dm.Message().RequestToTransport = &prototk.TransportMessage_ConfigureTransport{ConfigureTransport: req}
		},
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) bool {
			if r, ok := dm.Message().ResponseFromTransport.(*prototk.TransportMessage_ConfigureTransportRes); ok {
				res = r.ConfigureTransportRes
			}
			return res != nil
		},
	)
	return
}

func (br *TransportBridge) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (res *prototk.SendMessageResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) {
			dm.Message().RequestToTransport = &prototk.TransportMessage_SendMessage{SendMessage: req}
		},
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) bool {
			if r, ok := dm.Message().ResponseFromTransport.(*prototk.TransportMessage_SendMessageRes); ok {
				res = r.SendMessageRes
			}
			return res != nil
		},
	)
	return
}

func (br *TransportBridge) GetLocalDetails(ctx context.Context, req *prototk.GetLocalDetailsRequest) (res *prototk.GetLocalDetailsResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) {
			dm.Message().RequestToTransport = &prototk.TransportMessage_GetLocalDetails{GetLocalDetails: req}
		},
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) bool {
			if r, ok := dm.Message().ResponseFromTransport.(*prototk.TransportMessage_GetLocalDetailsRes); ok {
				res = r.GetLocalDetailsRes
			}
			return res != nil
		},
	)
	return
}

func (br *TransportBridge) ActivatePeer(ctx context.Context, req *prototk.ActivatePeerRequest) (res *prototk.ActivatePeerResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) {
			dm.Message().RequestToTransport = &prototk.TransportMessage_ActivatePeer{ActivatePeer: req}
		},
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) bool {
			if r, ok := dm.Message().ResponseFromTransport.(*prototk.TransportMessage_ActivatePeerRes); ok {
				res = r.ActivatePeerRes
			}
			return res != nil
		},
	)
	return
}

func (br *TransportBridge) DeactivatePeer(ctx context.Context, req *prototk.DeactivatePeerRequest) (res *prototk.DeactivatePeerResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) {
			dm.Message().RequestToTransport = &prototk.TransportMessage_DeactivatePeer{DeactivatePeer: req}
		},
		func(dm plugintk.PluginMessage[prototk.TransportMessage]) bool {
			if r, ok := dm.Message().ResponseFromTransport.(*prototk.TransportMessage_DeactivatePeerRes); ok {
				res = r.DeactivatePeerRes
			}
			return res != nil
		},
	)
	return
}
