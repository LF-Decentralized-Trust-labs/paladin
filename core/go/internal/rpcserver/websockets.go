// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpcserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (s *rpcServer) newWSConnection(conn *websocket.Conn) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	c := &webSocketConnection{
		id:      tktypes.ShortID(),
		server:  s,
		conn:    conn,
		send:    make(chan []byte),
		closing: make(chan struct{}),
	}
	c.ctx, c.cancelCtx = context.WithCancel(log.WithLogField(s.bgCtx, "wsconn", c.id))

	s.wsConnections[c.id] = c
	go c.listen()
	go c.sender()
}

func (s *rpcServer) wsClosed(id string) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	delete(s.wsConnections, id)
}

func (s *rpcServer) ethSubList() []*ethSubscription {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	subs := make([]*ethSubscription, 0)
	for _, wsc := range s.wsConnections {
		subs = append(subs, wsc.subscriptions...)
	}
	return subs
}

func (s *rpcServer) processSubscribe(ctx context.Context, rpcReq *rpcbackend.RPCRequest, wsc *webSocketConnection) (*rpcbackend.RPCResponse, bool) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	var eventType string
	if len(rpcReq.Params) > 0 {
		eventType = rpcReq.Params[0].AsString()
	}
	if eventType == "" {
		return rpcbackend.RPCErrorResponse(i18n.NewError(ctx, msgs.MsgJSONRPCInvalidParam, rpcReq.Method, 0, ""),
			rpcReq.ID, rpcbackend.RPCCodeInvalidRequest), false
	}
	var params1 tktypes.RawJSON
	if len(rpcReq.Params) > 1 {
		params1 = rpcReq.Params[1].Bytes()
	}
	sub := &ethSubscription{
		c:         wsc,
		id:        uuid.New().String(),
		eventType: eventType,
		params:    params1,
	}
	wsc.subscriptions = append(wsc.subscriptions, sub)

	return &rpcbackend.RPCResponse{
		ID:      rpcReq.ID,
		JSONRpc: "2.0",
		Result:  fftypes.JSONAnyPtr(fmt.Sprintf(`"%s"`, sub.id)),
	}, true
}

func (s *rpcServer) processUnsubscribe(ctx context.Context, rpcReq *rpcbackend.RPCRequest, wsc *webSocketConnection) (*rpcbackend.RPCResponse, bool) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	var subID string
	if len(rpcReq.Params) > 0 {
		subID = rpcReq.Params[0].AsString()
	}
	if subID == "" {
		return rpcbackend.RPCErrorResponse(i18n.NewError(ctx, msgs.MsgJSONRPCInvalidParam, rpcReq.Method, 0, ""),
			rpcReq.ID, rpcbackend.RPCCodeInvalidRequest), false
	}

	// Trim the sub
	found := false
	var newSubs []*ethSubscription
	for _, s := range wsc.subscriptions {
		if s.id == subID {
			found = true
		} else {
			newSubs = append(newSubs, s)
		}
	}
	wsc.subscriptions = newSubs

	return &rpcbackend.RPCResponse{
		ID:      rpcReq.ID,
		JSONRpc: "2.0",
		Result:  fftypes.JSONAnyPtr(fmt.Sprintf("%t", found)),
	}, true
}

func (s *rpcServer) EthPublish(eventType string, result interface{}) {
	subs := s.ethSubList()
	for _, s := range subs {
		if s.eventType == eventType {
			b, _ := json.Marshal(&ethPublication{
				JSONRPC: "2.0",
				Method:  "eth_subscription",
				Params: ethPublicationParams{
					Subscription: s.id,
					Result:       result,
				},
			})
			select {
			case s.c.send <- b:
			case <-s.c.closing:
			}
		}
	}
}

type ethSubscription struct {
	c         *webSocketConnection
	id        string
	eventType string
	params    tktypes.RawJSON
}

type webSocketConnection struct {
	ctx           context.Context
	cancelCtx     context.CancelFunc
	server        *rpcServer
	id            string
	closeMux      sync.Mutex
	closed        bool
	conn          *websocket.Conn
	subscriptions []*ethSubscription // TODO: Decide JSON/RPC sub model
	send          chan ([]byte)
	closing       chan (struct{})
}

type ethPublicationParams struct {
	Subscription string      `json:"subscription"`
	Result       interface{} `json:"result"`
}

type ethPublication struct {
	JSONRPC string               `json:"jsonrpc"`
	Method  string               `json:"method"`
	Params  ethPublicationParams `json:"params"`
}

func (c *webSocketConnection) close() {
	c.closeMux.Lock()
	if !c.closed {
		c.closed = true
		c.conn.Close()
		close(c.closing)
		c.cancelCtx()
	}
	c.closeMux.Unlock()

	c.server.wsClosed(c.id)
	log.L(c.ctx).Infof("WS disconnected")
}

func (c *webSocketConnection) sender() {
	defer c.close()
	for {
		select {
		case payload := <-c.send:
			log.L(c.ctx).Tracef("Sending: %s", payload)
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				log.L(c.ctx).Errorf("Send failed - closing connection: %s", err)
				return
			}
		case <-c.closing:
			log.L(c.ctx).Infof("Closing")
			return
		}
	}
}

func (c *webSocketConnection) handleMessage(payload []byte) {
	res, _ := c.server.rpcHandler(c.ctx, bytes.NewBuffer(payload), c)
	c.sendMessage(res)
}

func (c *webSocketConnection) sendMessage(res interface{}) {
	payload, err := json.Marshal(res)
	if err != nil {
		log.L(c.ctx).Errorf("Failed to serialize JSON/RPC response %s", payload)
		c.close()
		return
	}
	select {
	case c.send <- payload:
	case <-c.ctx.Done():
	}
}

func (c *webSocketConnection) listen() {
	defer c.close()
	log.L(c.ctx).Infof("WS connected")
	for {
		_, b, err := c.conn.ReadMessage()
		if err != nil {
			log.L(c.ctx).Errorf("Error: %s", err)
			return
		}
		log.L(c.ctx).Tracef("Received: %s", b)
		go c.handleMessage(b)
	}
}
