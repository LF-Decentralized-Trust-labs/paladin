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

type DomainAPI interface {
	ConfigureDomain(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitContract(context.Context, *prototk.InitContractRequest) (*prototk.InitContractResponse, error)
	InitTransaction(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
	HandleEventBatch(context.Context, *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error)
	Sign(context.Context, *prototk.SignRequest) (*prototk.SignResponse, error)
	GetVerifier(context.Context, *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error)
	ValidateStateHashes(context.Context, *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error)
	InitCall(context.Context, *prototk.InitCallRequest) (*prototk.InitCallResponse, error)
	ExecCall(context.Context, *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error)
	BuildReceipt(context.Context, *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error)
	ConfigurePrivacyGroup(context.Context, *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error)
	InitPrivacyGroup(context.Context, *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error)
	WrapPrivacyGroupEVMTX(context.Context, *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error)
}

type DomainCallbacks interface {
	FindAvailableStates(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
	EncodeData(context.Context, *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error)
	DecodeData(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error)
	RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error)
	SendTransaction(ctx context.Context, tx *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error)
	LocalNodeName(context.Context, *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error)
	GetStatesByID(ctx context.Context, req *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error)
}

type DomainFactory func(callbacks DomainCallbacks) DomainAPI

func NewDomain(df DomainFactory) PluginBase {
	impl := &domainPlugin{
		factory: df,
	}
	return NewPluginBase(
		prototk.PluginInfo_DOMAIN,
		func(ctx context.Context, client prototk.PluginControllerClient) (grpc.BidiStreamingClient[prototk.DomainMessage, prototk.DomainMessage], error) {
			return client.ConnectDomain(ctx)
		},
		impl,
	)
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

type DomainMessageWrapper struct{}

type domainPlugin struct {
	DomainMessageWrapper
	factory DomainFactory
}

func (dp *DomainMessageWrapper) Wrap(m *prototk.DomainMessage) PluginMessage[prototk.DomainMessage] {
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
	case *prototk.DomainMessage_InitContract:
		resMsg := &prototk.DomainMessage_InitContractRes{}
		resMsg.InitContractRes, err = dp.api.InitContract(ctx, input.InitContract)
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
	case *prototk.DomainMessage_HandleEventBatch:
		resMsg := &prototk.DomainMessage_HandleEventBatchRes{}
		resMsg.HandleEventBatchRes, err = dp.api.HandleEventBatch(ctx, input.HandleEventBatch)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_Sign:
		resMsg := &prototk.DomainMessage_SignRes{}
		resMsg.SignRes, err = dp.api.Sign(ctx, input.Sign)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_GetVerifier:
		resMsg := &prototk.DomainMessage_GetVerifierRes{}
		resMsg.GetVerifierRes, err = dp.api.GetVerifier(ctx, input.GetVerifier)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_ValidateStateHashes:
		resMsg := &prototk.DomainMessage_ValidateStateHashesRes{}
		resMsg.ValidateStateHashesRes, err = dp.api.ValidateStateHashes(ctx, input.ValidateStateHashes)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_InitCall:
		resMsg := &prototk.DomainMessage_InitCallRes{}
		resMsg.InitCallRes, err = dp.api.InitCall(ctx, input.InitCall)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_ExecCall:
		resMsg := &prototk.DomainMessage_ExecCallRes{}
		resMsg.ExecCallRes, err = dp.api.ExecCall(ctx, input.ExecCall)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_BuildReceipt:
		resMsg := &prototk.DomainMessage_BuildReceiptRes{}
		resMsg.BuildReceiptRes, err = dp.api.BuildReceipt(ctx, input.BuildReceipt)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_ConfigurePrivacyGroup:
		resMsg := &prototk.DomainMessage_ConfigurePrivacyGroupRes{}
		resMsg.ConfigurePrivacyGroupRes, err = dp.api.ConfigurePrivacyGroup(ctx, input.ConfigurePrivacyGroup)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_InitPrivacyGroup:
		resMsg := &prototk.DomainMessage_InitPrivacyGroupRes{}
		resMsg.InitPrivacyGroupRes, err = dp.api.InitPrivacyGroup(ctx, input.InitPrivacyGroup)
		res.ResponseFromDomain = resMsg
	case *prototk.DomainMessage_WrapPrivacyGroupEvmtx:
		resMsg := &prototk.DomainMessage_WrapPrivacyGroupEvmtxRes{}
		resMsg.WrapPrivacyGroupEvmtxRes, err = dp.api.WrapPrivacyGroupEVMTX(ctx, input.WrapPrivacyGroupEvmtx)
		res.ResponseFromDomain = resMsg
	default:
		err = i18n.NewError(ctx, pldmsgs.MsgPluginUnsupportedRequest, input)
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

func (dp *domainHandler) EncodeData(ctx context.Context, req *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_EncodeData{
			EncodeData: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_EncodeDataRes) *prototk.EncodeDataResponse {
		return msg.EncodeDataRes
	})
}

func (dp *domainHandler) DecodeData(ctx context.Context, req *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_DecodeData{
			DecodeData: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_DecodeDataRes) *prototk.DecodeDataResponse {
		return msg.DecodeDataRes
	})
}

func (dp *domainHandler) RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_RecoverSigner{
			RecoverSigner: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_RecoverSignerRes) *prototk.RecoverSignerResponse {
		return msg.RecoverSignerRes
	})
}

func (dp *domainHandler) SendTransaction(ctx context.Context, req *prototk.SendTransactionRequest) (*prototk.SendTransactionResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_SendTransaction{
			SendTransaction: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_SendTransactionRes) *prototk.SendTransactionResponse {
		return msg.SendTransactionRes
	})
}

func (dp *domainHandler) LocalNodeName(ctx context.Context, req *prototk.LocalNodeNameRequest) (*prototk.LocalNodeNameResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_LocalNodeName{
			LocalNodeName: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_LocalNodeNameRes) *prototk.LocalNodeNameResponse {
		return msg.LocalNodeNameRes
	})
}

func (dp *domainHandler) GetStatesByID(ctx context.Context, req *prototk.GetStatesByIDRequest) (*prototk.GetStatesByIDResponse, error) {
	res, err := dp.proxy.RequestFromPlugin(ctx, dp.Wrap(&prototk.DomainMessage{
		RequestFromDomain: &prototk.DomainMessage_GetStatesById{
			GetStatesById: req,
		},
	}))
	return responseToPluginAs(ctx, res, err, func(msg *prototk.DomainMessage_GetStatesByIdRes) *prototk.GetStatesByIDResponse {
		return msg.GetStatesByIdRes
	})
}

type DomainAPIFunctions struct {
	ConfigureDomain       func(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain            func(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy            func(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy         func(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitContract          func(context.Context, *prototk.InitContractRequest) (*prototk.InitContractResponse, error)
	InitTransaction       func(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction   func(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction    func(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction    func(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
	HandleEventBatch      func(context.Context, *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error)
	Sign                  func(context.Context, *prototk.SignRequest) (*prototk.SignResponse, error)
	GetVerifier           func(context.Context, *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error)
	ValidateStateHashes   func(context.Context, *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error)
	InitCall              func(context.Context, *prototk.InitCallRequest) (*prototk.InitCallResponse, error)
	ExecCall              func(context.Context, *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error)
	BuildReceipt          func(context.Context, *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error)
	ConfigurePrivacyGroup func(context.Context, *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error)
	InitPrivacyGroup      func(context.Context, *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error)
	WrapPrivacyGroupEVMTX func(context.Context, *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error)
}

type DomainAPIBase struct {
	Functions *DomainAPIFunctions
}

func (db *DomainAPIBase) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.ConfigureDomain)
}

func (db *DomainAPIBase) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitDomain)
}

func (db *DomainAPIBase) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitDeploy)
}

func (db *DomainAPIBase) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.PrepareDeploy)
}

func (db *DomainAPIBase) InitContract(ctx context.Context, req *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitContract)
}

func (db *DomainAPIBase) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitTransaction)
}

func (db *DomainAPIBase) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.AssembleTransaction)
}

func (db *DomainAPIBase) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.EndorseTransaction)
}

func (db *DomainAPIBase) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.PrepareTransaction)
}

func (db *DomainAPIBase) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.HandleEventBatch)
}

func (db *DomainAPIBase) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.Sign)
}

func (db *DomainAPIBase) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.GetVerifier)
}

func (db *DomainAPIBase) ValidateStateHashes(ctx context.Context, req *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.ValidateStateHashes)
}

func (db *DomainAPIBase) InitCall(ctx context.Context, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitCall)
}

func (db *DomainAPIBase) ExecCall(ctx context.Context, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.ExecCall)
}

func (db *DomainAPIBase) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.BuildReceipt)
}

func (db *DomainAPIBase) ConfigurePrivacyGroup(ctx context.Context, req *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.ConfigurePrivacyGroup)
}

func (db *DomainAPIBase) InitPrivacyGroup(ctx context.Context, req *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.InitPrivacyGroup)
}

func (db *DomainAPIBase) WrapPrivacyGroupEVMTX(ctx context.Context, req *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
	return callPluginImpl(ctx, req, db.Functions.WrapPrivacyGroupEVMTX)
}
