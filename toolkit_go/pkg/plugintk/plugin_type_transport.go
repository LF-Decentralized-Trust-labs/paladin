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

type TransportAPI interface {
	ConfigureTransport(context.Context, *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error)
	InitTransport(context.Context, *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error)
	SendMessage(context.Context, *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error)
}

type TransportCallbacks interface {
	GetTransportDetails(context.Context, *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error)
	Receive(context.Context, *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error)
}

type TransportFactory func(callbacks TransportCallbacks) TransportAPI

func NewTransport(df TransportFactory) PluginBase {
	impl := &TransportPlugin{
		factory: df,
	}
	return NewPluginBase(
		prototk.PluginType_TRANSPORT,
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

type TransportPlugin struct {
	TransportMessageWrapper
	factory TransportFactory
}

func (tmw *TransportMessageWrapper) Wrap(m *prototk.TransportMessage) PluginMessage[prototk.TransportMessage] {
	return &TransportPluginMessage{m: m}
}

func (tmw *TransportPlugin) NewHandler(proxy PluginProxy[prototk.TransportMessage]) PluginHandler[prototk.TransportMessage] {
	th := &TransportHandler{
		TransportPlugin: tmw,
		proxy:           proxy,
	}
	th.api = tmw.factory(th)
	return th
}

type TransportHandler struct {
	*TransportPlugin
	api   TransportAPI
	proxy PluginProxy[prototk.TransportMessage]
}

func (th *TransportHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.TransportMessage]) (PluginMessage[prototk.TransportMessage], error) {
	req := iReq.Message()
	res := &prototk.TransportMessage{}
	var err error
	switch input := req.RequestToTransport.(type) {
	case *prototk.TransportMessage_ConfigureTransport:
		resMsg := &prototk.TransportMessage_ConfigureTransportRes{}
		resMsg.ConfigureTransportRes, err = th.api.ConfigureTransport(ctx, input.ConfigureTransport)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_InitTransport:
		resMsg := &prototk.TransportMessage_InitTransportRes{}
		resMsg.InitTransportRes, err = th.api.InitTransport(ctx, input.InitTransport)
		res.ResponseFromTransport = resMsg
	case *prototk.TransportMessage_SendMessage:
		resMsg := &prototk.TransportMessage_SendMessageRes{}
		resMsg.SendMessageRes, err = th.api.SendMessage(ctx, input.SendMessage)
		res.ResponseFromTransport = resMsg
	default:
		err = i18n.NewError(ctx, tkmsgs.MsgPluginUnsupportedRequest, input)
	}
	return th.Wrap(res), err
}

func (th *TransportHandler) Receive(ctx context.Context, req *prototk.ReceiveMessageRequest) (*prototk.ReceiveMessageResponse, error) {
	res, err := th.proxy.RequestFromPlugin(ctx, th.Wrap(&prototk.TransportMessage{
		RequestFromTransport: &prototk.TransportMessage_ReceiveMessage{
			ReceiveMessage: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.TransportMessage_ReceiveMessageRes) *prototk.ReceiveMessageResponse {
		return msg.ReceiveMessageRes
	})
}

func (th *TransportHandler) GetTransportDetails(ctx context.Context, req *prototk.GetTransportDetailsRequest) (*prototk.GetTransportDetailsResponse, error) {
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
	InitTransport      func(context.Context, *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error)
	SendMessage        func(context.Context, *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error)
}

type TransportAPIBase struct {
	Functions *TransportAPIFunctions
}

func (tb *TransportAPIBase) ConfigureTransport(ctx context.Context, req *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.ConfigureTransport)
}

func (tb *TransportAPIBase) InitTransport(ctx context.Context, req *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.InitTransport)
}

func (tb *TransportAPIBase) SendMessage(ctx context.Context, req *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
	return callPluginImpl(ctx, req, tb.Functions.SendMessage)
}
