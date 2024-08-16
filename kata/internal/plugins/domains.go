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
	"fmt"
	"runtime/debug"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	pbp "github.com/kaleido-io/paladin/kata/pkg/proto/plugins"
	pb "google.golang.org/protobuf/proto"
)

type DomainManager interface {
	DomainRegistered(name string, id uuid.UUID, toDomain DomainAPI) (fromDomain DomainCallbacks)
}

type DomainAPI interface {
	ConfigureDomain(context.Context, *pbp.ConfigureDomainRequest) (*pbp.ConfigureDomainResponse, error)
	InitDomain(context.Context, *pbp.InitDomainRequest) (*pbp.InitDomainResponse, error)
	InitDeploy(context.Context, *pbp.InitDeployRequest) (*pbp.InitDeployResponse, error)
	PrepareDeploy(context.Context, *pbp.PrepareDeployRequest) (*pbp.PrepareDeployResponse, error)
	InitTransaction(context.Context, *pbp.InitTransactionRequest) (*pbp.InitTransactionResponse, error)
	AssembleTransaction(context.Context, *pbp.AssembleTransactionRequest) (*pbp.AssembleTransactionResponse, error)
	EndorseTransaction(context.Context, *pbp.EndorseTransactionRequest) (*pbp.EndorseTransactionResponse, error)
	PrepareTransaction(context.Context, *pbp.PrepareTransactionRequest) (*pbp.PrepareTransactionResponse, error)
}

type DomainCallbacks interface {
	FindAvailableStates(context.Context, *pbp.FindAvailableStatesRequest) (*pbp.FindAvailableStatesResponse, error)
}

type domainHandler struct {
	pc        *pluginController
	plugin    *plugin[DomainCallbacks]
	callbacks DomainCallbacks
	send      chan *pbp.DomainMessage
}

func (pc *pluginController) newDomainHandler(plugin *plugin[DomainCallbacks], sendStream pbp.PluginController_ConnectDomainServer) *domainHandler {
	dh := &domainHandler{
		pc:   pc,
		send: make(chan *pbp.DomainMessage),
	}
	dh.callbacks = pc.domainManager.DomainRegistered(plugin.name, plugin.id, dh)
	go dh.sender(sendStream)
	return dh
}

func (dh *domainHandler) ConfigureDomain(ctx context.Context, req *pbp.ConfigureDomainRequest) (res *pbp.ConfigureDomainResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_ConfigureDomain{ConfigureDomain: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_ConfigureDomainRes); ok {
				res = r.ConfigureDomainRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) InitDomain(ctx context.Context, req *pbp.InitDomainRequest) (res *pbp.InitDomainResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_InitDomain{InitDomain: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_InitDomainRes); ok {
				res = r.InitDomainRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) InitDeploy(ctx context.Context, req *pbp.InitDeployRequest) (res *pbp.InitDeployResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_InitDeploy{InitDeploy: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_InitDeployRes); ok {
				res = r.InitDeployRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) PrepareDeploy(ctx context.Context, req *pbp.PrepareDeployRequest) (res *pbp.PrepareDeployResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_PrepareDeploy{PrepareDeploy: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_PrepareDeployRes); ok {
				res = r.PrepareDeployRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) InitTransaction(ctx context.Context, req *pbp.InitTransactionRequest) (res *pbp.InitTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_InitTransaction{InitTransaction: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_InitTransactionRes); ok {
				res = r.InitTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) AssembleTransaction(ctx context.Context, req *pbp.AssembleTransactionRequest) (res *pbp.AssembleTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_AssembleTransaction{AssembleTransaction: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_AssembleTransactionRes); ok {
				res = r.AssembleTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) EndorseTransaction(ctx context.Context, req *pbp.EndorseTransactionRequest) (res *pbp.EndorseTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_EndorseTransaction{EndorseTransaction: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_EndorseTransactionRes); ok {
				res = r.EndorseTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) PrepareTransaction(ctx context.Context, req *pbp.PrepareTransactionRequest) (res *pbp.PrepareTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *pbp.DomainMessage) {
			dm.RequestToDomain = &pbp.DomainMessage_PrepareTransaction{PrepareTransaction: req}
		},
		func(dm *pbp.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*pbp.DomainMessage_PrepareTransactionRes); ok {
				res = r.PrepareTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) sender(sendStream pbp.PluginController_ConnectDomainServer) {
	for msg := range dh.send {
		if err := sendStream.Send(msg); err != nil {
			// gRPC promises to abort the stream is this case, so just log and return
			log.L(context.Background()).Errorf("domain %s stream send error: %s", dh.plugin.name, err)
		}
	}
}

func (dh *domainHandler) close() {
	close(dh.send)
}

func (dh *domainHandler) requestToDomain(ctx context.Context,
	// The protobuf codegen isn't generics friendly, so we use functions here
	setReqBody func(*pbp.DomainMessage),
	getResBody func(*pbp.DomainMessage) bool,
) error {
	req := &pbp.DomainMessage{
		DomainId:    dh.plugin.id.String(),
		MessageId:   uuid.New().String(),
		MessageType: pbp.DomainMessage_REQUEST_TO_DOMAIN,
	}
	setReqBody(req)
	inflight := dh.pc.domainRequests.addInflight(ctx, req)
	defer inflight.cancel()

	log.L(ctx).Infof("DOMAIN(%s)[%s] => %s", dh.plugin.name, req.MessageId, req.RequestToDomain.(pb.Message).ProtoReflect().Descriptor().FullName())

	select {
	case dh.send <- req:
	case <-ctx.Done():
		log.L(ctx).Infof("DOMAIN(%s)[%s] <= TIMEOUT (SEND)", dh.plugin.name, req.MessageId)
		return i18n.NewError(ctx, msgs.MsgContextCanceled)
	}

	res, err := inflight.wait(ctx)
	if err != nil {
		log.L(ctx).Infof("DOMAIN(%s)[%s] <= TIMEOUT (RECEIVE): %s", dh.plugin.name, req.MessageId, err)
		return err
	}
	responseOk := getResBody(res)
	if res.MessageType != pbp.DomainMessage_RESPONSE_FROM_DOMAIN || !responseOk {
		var errorMessage string
		if res.MessageType == pbp.DomainMessage_ERROR_RESPONSE && res.ErrorMessage != nil {
			// We've got a formatted error from the other side
			errorMessage = *res.ErrorMessage
		} else {
			// We got something unexpected - log whatever we've got in full
			errorMessage = jsonProto(res)
		}
		log.L(ctx).Infof("DOMAIN(%s)[%s] <= ERROR: %s", dh.plugin.name, req.MessageId, errorMessage)
		return err
	}

	log.L(ctx).Infof("DOMAIN(%s)[%s] <= %s", dh.plugin.name, req.MessageId, res.ResponseFromDomain.(pb.Message).ProtoReflect().Descriptor().FullName())
	return nil
}

func (dh *domainHandler) requestFromDomain(ctx context.Context, req *pbp.DomainMessage) {
	reply := &pbp.DomainMessage{
		DomainId:      dh.plugin.id.String(),
		MessageId:     uuid.New().String(),
		CorrelationId: &req.MessageId,
		MessageType:   pbp.DomainMessage_RESPONSE_TO_DOMAIN,
	}
	var err error
	defer func() {
		var errorMessage string
		panic := recover()
		if panic != nil {
			log.L(ctx).Errorf("Panic handling domain %s request %s: %s\n%s", dh.plugin.name, req.MessageId, panic, debug.Stack())
			errorMessage = fmt.Sprintf("%s", panic)
		} else if err != nil {
			errorMessage = err.Error()
		}
		if errorMessage != "" {
			reply.MessageType = pbp.DomainMessage_ERROR_RESPONSE
			reply.ErrorMessage = &errorMessage
			reply.ResponseToDomain = nil
		}

	}()
	switch msg := req.RequestFromDomain.(type) {
	case *pbp.DomainMessage_FindAvailableStates:
		var res *pbp.FindAvailableStatesResponse
		res, err = dh.callbacks.FindAvailableStates(ctx, msg.FindAvailableStates)
		if err == nil {
			reply.ResponseToDomain = &pbp.DomainMessage_FindAvailableStatesRes{
				FindAvailableStatesRes: res,
			}
		}
	default:
	}

}
