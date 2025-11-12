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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/httpserver"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/router"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/staticserver"
	"github.com/gorilla/websocket"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const authResultKey contextKey = "authResult" // For storing authenticated results (used for both HTTP and WebSocket)

type RPCServer interface {
	Start() error
	Stop()
	HTTPAddr() net.Addr
	WSAddr() net.Addr

	Register(module *RPCModule)
	SetAuthorizers(auths []Authorizer)

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
			ReadBufferSize:  int(confutil.ByteSize(conf.WS.ReadBufferSize, 0, *pldconf.RPCServerConfigDefaults.WS.ReadBufferSize)),
			WriteBufferSize: int(confutil.ByteSize(conf.WS.WriteBufferSize, 0, *pldconf.RPCServerConfigDefaults.WS.WriteBufferSize)),
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
	authorizers   []Authorizer
}

type Authorizer interface {
	Authenticate(ctx context.Context, headers map[string]string) (result string, err error)
	Authorize(ctx context.Context, result string, method string, payload []byte) bool
}

func (s *rpcServer) SetAuthorizers(auths []Authorizer) {
	s.authorizers = auths
}

func (s *rpcServer) Register(module *RPCModule) {
	log.L(s.bgCtx).Debugf("RPC module %s registered: %v", module.group, module.MethodNames())
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

	ctx := req.Context()

	// Authenticate BEFORE parsing request body if authorizers are configured
	authenticated, authenticationResults := s.authenticate(ctx, req)
	if !authenticated {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}
	if authenticationResults != nil {
		// Store authentication results in context for use in authorization phase
		ctx = context.WithValue(ctx, authResultKey, authenticationResults)
	}

	r := s.rpcHandler(ctx, req.Body, nil /* not websockets */)

	res.Header().Set("Content-Type", "application/json; charset=utf-8")
	status := http.StatusOK
	if !r.isOK {
		status = http.StatusInternalServerError
	}
	res.WriteHeader(status)
	_ = json.NewEncoder(res).Encode(r.res)
}

func (s *rpcServer) wsHandler(res http.ResponseWriter, req *http.Request) {
	// Authenticate BEFORE parsing request body if authorizers are configured
	authenticated, authenticationResults := s.authenticate(req.Context(), req)
	if !authenticated {
		res.WriteHeader(http.StatusUnauthorized)
		return
	}
	if authenticationResults != nil {
		// Store authentication results in context for use in authorization phase
		req = req.WithContext(context.WithValue(req.Context(), authResultKey, authenticationResults))
	}
	// Now proceed with upgrade (only if auth succeeded or not required)
	conn, err := s.wsUpgrader.Upgrade(res, req, nil)
	if err != nil {
		log.L(req.Context()).Errorf("WebSocket upgrade failed: %s", err)
		return
	}
	s.newWSConnection(conn, req)
}

func (s *rpcServer) authenticate(ctx context.Context, req *http.Request) (bool, []string) {
	if len(s.authorizers) == 0 {
		return true, nil
	}

	// Extract headers for authentication
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value for each header
		}
	}

	authenticationResults := make([]string, len(s.authorizers))
	for i, auth := range s.authorizers {
		authenticationResult, err := auth.Authenticate(ctx, headers)
		if err != nil {
			log.L(ctx).Errorf("HTTP authentication failed at authorizer %d: %s", i, err)
			return false, nil
		}
		authenticationResults[i] = authenticationResult
	}
	return true, authenticationResults
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
