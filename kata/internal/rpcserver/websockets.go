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

	"github.com/gorilla/websocket"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

func (s *rpcServer) newWSConnection(conn *websocket.Conn) {
	s.wsMux.Lock()
	defer s.wsMux.Unlock()

	c := &webSocketConnection{
		id:      types.ShortID(),
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

type webSocketConnection struct {
	ctx       context.Context
	cancelCtx context.CancelFunc
	server    *rpcServer
	id        string
	closeMux  sync.Mutex
	closed    bool
	conn      *websocket.Conn
	send      chan ([]byte)
	closing   chan (struct{})
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
	res, _ := c.server.rpcHandler(c.ctx, bytes.NewBuffer(payload))
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
