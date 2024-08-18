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
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/domaintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type DomainManager interface {
	DomainRegistered(name string, id uuid.UUID, toDomain domaintk.DomainAPI) (fromDomain domaintk.DomainCallbacks)
}

type domainHandler struct {
	ctx        context.Context
	cancelCtx  context.CancelFunc
	pc         *pluginController
	plugin     *plugin[domaintk.DomainCallbacks]
	callbacks  domaintk.DomainCallbacks
	sendStream prototk.PluginController_ConnectDomainServer
	sendChl    chan *prototk.DomainMessage
	senderDone chan struct{}
}

// Domains connect over this channel, and must announce themselves with their ID to complete the load
func (pc *pluginController) domainServer(stream prototk.PluginController_ConnectDomainServer) error {
	ctx := stream.Context()
	var plugin *plugin[domaintk.DomainCallbacks]
	defer func() {
		// If we got to the point we've initialized, then we're unregistered & uninitialized when we return
		if plugin != nil {
			pc.mux.Lock()
			plugin.registered = false
			plugin.initialized = false
			pc.mux.Unlock()
			if plugin.handler != nil {
				plugin.handler.close()
			}
		}
		// Notify as we might need re-initialization
		pc.tapLoadingProgressed()
	}()
	domainName := "UNINITIALIZED"
	// We are the receiving routine for the gRPC stream (we do NOT send)
	for {
		msg, err := stream.Recv()
		if err != nil {
			log.L(ctx).Errorf("Stream for domain %s failed: %s", domainName, err)
			return err
		}
		if plugin == nil && msg.MessageType != prototk.DomainMessage_REGISTER {
			log.L(ctx).Warnf("Domain %s sent request before registration: %s", domainName, jsonProto(msg))
			continue
		}
		switch msg.MessageType {
		case prototk.DomainMessage_REGISTER:
			if plugin != nil {
				log.L(ctx).Warnf("Domain %s sent request duplicate registration: %s", domainName, jsonProto(msg))
				continue
			}
			plugin, err = getPluginByIDString(pc, pc.domainPlugins, msg.DomainId, prototk.PluginInfo_DOMAIN)
			if err != nil {
				log.L(ctx).Errorf("Request to register an unknown domain %s", msg.DomainId)
				// Close the connection to this plugin
				return err
			}
			// We now have the plugin ready for use
			domainName = plugin.name
			// The handler starts and owns the sending routine (we only use it on this routine, so no locking on this line)
			plugin.handler = pc.newDomainHandler(plugin, stream)
		case prototk.DomainMessage_RESPONSE_FROM_DOMAIN,
			prototk.DomainMessage_ERROR_RESPONSE:
			// If this is an in-flight request, then pass it back to the handler over the request channel
			req := pc.domainRequests.GetInflightCorrelID(msg.CorrelationId)
			if req == nil {
				log.L(ctx).Warnf("Domain %s sent response for unknown/expired request: %s", domainName, jsonProto(msg))
				continue
			}
			req.Complete(msg)
		case prototk.DomainMessage_REQUEST_FROM_DOMAIN:
			// We can't block the stream for this processing, so kick it off to a go routine for the handler
			go plugin.handler.requestFromDomain(ctx, msg)
		}

	}
}

func (pc *pluginController) newDomainHandler(plugin *plugin[domaintk.DomainCallbacks], sendStream prototk.PluginController_ConnectDomainServer) *domainHandler {
	dh := &domainHandler{
		pc:         pc,
		plugin:     plugin,
		sendChl:    make(chan *prototk.DomainMessage),
		senderDone: make(chan struct{}),
		sendStream: sendStream,
	}
	dh.ctx, dh.cancelCtx = context.WithCancel(log.WithLogField(pc.bgCtx, "domain_handler", plugin.name))
	dh.callbacks = pc.domainManager.DomainRegistered(plugin.name, plugin.id, dh)
	go dh.sender()
	return dh
}

func (dh *domainHandler) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (res *prototk.ConfigureDomainResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_ConfigureDomain{ConfigureDomain: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_ConfigureDomainRes); ok {
				res = r.ConfigureDomainRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (res *prototk.InitDomainResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_InitDomain{InitDomain: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_InitDomainRes); ok {
				res = r.InitDomainRes
			}
			return res != nil
		},
	)
	if err == nil {
		// At this point the plugin is initialized
		dh.plugin.notifyInitialized()
	}
	return
}

func (dh *domainHandler) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (res *prototk.InitDeployResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_InitDeploy{InitDeploy: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_InitDeployRes); ok {
				res = r.InitDeployRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (res *prototk.PrepareDeployResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_PrepareDeploy{PrepareDeploy: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_PrepareDeployRes); ok {
				res = r.PrepareDeployRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (res *prototk.InitTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_InitTransaction{InitTransaction: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_InitTransactionRes); ok {
				res = r.InitTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (res *prototk.AssembleTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_AssembleTransaction{AssembleTransaction: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_AssembleTransactionRes); ok {
				res = r.AssembleTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (res *prototk.EndorseTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_EndorseTransaction{EndorseTransaction: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_EndorseTransactionRes); ok {
				res = r.EndorseTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (res *prototk.PrepareTransactionResponse, err error) {
	err = dh.requestToDomain(ctx,
		func(dm *prototk.DomainMessage) {
			dm.RequestToDomain = &prototk.DomainMessage_PrepareTransaction{PrepareTransaction: req}
		},
		func(dm *prototk.DomainMessage) bool {
			if r, ok := dm.ResponseFromDomain.(*prototk.DomainMessage_PrepareTransactionRes); ok {
				res = r.PrepareTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (dh *domainHandler) sender() {
	defer close(dh.senderDone)
	for {
		var msg *prototk.DomainMessage
		select {
		case msg = <-dh.sendChl:
		case <-dh.ctx.Done():
			log.L(dh.ctx).Debugf("domain handler ending")
			return
		}
		if err := dh.sendStream.Send(msg); err != nil {
			// gRPC promises to abort the stream is this case, so just log and return
			log.L(dh.ctx).Errorf("domain %s stream send error: %s", dh.plugin.name, err)
			return
		}
	}
}

// returns once send is dispatched to sendChl, or context is cancelled
func (dh *domainHandler) send(ctx context.Context, msg *prototk.DomainMessage) {
	select {
	case dh.sendChl <- msg:
	case <-ctx.Done():
	}
}

func (dh *domainHandler) close() {
	dh.cancelCtx()
	<-dh.senderDone
}

func (dh *domainHandler) requestToDomain(ctx context.Context,
	// The protobuf codegen isn't generics friendly, so we use functions here
	setReqBody func(*prototk.DomainMessage),
	getResBody func(*prototk.DomainMessage) bool,
) (err error) {
	msgID := uuid.New()
	req := &prototk.DomainMessage{
		DomainId:    dh.plugin.id.String(),
		MessageId:   msgID.String(),
		MessageType: prototk.DomainMessage_REQUEST_TO_DOMAIN,
	}
	setReqBody(req)
	inflight := dh.pc.domainRequests.AddInflight(msgID)
	defer inflight.Cancel()

	startTime := time.Now()
	log.L(ctx).Infof("DOMAIN(%s)[%s] => %T", dh.plugin.name, req.MessageId, req.RequestToDomain)

	dh.send(ctx, req)
	res, err := inflight.Wait(ctx)
	if err != nil {
		log.L(ctx).Infof("DOMAIN(%s)[%s] <= TIMEOUT [%s]: %s", dh.plugin.name, req.MessageId, time.Since(startTime), err)
		return err
	}
	responseOk := getResBody(res)
	if res.MessageType != prototk.DomainMessage_RESPONSE_FROM_DOMAIN || !responseOk {
		var errorMessage string
		if res.MessageType == prototk.DomainMessage_ERROR_RESPONSE && res.ErrorMessage != nil {
			// We've got a formatted error from the other side
			errorMessage = *res.ErrorMessage
		} else {
			// We got something unexpected - log whatever we've got in full
			errorMessage = jsonProto(res)
		}
		log.L(ctx).Infof("DOMAIN(%s)[%s] <= ERROR [%s]: %s", dh.plugin.name, req.MessageId, time.Since(startTime), errorMessage)
		return i18n.NewError(ctx, msgs.MsgPluginInvalidResponse, dh.plugin.def.Plugin.PluginType, dh.plugin.name, errorMessage)
	}

	log.L(ctx).Infof("DOMAIN(%s)[%s] <= %T [%s]", dh.plugin.name, req.MessageId, res.ResponseFromDomain, time.Since(startTime))
	return nil
}

func (dh *domainHandler) requestFromDomain(ctx context.Context, req *prototk.DomainMessage) {
	reply := &prototk.DomainMessage{
		DomainId:      dh.plugin.id.String(),
		MessageId:     uuid.New().String(),
		CorrelationId: &req.MessageId,
		MessageType:   prototk.DomainMessage_RESPONSE_TO_DOMAIN,
	}
	startTime := time.Now()
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
			reply.MessageType = prototk.DomainMessage_ERROR_RESPONSE
			reply.ErrorMessage = &errorMessage
			reply.ResponseToDomain = nil
			log.L(ctx).Infof("FROM_DOMAIN(%s)[%s] <= ERROR [%s]: %s", dh.plugin.name, req.MessageId, time.Since(startTime), errorMessage)
		} else {
			log.L(ctx).Infof("FROM_DOMAIN(%s)[%s] <= %T [%s]", dh.plugin.name, req.MessageId, reply.ResponseToDomain, time.Since(startTime))
		}
		dh.send(ctx, reply)
	}()
	log.L(ctx).Infof("FROM_DOMAIN(%s)[%s] => %T", dh.plugin.name, req.MessageId, req.RequestFromDomain)
	switch msg := req.RequestFromDomain.(type) {
	case *prototk.DomainMessage_FindAvailableStates:
		var res *prototk.FindAvailableStatesResponse
		res, err = dh.callbacks.FindAvailableStates(ctx, msg.FindAvailableStates)
		if err == nil {
			reply.ResponseToDomain = &prototk.DomainMessage_FindAvailableStatesRes{
				FindAvailableStatesRes: res,
			}
		}
	default:
		err = i18n.NewError(ctx, msgs.MsgPluginInvalidRequest, dh.plugin.def.Plugin.PluginType, dh.plugin.name, req.RequestFromDomain)
	}

}
