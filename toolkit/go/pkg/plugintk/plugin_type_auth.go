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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
)

type RPCAuthAPI interface {
	ConfigureRPCAuthorizer(context.Context, *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error)
	Authenticate(context.Context, *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error)
	Authorize(context.Context, *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error)
}

type RPCAuthCallbacks interface {
}

type RPCAuthFactory func(callbacks RPCAuthCallbacks) RPCAuthAPI

func NewRPCAuthPlugin(af RPCAuthFactory) PluginBase {
	impl := &authPlugin{
		factory: af,
	}
	return NewPluginBase(
		prototk.PluginInfo_RPC_AUTH,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.RPCAuthMessage, prototk.RPCAuthMessage], error) {
			return client.ConnectRPCAuthPlugin(ctx)
		},
		impl,
	)
}

type RPCAuthPluginMessage struct {
	m *prototk.RPCAuthMessage
}

func (pm *RPCAuthPluginMessage) Header() *prototk.Header {
	if pm.m.Header == nil {
		pm.m.Header = &prototk.Header{}
	}
	return pm.m.Header
}

func (pm *RPCAuthPluginMessage) RequestToPlugin() any {
	return pm.m.RequestToAuthplugin
}

func (pm *RPCAuthPluginMessage) ResponseFromPlugin() any {
	return pm.m.ResponseFromAuthplugin
}

func (pm *RPCAuthPluginMessage) RequestFromPlugin() any {
	return nil
}

func (pm *RPCAuthPluginMessage) ResponseToPlugin() any {
	return nil
}

func (pm *RPCAuthPluginMessage) Message() *prototk.RPCAuthMessage {
	return pm.m
}

func (pm *RPCAuthPluginMessage) ProtoMessage() pb.Message {
	return pm.m
}

type RPCAuthMessageWrapper struct{}

type authPlugin struct {
	RPCAuthMessageWrapper
	factory RPCAuthFactory
}

func (amw *RPCAuthMessageWrapper) Wrap(m *prototk.RPCAuthMessage) PluginMessage[prototk.RPCAuthMessage] {
	return &RPCAuthPluginMessage{m: m}
}

func (ap *authPlugin) NewHandler(proxy PluginProxy[prototk.RPCAuthMessage]) PluginHandler[prototk.RPCAuthMessage] {
	ah := &rpcAuthHandler{
		authPlugin: ap,
		proxy:      proxy,
	}
	ah.api = ap.factory(ah)
	return ah
}

type rpcAuthHandler struct {
	*authPlugin
	api   RPCAuthAPI
	proxy PluginProxy[prototk.RPCAuthMessage]
}

func (ah *rpcAuthHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.RPCAuthMessage]) (PluginMessage[prototk.RPCAuthMessage], error) {
	req := iReq.Message()
	res := &prototk.RPCAuthMessage{}
	var err error
	switch input := req.RequestToAuthplugin.(type) {
	case *prototk.RPCAuthMessage_ConfigureRpcAuthorizer:
		resMsg := &prototk.RPCAuthMessage_ConfigureRpcAuthorizerRes{}
		resMsg.ConfigureRpcAuthorizerRes, err = ah.api.ConfigureRPCAuthorizer(ctx, input.ConfigureRpcAuthorizer)
		res.ResponseFromAuthplugin = resMsg
	case *prototk.RPCAuthMessage_Authenticate:
		resMsg := &prototk.RPCAuthMessage_AuthenticateRes{}
		resMsg.AuthenticateRes, err = ah.api.Authenticate(ctx, input.Authenticate)
		res.ResponseFromAuthplugin = resMsg
	case *prototk.RPCAuthMessage_Authorize:
		resMsg := &prototk.RPCAuthMessage_AuthorizeRes{}
		resMsg.AuthorizeRes, err = ah.api.Authorize(ctx, input.Authorize)
		res.ResponseFromAuthplugin = resMsg
	default:
		err = i18n.NewError(ctx, pldmsgs.MsgPluginUnsupportedRequest, input)
	}
	return ah.Wrap(res), err
}

type RPCAuthAPIFunctions struct {
	ConfigureRPCAuthorizer func(context.Context, *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error)
	Authenticate           func(context.Context, *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error)
	Authorize              func(context.Context, *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error)
}

type RPCAuthAPIBase struct {
	Functions *RPCAuthAPIFunctions
}

func (aab *RPCAuthAPIBase) ConfigureRPCAuthorizer(ctx context.Context, req *prototk.ConfigureRPCAuthorizerRequest) (*prototk.ConfigureRPCAuthorizerResponse, error) {
	return callPluginImpl(ctx, req, aab.Functions.ConfigureRPCAuthorizer)
}

func (aab *RPCAuthAPIBase) Authenticate(ctx context.Context, req *prototk.AuthenticateRequest) (*prototk.AuthenticateResponse, error) {
	return callPluginImpl(ctx, req, aab.Functions.Authenticate)
}

func (aab *RPCAuthAPIBase) Authorize(ctx context.Context, req *prototk.AuthorizeRequest) (*prototk.AuthorizeResponse, error) {
	return callPluginImpl(ctx, req, aab.Functions.Authorize)
}
