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

type DomainAPI interface {
	ConfigureDomain(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitTransaction(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
}

type DomainCallbacks interface {
	FindAvailableStates(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}

type DomainFactory func(callbacks DomainCallbacks) DomainAPI

func NewDomain(df DomainFactory) PluginBase {
	impl := &domainPlugin{df}
	return NewPluginBase(
		prototk.PluginType_DOMAIN,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.DomainMessage, prototk.DomainMessage], error) {
			return client.ConnectDomain(ctx)
		},
		impl,
	)
}

func DomainImplementation(d *DomainAPIFunctions) DomainAPI {
	return &domainAPIBase{d}
}

type domainPluginMessage struct {
	m *prototk.DomainMessage
}

func (pm *domainPluginMessage) Header() *prototk.Header {
	if pm.m.Header == nil {
		pm.m.Header = &prototk.Header{}
	}
	return pm.m.Header
}

func (pm *domainPluginMessage) RequestToPlugin() any {
	return pm.m.RequestToDomain
}

func (pm *domainPluginMessage) ResponseFromPlugin() any {
	return pm.m.ResponseFromDomain
}

func (pm *domainPluginMessage) RequestFromPlugin() any {
	return pm.m.RequestFromDomain
}

func (pm *domainPluginMessage) ResponseToPlugin() any {
	return pm.m.ResponseToDomain
}

func (pm *domainPluginMessage) Message() *prototk.DomainMessage {
	return pm.m
}

func (pm *domainPluginMessage) ProtoMessage() pb.Message {
	return pm.m
}

type domainPlugin struct {
	factory DomainFactory
}

func (dp *domainPlugin) Wrap(m *prototk.DomainMessage) PluginMessage[prototk.DomainMessage] {
	return &domainPluginMessage{m: m}
}

func (dp *domainPlugin) NewHandler(proxy PluginProxy[prototk.DomainMessage]) PluginHandler[prototk.DomainMessage] {
	dh := &domainHandler{
		domainPlugin: dp,
		proxy:        proxy,
	}
	dh.api = dp.factory(dh)
	return dh
}

type domainHandler struct {
	*domainPlugin
	api   DomainAPI
	proxy PluginProxy[prototk.DomainMessage]
}

func (dp *domainHandler) RequestToPlugin(ctx context.Context, iReq PluginMessage[prototk.DomainMessage]) (PluginMessage[prototk.DomainMessage], error) {
	req := iReq.Message()
	res := &prototk.DomainMessage{}
	var err error
	switch input := req.RequestToDomain.(type) {
	case *prototk.DomainMessage_ConfigureDomain:
		resMsg := &prototk.DomainMessage_ConfigureDomainRes{}
		resMsg.ConfigureDomainRes, err = dp.api.ConfigureDomain(ctx, input.ConfigureDomain)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_InitDomain:
		resMsg := &prototk.DomainMessage_InitDomainRes{}
		resMsg.InitDomainRes, err = dp.api.InitDomain(ctx, input.InitDomain)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_InitDeploy:
		resMsg := &prototk.DomainMessage_InitDeployRes{}
		resMsg.InitDeployRes, err = dp.api.InitDeploy(ctx, input.InitDeploy)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_PrepareDeploy:
		resMsg := &prototk.DomainMessage_PrepareDeployRes{}
		resMsg.PrepareDeployRes, err = dp.api.PrepareDeploy(ctx, input.PrepareDeploy)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_InitTransaction:
		resMsg := &prototk.DomainMessage_InitTransactionRes{}
		resMsg.InitTransactionRes, err = dp.api.InitTransaction(ctx, input.InitTransaction)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_AssembleTransaction:
		resMsg := &prototk.DomainMessage_AssembleTransactionRes{}
		resMsg.AssembleTransactionRes, err = dp.api.AssembleTransaction(ctx, input.AssembleTransaction)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_EndorseTransaction:
		resMsg := &prototk.DomainMessage_EndorseTransactionRes{}
		resMsg.EndorseTransactionRes, err = dp.api.EndorseTransaction(ctx, input.EndorseTransaction)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_PrepareTransaction:
		resMsg := &prototk.DomainMessage_PrepareTransactionRes{}
		resMsg.PrepareTransactionRes, err = dp.api.PrepareTransaction(ctx, input.PrepareTransaction)
		res.ResponseFromDomain = resMsg
	default:
		err = i18n.NewError(ctx, tkmsgs.MsgPluginUnsupportedRequest, input)
	}
	return dp.Wrap(res), err
}

func (dp *domainHandler) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_FindAvailableStates{
			FindAvailableStates: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_FindAvailableStatesRes) *prototk.FindAvailableStatesResponse {
		return msg.FindAvailableStatesRes
	})
}

type DomainAPIFunctions struct {
	ConfigureDomain     func(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain          func(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy          func(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy       func(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitTransaction     func(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction func(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction  func(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction  func(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
}

type domainAPIBase struct {
	d *DomainAPIFunctions
}

func (db *domainAPIBase) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	return callPluginImpl(ctx, req, db.d.ConfigureDomain)
}

func (db *domainAPIBase) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	return callPluginImpl(ctx, req, db.d.InitDomain)
}

func (db *domainAPIBase) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	return callPluginImpl(ctx, req, db.d.InitDeploy)
}

func (db *domainAPIBase) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	return callPluginImpl(ctx, req, db.d.PrepareDeploy)
}

func (db *domainAPIBase) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.d.InitTransaction)
}

func (db *domainAPIBase) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.d.AssembleTransaction)
}

func (db *domainAPIBase) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.d.EndorseTransaction)
}

func (db *domainAPIBase) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.d.PrepareTransaction)
}
