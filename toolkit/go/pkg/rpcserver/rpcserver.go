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
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/httpserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/router"
	"github.com/kaleido-io/paladin/toolkit/pkg/staticserver"
)

type RPCServer interface {
	Start() error
	Stop()
	HTTPAddr() net.Addr
	WSAddr() net.Addr

	Register(module *RPCModule)
	EthPublish(eventType string, result interface{}) // Note this is an `eth_` specific extension, with no ack or reliability
	WSSubscriptionCount(eventType string) int

	WSHandler(w http.ResponseWriter, r *http.Request)   // Provides access to the WebSocket handler directly to be able to install it into another server
	HTTPHandler(w http.ResponseWriter, r *http.Request) // Provides access to the http handler directly to be able to install it into another server
}

func NewRPCServer(ctx context.Context, conf *pldconf.RPCServerConfig) (_ *rpcServer, err error) {
	s := &rpcServer{
		bgCtx:         ctx,
		wsConnections: make(map[string]*webSocketConnection),
		rpcModules:    make(map[string]*RPCModule),
	}

	// Add the HTTP server
	if !conf.HTTP.Disabled {
		r, err := router.NewRouter(s.bgCtx, "JSON/RPC (HTTP)", &conf.HTTP.HTTPServerConfig)
		if err != nil {
			return s, err
		}

		// Add the static servers to the router
		for _, s := range conf.HTTP.StaticServers {
			if !s.Enabled {
				continue
			}
			server := staticserver.NewStaticServer(s)
			r.PathPrefixHandleFunc(s.URLPath, server.HTTPHandler)
		}

		// Add the JSON RPC main handler to the root path
		r.HandleFunc("/", s.httpHandler)

		s.httpServer = r
	}

	// Add the WebSocket server
	if !conf.WS.Disabled {
		s.wsUpgrader = &websocket.Upgrader{
			ReadBufferSize:  int(confutil.ByteSize(conf.WS.ReadBufferSize, 0, *pldconf.WSDefaults.ReadBufferSize)),
			WriteBufferSize: int(confutil.ByteSize(conf.WS.WriteBufferSize, 0, *pldconf.WSDefaults.WriteBufferSize)),
		}
		log.L(ctx).Infof("WebSocket server readBufferSize=%d writeBufferSize=%d", s.wsUpgrader.ReadBufferSize, s.wsUpgrader.WriteBufferSize)
		if s.wsServer, err = httpserver.NewServer(ctx, "JSON/RPC (WebSocket)", &conf.WS.HTTPServerConfig, http.HandlerFunc(s.wsHandler)); err != nil {
			return nil, err
		}
	}

	return s, err
}

// rpcServer implements the RPCServer interface
var _ RPCServer = &rpcServer{}

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

func (s *rpcServer) WSHandler(res http.ResponseWriter, req *http.Request) {
	s.wsHandler(res, req)
}

func (s *rpcServer) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.httpHandler(w, r)
}

func (s *rpcServer) httpHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		res.WriteHeader(http.StatusMethodNotAllowed)
	}

	rpcRes, isOK := s.rpcHandler(req.Context(), req.Body, nil /* not websockets */)

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
