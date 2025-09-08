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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
)

type TransportAPI interface {
	ConfigureTransport(context.Context, *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error)
	SendMessage(context.Context, *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error)
	GetLocalDetails(context.Context, *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error)
	ActivatePeer(context.Context, *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error)
	DeactivatePeer(context.Context, *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error)
}

type TransportCallbacks interface {
	GetTransportDetails(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	ReceiveMessage(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

type TransportFactory func(callbacks TransportCallbacks) TransportAPI

func NewTransport(df TransportFactory) PluginBase {
	impl := &transportPlugin{
		factory: df,
	}
	return NewPluginBase(
		prototk.PluginInfo_TRANSPORT,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.TransportMessage, prototk.TransportMessage], error) {
			return client.ConnectTransport(ctx)
		},
		impl,
	)
}

type TransportPluginMessage struct {
	m *prototk.TransportMessage
}

func (pm *TransportPluginMessage) Header() *prototk.Header {
	if pm.m.Header == nil {
		pm.m.Header = &prototk.Header{}
	}
	return pm.m.Header
}

func (pm *TransportPluginMessage) RequestToPlugin() any {
	return pm.m.RequestToTransport
}

func (pm *TransportPluginMessage) ResponseFromPlugin() any {
	return pm.m.ResponseFromTransport
}

func (pm *TransportPluginMessage) RequestFromPlugin() any {
	return pm.m.RequestFromTransport
}

func (pm *TransportPluginMessage) ResponseToPlugin() any {
	return pm.m.ResponseToTransport
}

func (pm *TransportPluginMessage) Message() *prototk.TransportMessage {
	return pm.m
}

func (pm *TransportPluginMessage) ProtoMessage() pb.Message {
	return pm.m
}

type TransportMessageWrapper struct{}

type transportPlugin struct {
	TransportMessageWrapper
	factory TransportFactory
}

func (tmw *TransportMessageWrapper) Wrap(m *prototk.TransportMessage) PluginMessage[prototk.TransportMessage] {
	return &TransportPluginMessage{m: m}
}

func (tp *transportPlugin) NewHandler(proxy PluginProxy[prototk.TransportMessage]) PluginHandler[prototk.TransportMessage] {
	th := &transportHandler{
		transportPlugin: tp,
		proxy:           proxy,
	}
	th.api = tp.factory(th)
	return th
}

type transportHandler struct {
	*transportPlugin
	api   TransportAPI
	proxy PluginProxy[prototk.TransportMessage]
}

func (th *transportHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.TransportMessage]) (PluginMessage[prototk.TransportMessage], error) {
	req := iReq.Message()
	res := &prototk.TransportMessage{}
	var err error
	switch input := req.RequestToTransport.(type) {
	case *prototk.TransportMessage_ConfigureTransport:
		resMsg := &prototk.TransportMessage_ConfigureTransportRes{}
		resMsg.ConfigureTransportRes, err = th.api.ConfigureTransport(ctx, input.ConfigureTransport)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_SendMessage:
		resMsg := &prototk.TransportMessage_SendMessageRes{}
		resMsg.SendMessageRes, err = th.api.SendMessage(ctx, input.SendMessage)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_GetLocalDetails:
		resMsg := &prototk.TransportMessage_GetLocalDetailsRes{}
		resMsg.GetLocalDetailsRes, err = th.api.GetLocalDetails(ctx, input.GetLocalDetails)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_ActivatePeer:
		resMsg := &prototk.TransportMessage_ActivatePeerRes{}
		resMsg.ActivatePeerRes, err = th.api.ActivatePeer(ctx, input.ActivatePeer)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_DeactivatePeer:
		resMsg := &prototk.TransportMessage_DeactivatePeerRes{}
		resMsg.DeactivatePeerRes, err = th.api.DeactivatePeer(ctx, input.DeactivatePeer)
		res.ResponseFromTransport = resMsg
	default:
		err = i18n.NewError(ctx, pldmsgs.MsgPluginUnsupportedRequest, input)
	}
	return th.Wrap(res), err
}

func (th *transportHandler) ReceiveMessage(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	res, err := th.proxy.RequestFromPlugin(ctx, th.Wrap(&prototk.TransportMessage{
		RequestFromTransport: &prototk.TransportMessage_ReceiveMessage{
			ReceiveMessage: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.TransportMessage_ReceiveMessageRes) *prototk.ReceiveMessageResponse {
		return msg.ReceiveMessageRes
	})
}

func (th *transportHandler) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
	res, err := th.proxy.RequestFromPlugin(ctx, th.Wrap(&prototk.TransportMessage{
		RequestFromTransport: &prototk.TransportMessage_GetTransportDetails{
			GetTransportDetails: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.TransportMessage_GetTransportDetailsRes) *prototk.GetTransportDetailsResponse {
		return msg.GetTransportDetailsRes
	})
}

type TransportAPIFunctions struct {
	ConfigureTransport func(context.Context, *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error)
	SendMessage        func(context.Context, *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error)
	GetLocalDetails    func(context.Context, *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error)
	ActivatePeer       func(context.Context, *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error)
	DeactivatePeer     func(context.Context, *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error)
}

type TransportAPIBase struct {
	Functions *TransportAPIFunctions
}

func (tb *TransportAPIBase) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.ConfigureTransport)
}

func (tb *TransportAPIBase) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.SendMessage)
}

func (tb *TransportAPIBase) GetLocalDetails(ctx context.Context, req *prototk.GetLocalDetailsRequest) (*prototk.GetLocalDetailsResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.GetLocalDetails)
}

func (tb *TransportAPIBase) ActivatePeer(ctx context.Context, req *prototk.ActivatePeerRequest) (*prototk.ActivatePeerResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.ActivatePeer)
}

func (tb *TransportAPIBase) DeactivatePeer(ctx context.Context, req *prototk.DeactivatePeerRequest) (*prototk.DeactivatePeerResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.DeactivatePeer)
}
