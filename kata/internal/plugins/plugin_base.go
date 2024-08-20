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
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/inflight"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
)

// managerToPlugin is the interface the common layer exposes to the type of plugin, for it to be able
// to implement the Paladin-to-Plugin initiated exchanges.
//
// functions are used rather than types to set the request/reply inputs, due to specifics
// of the way typing works in the go protobuf codegen library
type managerToPlugin[M any] interface {
	RequestReply(ctx context.Context, pluginId string, reqFn func(plugintk.PluginMessage[M]), resFn func(plugintk.PluginMessage[M]) (ok bool)) error
}

// pluginToManager is the inverse interface where requests are received from plugins, and need
// to be handled by the manager.
//
// Here a function is only needed to store the response - the request is received from the plugin
type pluginToManager[M any] interface {
	RequestReply(ctx context.Context, req plugintk.PluginMessage[M]) (resFn func(plugintk.PluginMessage[M]), err error)
}

// each type of plugin implements a bridge that is just the specific set of operations mapped
// down on the toPlugin and fromPlugin interfaces as appropriate
type pluginBridgeFactory[M any] func(plugin *plugin[M], toPlugin managerToPlugin[M]) (fromPlugin pluginToManager[M], err error)

type plugin[M any] struct {
	pc   *pluginController
	name string
	id   uuid.UUID
	def  *prototk.PluginLoad

	initializing bool
	registered   bool
	initialized  bool
}

type pluginHandler[M any] struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	// Reference back to the section of the controller for this handler
	pc        *pluginController
	pluginMap map[uuid.UUID]*plugin[M]

	// Reference to the manager
	wrapper       plugintk.PluginMessageWrapper[M]
	bridgeFactory pluginBridgeFactory[M]

	// Runtime state that exists from creation time
	inflight   *inflight.InflightManager[uuid.UUID, plugintk.PluginMessage[M]]
	stream     grpc.BidiStreamingServer[M, M]
	sendChl    chan plugintk.PluginMessage[M]
	serverDone chan struct{}
	senderDone chan struct{}

	// Plugin gets bound late after the stream is started
	pluginToManager pluginToManager[M]
}

func (p *plugin[CB]) notifyInitialized() {
	p.pc.mux.Lock()
	p.initialized = true
	p.pc.mux.Unlock()
	log.L(p.pc.bgCtx).Errorf("Plugin load %s [%s] (type=%s) completed", p.def.Plugin, p.id, p.def.Plugin.PluginType)
	p.pc.tapLoadingProgressed()
}

func (p *plugin[CB]) notifyStopped() {
	p.pc.mux.Lock()
	p.registered = false
	p.initialized = false
	p.pc.mux.Unlock()
	log.L(p.pc.bgCtx).Errorf("Plugin stopped %s [%s] (type=%s)", p.def.Plugin, p.id, p.def.Plugin.PluginType)
	p.pc.tapLoadingProgressed()
}

func newPluginHandler[M any](pc *pluginController, pluginMap map[uuid.UUID]*plugin[M], stream grpc.BidiStreamingServer[M, M], wrapper plugintk.PluginMessageWrapper[M], bridgeFactory pluginBridgeFactory[M]) *pluginHandler[M] {
	ph := &pluginHandler[M]{
		pc:            pc,
		pluginMap:     pluginMap,
		wrapper:       wrapper,
		bridgeFactory: bridgeFactory,
		inflight:      inflight.NewInflightManager[uuid.UUID, plugintk.PluginMessage[M]](uuid.Parse),
		sendChl:       make(chan plugintk.PluginMessage[M]),
		serverDone:    make(chan struct{}),
		senderDone:    make(chan struct{}),
		stream:        stream,
	}
	ph.ctx, ph.cancelCtx = context.WithCancel(ph.pc.bgCtx)
	return ph
}

// Plugins connect over this channel, and must announce themselves with their ID to complete the load
func (ph *pluginHandler[M]) serve() error {
	defer close(ph.serverDone)

	go ph.sender()

	serverCtx := ph.stream.Context()
	var plugin *plugin[M]
	defer func() {
		// If we got to the point we've initialized, then we're unregistered & uninitialized when we return
		if plugin != nil {
			plugin.notifyStopped()
		}
		ph.close()
	}()
	// We are the receiving routine for the gRPC stream (we do NOT send)
	pluginId := "" // plugin ID not known until it registers
	for {
		iMsg, err := ph.stream.Recv()
		if err != nil {
			log.L(serverCtx).Errorf("Stream for plugin ailed: %s", err)
			return err
		}
		msg := ph.wrapper.Wrap(iMsg)
		header := msg.Header()
		if plugin == nil && header.MessageType != prototk.Header_REGISTER {
			log.L(serverCtx).Warnf("Plugin sent request before registration: %s", plugintk.PluginMessageToJSON(msg))
			continue
		}
		switch header.MessageType {
		case prototk.Header_REGISTER:
			if plugin != nil {
				log.L(serverCtx).Warnf("Plugin sent request duplicate registration: %s", plugintk.PluginMessageToJSON(msg))
				continue
			}
			plugin, err = getPluginByIDString(ph.pc, ph.pluginMap, header.PluginId, prototk.PluginInfo_DOMAIN)
			if err != nil {
				log.L(serverCtx).Errorf("Request to register an unknown plugin %s", header.PluginId)
				// Close the connection to this plugin
				return err
			}
			// Update the context for us, and for request/reply, to include details of the plugin
			debugInfo := fmt.Sprintf("%s[%s/%s]", plugin.def.Plugin.PluginType, plugin.name, plugin.id)
			ph.ctx = log.WithLogField(ph.ctx, "plugin", debugInfo) // dirty write - as it's just debug info being added
			serverCtx = log.WithLogField(serverCtx, "plugin", debugInfo)
			// We now have the plugin ready for use
			pluginId = plugin.id.String()
			ph.pluginToManager, err = ph.bridgeFactory(plugin, ph)
			if err != nil {
				log.L(serverCtx).Errorf("Plugin factory failed %s: %s", header.PluginId, err)
				// Close the connection to this plugin
				return err
			}
		case prototk.Header_RESPONSE_FROM_PLUGIN,
			prototk.Header_ERROR_RESPONSE:
			// If this is an in-flight request, then pass it back to the handler over the request channel
			if header.CorrelationId == nil {
				log.L(serverCtx).Warnf("Plugin sent response with missing correlationID: %s", plugintk.PluginMessageToJSON(msg))
				continue
			}
			req := ph.inflight.GetInflightStr(*header.CorrelationId)
			if req == nil {
				log.L(serverCtx).Warnf("Plugin sent response for unknown/expired request: %s", plugintk.PluginMessageToJSON(msg))
				continue
			}
			req.Complete(msg)
		case prototk.Header_REQUEST_FROM_PLUGIN:
			// We can't block the stream for this processing, so kick it off to a go routine for the handler
			go ph.handleRequestFromPlugin(serverCtx, pluginId, msg)
		}

	}
}

func (ph *pluginHandler[M]) sender() {
	defer close(ph.senderDone)
	for {
		var msg plugintk.PluginMessage[M]
		select {
		case msg = <-ph.sendChl:
		case <-ph.ctx.Done():
			log.L(ph.ctx).Debugf("domain handler ending")
			return
		}
		if err := ph.stream.Send(msg.Message()); err != nil {
			// gRPC promises to abort the stream is this case, so just log and return
			log.L(ph.ctx).Errorf("stream send error: %s", err)
			return
		}
	}
}

// returns once send is dispatched to sendChl, or context is cancelled
func (ph *pluginHandler[M]) send(msg plugintk.PluginMessage[M]) {
	select {
	case ph.sendChl <- msg:
	case <-ph.ctx.Done():
	}
}

func (ph *pluginHandler[M]) close() {
	ph.cancelCtx()
	<-ph.senderDone
}

// Go routine started for each request
func (ph *pluginHandler[M]) handleRequestFromPlugin(ctx context.Context, pluginId string, req plugintk.PluginMessage[M]) {
	// Call the manager
	startTime := time.Now()
	log.L(ctx).Infof("[%s] ==> %T", req.Header().MessageId, req.RequestToPlugin())
	resFn, err := ph.pluginToManager.RequestReply(ctx, req)

	// Build and send the reply (success or error)
	replyID := uuid.NewString()
	res := ph.wrapper.Wrap(new(M))
	header := res.Header()
	header.PluginId = pluginId
	header.MessageId = replyID
	header.CorrelationId = &req.Header().MessageId
	if err != nil {
		log.L(ctx).Infof("[%s] <== [%s] ERROR [%s]", req.Header().MessageId, replyID, startTime)
		header.MessageType = prototk.Header_ERROR_RESPONSE
		errorMessage := err.Error()
		header.ErrorMessage = &errorMessage
	} else {
		log.L(ctx).Infof("[%s] <== [%s] %T [%s]", req.Header().MessageId, replyID, res.ResponseToPlugin(), startTime)
		header.MessageType = prototk.Header_RESPONSE_TO_PLUGIN
		resFn(res)
	}
	ph.send(res)
}

func (ph *pluginHandler[M]) RequestReply(ctx context.Context, pluginId string, reqFn func(plugintk.PluginMessage[M]), resFn func(plugintk.PluginMessage[M]) (ok bool)) error {
	// Log under our context so we get the plugin ID
	reqID := uuid.New()
	l := log.L(ph.ctx)

	// Caller is responsible for filling in the body
	req := ph.wrapper.Wrap(new(M))
	reqFn(req)

	// We are responsible for the header
	header := req.Header()
	header.PluginId = pluginId
	header.CorrelationId = nil
	header.MessageId = reqID.String()
	header.MessageType = prototk.Header_REQUEST_TO_PLUGIN
	header.ErrorMessage = nil

	// Create the in-flight record - under the request context (inflight manager will be cancelled if we end)
	inflight := ph.inflight.AddInflight(ctx, reqID)
	defer inflight.Cancel()
	l.Infof("[%s] ==> %T", reqID, req.RequestToPlugin())

	// Send the request
	ph.send(req)

	// Wait for a response, or cancel
	res, err := inflight.Wait()
	if err != nil {
		l.Warnf("[%s] <== CANCELLED [%s]", reqID, inflight.Age())
		return err
	}
	if res.Header().MessageType == prototk.Header_ERROR_RESPONSE {
		errMessage := ""
		if res.Header().ErrorMessage != nil {
			errMessage = *res.Header().ErrorMessage
		} else {
			errMessage = plugintk.PluginMessageToJSON(res)
		}
		l.Errorf("[%s] <== ERROR [%s]: %s", reqID, inflight.Age(), errMessage)
		return i18n.NewError(ctx, msgs.MsgPluginError, errMessage)
	}

	responseOk := resFn(res)
	if !responseOk {
		l.Errorf("[%s] <== BAD_RESPONSE [%s]: %T", reqID, inflight.Age(), res.ResponseFromPlugin())
		return i18n.NewError(ctx, msgs.MsgPluginBadResponseBody, res.ResponseFromPlugin())
	}

	l.Infof("[%s] <== [%s] %T [%s]", reqID, res.Header().MessageId, req.ResponseFromPlugin(), inflight.Age())
	return nil
}

// This is a bit faffy due to the type system of protobuf codegen
func callManagerImpl[M, ReqType, ResType any](ctx context.Context,
	req *ReqType,
	run func(context.Context, *ReqType) (*ResType, error),
	wrap func(*M, *ResType),
) (func(plugintk.PluginMessage[M]), error) {
	iRes, err := run(ctx, req)
	if err != nil {
		return nil, err
	}
	return func(resMsg plugintk.PluginMessage[M]) {
		wrap(resMsg.Message(), iRes)
	}, nil
}
