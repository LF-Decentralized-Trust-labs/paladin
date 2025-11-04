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

// The gRPC stream connected to by domain plugins
func (pm *pluginManager) ConnectDomain(stream prototk.PluginController_ConnectDomainServer) error {
	handler := newPluginHandler(pm, prototk.PluginInfo_DOMAIN, pm.domainPlugins, stream,
		&plugintk.DomainMessageWrapper{},
		func(plugin *plugin[prototk.DomainMessage], toPlugin managerToPlugin[prototk.DomainMessage]) (pluginToManager pluginToManager[prototk.DomainMessage], err error) {
			br := &domainBridge{
				plugin:     plugin,
				pluginType: plugin.def.Plugin.PluginType.String(),
				pluginName: plugin.name,
				pluginId:   plugin.id.String(),
				toPlugin:   toPlugin,
			}
			br.manager, err = pm.domainManager.DomainRegistered(plugin.name, br)
			if err != nil {
				return nil, err
			}
			return br, nil
		})
	return handler.serve()
}

type domainBridge struct {
	plugin     *plugin[prototk.DomainMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.DomainMessage]
	manager    plugintk.DomainCallbacks
}

// DomainManager calls this when it is satisfied the domain is fully initialized.
// WaitForStart will block until this is done.
func (br *domainBridge) Initialized() {
	br.plugin.notifyInitialized()
}

// requests to callbacks in the domain manager
func (br *domainBridge) RequestReply(ctx context.Context, reqMsg plugintk.PluginMessage[prototk.DomainMessage]) (resFn func(plugintk.PluginMessage[prototk.DomainMessage]), err error) {
	switch req := reqMsg.Message().RequestFromDomain.(type) {
	case *prototk.DomainMessage_FindAvailableStates:
		return callManagerImpl(ctx, req.FindAvailableStates,
			br.manager.FindAvailableStates,
			func(resMsg *prototk.DomainMessage, res *prototk.FindAvailableStatesResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_FindAvailableStatesRes{
					FindAvailableStatesRes: res,
				}
			},
		)
	case *prototk.DomainMessage_EncodeData:
		return callManagerImpl(ctx, req.EncodeData,
			br.manager.EncodeData,
			func(resMsg *prototk.DomainMessage, res *prototk.EncodeDataResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_EncodeDataRes{
					EncodeDataRes: res,
				}
			},
		)
	case *prototk.DomainMessage_DecodeData:
		return callManagerImpl(ctx, req.DecodeData,
			br.manager.DecodeData,
			func(resMsg *prototk.DomainMessage, res *prototk.DecodeDataResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_DecodeDataRes{
					DecodeDataRes: res,
				}
			},
		)
	case *prototk.DomainMessage_RecoverSigner:
		return callManagerImpl(ctx, req.RecoverSigner,
			br.manager.RecoverSigner,
			func(resMsg *prototk.DomainMessage, res *prototk.RecoverSignerResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_RecoverSignerRes{
					RecoverSignerRes: res,
				}
			},
		)
	case *prototk.DomainMessage_SendTransaction:
		return callManagerImpl(ctx, req.SendTransaction,
			br.manager.SendTransaction,
			func(resMsg *prototk.DomainMessage, res *prototk.SendTransactionResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_SendTransactionRes{
					SendTransactionRes: res,
				}
			},
		)
	case *prototk.DomainMessage_LocalNodeName:
		return callManagerImpl(ctx, req.LocalNodeName,
			br.manager.LocalNodeName,
			func(resMsg *prototk.DomainMessage, res *prototk.LocalNodeNameResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_LocalNodeNameRes{
					LocalNodeNameRes: res,
				}
			},
		)
	case *prototk.DomainMessage_GetStatesById:
		return callManagerImpl(ctx, req.GetStatesById,
			br.manager.GetStatesByID,
			func(resMsg *prototk.DomainMessage, res *prototk.GetStatesByIDResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_GetStatesByIdRes{
					GetStatesByIdRes: res,
				}
			},
		)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPluginBadRequestBody, req)
	}
}

func (br *domainBridge) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (res *prototk.ConfigureDomainResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_ConfigureDomain{ConfigureDomain: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_ConfigureDomainRes); ok {
				res = r.ConfigureDomainRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (res *prototk.InitDomainResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitDomain{InitDomain: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitDomainRes); ok {
				res = r.InitDomainRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (res *prototk.InitDeployResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitDeploy{InitDeploy: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitDeployRes); ok {
				res = r.InitDeployRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (res *prototk.PrepareDeployResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_PrepareDeploy{PrepareDeploy: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_PrepareDeployRes); ok {
				res = r.PrepareDeployRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitContract(ctx context.Context, req *prototk.InitContractRequest) (res *prototk.InitContractResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitContract{InitContract: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitContractRes); ok {
				res = r.InitContractRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (res *prototk.InitTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitTransaction{InitTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitTransactionRes); ok {
				res = r.InitTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (res *prototk.AssembleTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_AssembleTransaction{AssembleTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_AssembleTransactionRes); ok {
				res = r.AssembleTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (res *prototk.EndorseTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_EndorseTransaction{EndorseTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_EndorseTransactionRes); ok {
				res = r.EndorseTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (res *prototk.PrepareTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_PrepareTransaction{PrepareTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_PrepareTransactionRes); ok {
				res = r.PrepareTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (res *prototk.HandleEventBatchResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_HandleEventBatch{HandleEventBatch: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_HandleEventBatchRes); ok {
				res = r.HandleEventBatchRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) Sign(ctx context.Context, req *prototk.SignRequest) (res *prototk.SignResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_Sign{Sign: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_SignRes); ok {
				res = r.SignRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (res *prototk.GetVerifierResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_GetVerifier{GetVerifier: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_GetVerifierRes); ok {
				res = r.GetVerifierRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) ValidateStateHashes(ctx context.Context, req *prototk.ValidateStateHashesRequest) (res *prototk.ValidateStateHashesResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_ValidateStateHashes{ValidateStateHashes: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_ValidateStateHashesRes); ok {
				res = r.ValidateStateHashesRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitCall(ctx context.Context, req *prototk.InitCallRequest) (res *prototk.InitCallResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitCall{InitCall: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitCallRes); ok {
				res = r.InitCallRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) ExecCall(ctx context.Context, req *prototk.ExecCallRequest) (res *prototk.ExecCallResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_ExecCall{ExecCall: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_ExecCallRes); ok {
				res = r.ExecCallRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (res *prototk.BuildReceiptResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_BuildReceipt{BuildReceipt: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_BuildReceiptRes); ok {
				res = r.BuildReceiptRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) ConfigurePrivacyGroup(ctx context.Context, req *prototk.ConfigurePrivacyGroupRequest) (res *prototk.ConfigurePrivacyGroupResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_ConfigurePrivacyGroup{ConfigurePrivacyGroup: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_ConfigurePrivacyGroupRes); ok {
				res = r.ConfigurePrivacyGroupRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitPrivacyGroup(ctx context.Context, req *prototk.InitPrivacyGroupRequest) (res *prototk.InitPrivacyGroupResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitPrivacyGroup{InitPrivacyGroup: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitPrivacyGroupRes); ok {
				res = r.InitPrivacyGroupRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) WrapPrivacyGroupEVMTX(ctx context.Context, req *prototk.WrapPrivacyGroupEVMTXRequest) (res *prototk.WrapPrivacyGroupEVMTXResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_WrapPrivacyGroupEvmtx{WrapPrivacyGroupEvmtx: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_WrapPrivacyGroupEvmtxRes); ok {
				res = r.WrapPrivacyGroupEvmtxRes
			}
			return res != nil
		},
	)
	return
}
