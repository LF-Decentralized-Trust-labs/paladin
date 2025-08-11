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
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
)

func (s *rpcServer) newWSConnection(conn *websocket.Conn) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	c := &webSocketConnection{
		id:             pldtypes.ShortID(),
		server:         s,
		conn:           conn,
		asyncInstances: make(map[uuid.UUID]*asyncWrapper),
		send:           make(chan []byte),
		closing:        make(chan struct{}),
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

type webSocketConnection struct {
	ctx            context.Context
	cancelCtx      context.CancelFunc
	server         *rpcServer
	id             string
	closeMux       sync.Mutex
	closed         bool
	conn           *websocket.Conn
	asyncMux       sync.Mutex
	asyncInstances map[uuid.UUID]*asyncWrapper
	send           chan ([]byte)
	closing        chan (struct{})
}

type asyncWrapper struct {
	id       uuid.UUID
	wsc      *webSocketConnection
	instance RPCAsyncInstance
}

func (aw *asyncWrapper) Send(method string, params any) {
	aw.wsc.sendMessage(&rpcclient.RPCResponse{
		JSONRpc: "2.0",
		Method:  method,
		Params:  pldtypes.JSONString(params),
	})
}

func (aw *asyncWrapper) Closed() {
	aw.wsc.handleCloseAsync(aw)
}

func (aw *asyncWrapper) ID() string {
	return aw.id.String()
}

func (c *webSocketConnection) asyncHandlerList() []RPCAsyncInstance {
	c.asyncMux.Lock()
	defer c.asyncMux.Unlock()

	handlers := make([]RPCAsyncInstance, 0, len(c.asyncInstances))
	for _, aw := range c.asyncInstances {
		handlers = append(handlers, aw.instance)
	}
	return handlers
}

func (c *webSocketConnection) handleCloseAsync(aw *asyncWrapper) {
	c.asyncMux.Lock()
	defer c.asyncMux.Unlock()

	delete(c.asyncInstances, aw.id)
}

func (c *webSocketConnection) handleNewAsync(ctx context.Context, rpcReq *rpcclient.RPCRequest, ash RPCAsyncHandler) (res *rpcclient.RPCResponse) {

	aw := &asyncWrapper{wsc: c, id: uuid.New()}
	aw.instance, res = ash.HandleStart(ctx, rpcReq, aw)

	c.asyncMux.Lock()
	defer c.asyncMux.Unlock()

	isOK := res.Error == nil
	if isOK && aw.instance != nil {
		c.asyncInstances[aw.id] = aw
	}
	return res
}

func (c *webSocketConnection) handleLifecycle(ctx context.Context, rpcReq *rpcclient.RPCRequest, ash RPCAsyncHandler) *rpcclient.RPCResponse {
	// Just passed for on-way handling by the async handler
	return ash.HandleLifecycle(ctx, rpcReq)

}

func (c *webSocketConnection) close() {
	c.closeMux.Lock()
	if !c.closed {
		c.closed = true
		_ = c.conn.Close()
		close(c.closing)
		c.cancelCtx()
	}
	c.closeMux.Unlock()

	// Let all the aysnc handlers know to cleanup
	for _, ah := range c.asyncHandlerList() {
		ah.ConnectionClosed()
	}

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
	r := c.server.rpcHandler(c.ctx, bytes.NewBuffer(payload), c)
	if r.sendRes {
		c.sendMessage(r.res)
	}
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
