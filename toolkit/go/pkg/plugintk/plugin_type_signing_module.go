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
package plugintk

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
)

type SigningModuleAPI interface {
	ConfigureSigningModule(context.Context, *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error)
	ResolveKey(context.Context, *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error)
	Sign(context.Context, *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error)
	ListKeys(context.Context, *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error)
	Close(context.Context, *prototk.CloseRequest) (*prototk.CloseResponse, error)
}

type SigningModuleCallbacks interface {
}

type SigningModuleFactory func(callbacks SigningModuleCallbacks) SigningModuleAPI

func NewSigningModule(smf SigningModuleFactory) PluginBase {
	impl := &signingModulePlugin{
		factory: smf,
	}
	return NewPluginBase(
		prototk.PluginInfo_SIGNING_MODULE,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.SigningModuleMessage, prototk.SigningModuleMessage], error) {
			return client.ConnectSigningModule(ctx)
		},
		impl,
	)
}

type SigningModulePluginMessage struct {
	m *prototk.SigningModuleMessage
}

func (pm *SigningModulePluginMessage) Header() *prototk.Header {
	if pm.m.Header == nil {
		pm.m.Header = &prototk.Header{}
	}
	return pm.m.Header
}

func (pm *SigningModulePluginMessage) RequestToPlugin() any {
	return pm.m.RequestToSigningModule
}

func (pm *SigningModulePluginMessage) ResponseFromPlugin() any {
	return pm.m.ResponseFromSigningModule
}

func (pm *SigningModulePluginMessage) RequestFromPlugin() any {
	return pm.m.RequestFromSigningModule
}

func (pm *SigningModulePluginMessage) ResponseToPlugin() any {
	return pm.m.ResponseToSigningModule
}

func (pm *SigningModulePluginMessage) Message() *prototk.SigningModuleMessage {
	return pm.m
}

func (pm *SigningModulePluginMessage) ProtoMessage() pb.Message {
	return pm.m
}

type SigningModuleMessageWrapper struct{}

type signingModulePlugin struct {
	SigningModuleMessageWrapper
	factory SigningModuleFactory
}

func (smmw *SigningModuleMessageWrapper) Wrap(m *prototk.SigningModuleMessage) PluginMessage[prototk.SigningModuleMessage] {
	return &SigningModulePluginMessage{m: m}
}

func (smp *signingModulePlugin) NewHandler(proxy PluginProxy[prototk.SigningModuleMessage]) PluginHandler[prototk.SigningModuleMessage] {
	smh := &signingModuleHandler{
		signingModulePlugin: smp,
		proxy:               proxy,
	}
	smh.api = smp.factory(smh)
	return smh
}

type signingModuleHandler struct {
	*signingModulePlugin
	api   SigningModuleAPI
	proxy PluginProxy[prototk.SigningModuleMessage]
}

func (smh *signingModuleHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.SigningModuleMessage]) (PluginMessage[prototk.SigningModuleMessage], error) {
	req := iReq.Message()
	res := &prototk.SigningModuleMessage{}
	var err error
	switch input := req.RequestToSigningModule.(type) {
	case *prototk.SigningModuleMessage_ConfigureSigningModule:
		resMsg := &prototk.SigningModuleMessage_ConfigureSigningModuleRes{}
		resMsg.ConfigureSigningModuleRes, err = smh.api.ConfigureSigningModule(ctx, input.ConfigureSigningModule)
		res.ResponseFromSigningModule = resMsg
	case *prototk.SigningModuleMessage_ResolveKey:
		resMsg := &prototk.SigningModuleMessage_ResolveKeyRes{}
		resMsg.ResolveKeyRes, err = smh.api.ResolveKey(ctx, input.ResolveKey)
		res.ResponseFromSigningModule = resMsg
	case *prototk.SigningModuleMessage_Sign:
		resMsg := &prototk.SigningModuleMessage_SignRes{}
		resMsg.SignRes, err = smh.api.Sign(ctx, input.Sign)
		res.ResponseFromSigningModule = resMsg
	case *prototk.SigningModuleMessage_ListKeys:
		resMsg := &prototk.SigningModuleMessage_ListKeysRes{}
		resMsg.ListKeysRes, err = smh.api.ListKeys(ctx, input.ListKeys)
		res.ResponseFromSigningModule = resMsg
	case *prototk.SigningModuleMessage_Close:
		resMsg := &prototk.SigningModuleMessage_CloseRes{}
		resMsg.CloseRes, err = smh.api.Close(ctx, input.Close)
		res.ResponseFromSigningModule = resMsg
	default:
		err = i18n.NewError(ctx, pldmsgs.MsgPluginUnsupportedRequest, input)
	}
	return smh.Wrap(res), err
}

type SigningModuleAPIFunctions struct {
	ConfigureSigningModule func(context.Context, *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error)
	ResolveKey             func(context.Context, *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error)
	Sign                   func(context.Context, *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error)
	ListKeys               func(context.Context, *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error)
	Close                  func(context.Context, *prototk.CloseRequest) (*prototk.CloseResponse, error)
}

type SigningModuleAPIBase struct {
	Functions *SigningModuleAPIFunctions
}

func (smb *SigningModuleAPIBase) ConfigureSigningModule(ctx context.Context, req *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
	return callPluginImpl(ctx, req, smb.Functions.ConfigureSigningModule)
}

func (smb *SigningModuleAPIBase) ResolveKey(ctx context.Context, req *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
	return callPluginImpl(ctx, req, smb.Functions.ResolveKey)
}

func (smb *SigningModuleAPIBase) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
	return callPluginImpl(ctx, req, smb.Functions.Sign)
}

func (smb *SigningModuleAPIBase) ListKeys(ctx context.Context, req *prototk.ListKeysRequest) (*prototk.ListKeysResponse, error) {
	return callPluginImpl(ctx, req, smb.Functions.ListKeys)
}

func (smb *SigningModuleAPIBase) Close(ctx context.Context, req *prototk.CloseRequest) (*prototk.CloseResponse, error) {
	return callPluginImpl(ctx, req, smb.Functions.Close)
}
