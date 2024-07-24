// Copyright © 2023 Kaleido, Inc.
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
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/httpserver"
)

type Server interface {
	Register(module *RPCModule)
	Start() error
	Stop()
	HTTPAddr() net.Addr
	WSAddr() net.Addr
}

func NewServer(ctx context.Context, conf *Config) (_ Server, err error) {
	s := &rpcServer{
		bgCtx:         ctx,
		wsConnections: make(map[string]*webSocketConnection),
		rpcModules:    make(map[string]*RPCModule),
	}

	if !conf.HTTP.Disabled {
		if s.httpServer, err = httpserver.NewServer(ctx, "JSON/RPC (HTTP)", &conf.HTTP.Config, http.HandlerFunc(s.httpHandler)); err != nil {
			return nil, err
		}
	}

	if !conf.WS.Disabled {
		s.wsUpgrader = &websocket.Upgrader{
			ReadBufferSize:  int(confutil.ByteSize(conf.WS.ReadBufferSize, 0, *WSDefaults.ReadBufferSize)),
			WriteBufferSize: int(confutil.ByteSize(conf.WS.WriteBufferSize, 0, *WSDefaults.WriteBufferSize)),
		}
		log.L(ctx).Infof("WebSocket server readBufferSize=%d writeBufferSize=%d", s.wsUpgrader.ReadBufferSize, s.wsUpgrader.WriteBufferSize)
		if s.wsServer, err = httpserver.NewServer(ctx, "JSON/RPC (WebSocket)", &conf.WS.Config, http.HandlerFunc(s.wsHandler)); err != nil {
			return nil, err
		}
	}

	return s, err
}

type rpcServer struct {
	bgCtx         context.Context
	httpServer    httpserver.Server
	wsServer      httpserver.Server
	wsMux         sync.Mutex
	wsUpgrader    *websocket.Upgrader
	wsConnections map[string]*webSocketConnection
	rpcModules    map[string]*RPCModule
}

func (s *rpcServer) Register(module *RPCModule) {
	s.rpcModules[module.group] = module
}

func (s *rpcServer) HTTPAddr() (a net.Addr) {
	if s.httpServer != nil {
		a = s.httpServer.Addr()
	}
	return a
}

func (s *rpcServer) WSAddr() (a net.Addr) {
	if s.wsServer != nil {
		a = s.wsServer.Addr()
	}
	return a
}

func (s *rpcServer) httpHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		res.WriteHeader(http.StatusMethodNotAllowed)
	}

	rpcRes, isOK := s.rpcHandler(req.Context(), req.Body)

	res.Header().Set("Content-Type", "application/json; charset=utf-8")
	status := http.StatusOK
	if !isOK {
		status = http.StatusInternalServerError
	}
	res.WriteHeader(status)
	_ = json.NewEncoder(res).Encode(rpcRes)
}

func (s *rpcServer) wsHandler(res http.ResponseWriter, req *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(res, req, nil)
	if err != nil {
		log.L(req.Context()).Errorf("WebSocket upgrade failed: %s", err)
		return
	}
	s.newWSConnection(conn)
}

func (s *rpcServer) Start() (err error) {
	if s.httpServer != nil {
		err = s.httpServer.Start()
	}
	if err == nil && s.wsServer != nil {
		err = s.wsServer.Start()
	}
	return err
}

func (s *rpcServer) Stop() {
	wg := new(sync.WaitGroup)
	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.httpServer.Stop()
		}()
	}
	if s.wsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.wsServer.Stop()
		}()
	}
	wg.Wait()
}
