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
package plugintk

import (
	"context"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
)

type RegistryAPI interface {
	ConfigureRegistry(context.Context, *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error)
}

type RegistryCallbacks interface {
	UpsertTransportDetails(context.Context, *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error)
}

type RegistryFactory func(callbacks RegistryCallbacks) RegistryAPI

func NewRegistry(df RegistryFactory) PluginBase {
	impl := &registryPlugin{
		factory: df,
	}
	return NewPluginBase(
		prototk.PluginInfo_REGISTRY,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.RegistryMessage, prototk.RegistryMessage], error) {
			return client.ConnectRegistry(ctx)
		},
		impl,
	)
}

type RegistryPluginMessage struct {
	m *prototk.RegistryMessage
}

func (pm *RegistryPluginMessage) Header() *prototk.Header {
	if pm.m.Header == nil {
		pm.m.Header = &prototk.Header{}
	}
	return pm.m.Header
}

func (pm *RegistryPluginMessage) RequestToPlugin() any {
	return pm.m.RequestToRegistry
}

func (pm *RegistryPluginMessage) ResponseFromPlugin() any {
	return pm.m.ResponseFromRegistry
}

func (pm *RegistryPluginMessage) RequestFromPlugin() any {
	return pm.m.RequestFromRegistry
}

func (pm *RegistryPluginMessage) ResponseToPlugin() any {
	return pm.m.ResponseToRegistry
}

func (pm *RegistryPluginMessage) Message() *prototk.RegistryMessage {
	return pm.m
}

func (pm *RegistryPluginMessage) ProtoMessage() pb.Message {
	return pm.m
}

type RegistryMessageWrapper struct{}

type registryPlugin struct {
	RegistryMessageWrapper
	factory RegistryFactory
}

func (tmw *RegistryMessageWrapper) Wrap(m *prototk.RegistryMessage) PluginMessage[prototk.RegistryMessage] {
	return &RegistryPluginMessage{m: m}
}

func (rp *registryPlugin) NewHandler(proxy PluginProxy[prototk.RegistryMessage]) PluginHandler[prototk.RegistryMessage] {
	th := &registryHandler{
		registryPlugin: rp,
		proxy:          proxy,
	}
	th.api = rp.factory(th)
	return th
}

type registryHandler struct {
	*registryPlugin
	api   RegistryAPI
	proxy PluginProxy[prototk.RegistryMessage]
}

func (th *registryHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.RegistryMessage]) (PluginMessage[prototk.RegistryMessage], error) {
	req := iReq.Message()
	res := &prototk.RegistryMessage{}
	var err error
	switch input := req.RequestToRegistry.(type) {
	case *prototk.RegistryMessage_ConfigureRegistry:
		resMsg := &prototk.RegistryMessage_ConfigureRegistryRes{}
		resMsg.ConfigureRegistryRes, err = th.api.ConfigureRegistry(ctx, input.ConfigureRegistry)
		res.ResponseFromRegistry = resMsg
	default:
		err = i18n.NewError(ctx, tkmsgs.MsgPluginUnsupportedRequest, input)
	}
	return th.Wrap(res), err
}

func (dh *registryHandler) UpsertTransportDetails(ctx context.Context, req *prototk.UpsertTransportDetails) (*prototk.UpsertTransportDetailsResponse, error) {
	res, err := dh.proxy.RequestFromPlugin(ctx, dh.Wrap(&prototk.RegistryMessage{
		RequestFromRegistry: &prototk.RegistryMessage_UpsertTransportDetails{
			UpsertTransportDetails: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.RegistryMessage_UpsertTransportDetailsRes) *prototk.UpsertTransportDetailsResponse {
		return msg.UpsertTransportDetailsRes
	})
}

type RegistryAPIFunctions struct {
	ConfigureRegistry func(context.Context, *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error)
}

type RegistryAPIBase struct {
	Functions *RegistryAPIFunctions
}

func (tb *RegistryAPIBase) ConfigureRegistry(ctx context.Context, req *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.ConfigureRegistry)
}