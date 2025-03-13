// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package router

import (
	"context"
	"net"
	"net/http"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/httpserver"
	"github.com/gorilla/mux"
)

type Router interface {
	Start() error
	Stop()
	Addr() net.Addr

	HandleFunc(path string, f func(http.ResponseWriter, *http.Request))
	PathPrefixHandleFunc(path string, f func(http.ResponseWriter, *http.Request))
}

func NewRouter(ctx context.Context, description string, conf *pldconf.HTTPServerConfig) (_ *router, err error) {
	r := &router{
		ctx:    ctx,
		router: mux.NewRouter(),
	}

	r.server, err = httpserver.NewServer(ctx, description, conf, r.router)
	return r, err
}

// rpcServer implements the RPCServer interface
var _ Router = &router{}

type router struct {
	ctx    context.Context
	router *mux.Router
	server httpserver.Server
}

func (r *router) HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) {
	r.router.HandleFunc(path, f)
}

func (r *router) PathPrefixHandleFunc(path string, f func(http.ResponseWriter, *http.Request)) {
	r.router.PathPrefix(path).HandlerFunc(f)
}

func (r *router) Addr() (a net.Addr) {
	if r.server != nil {
		a = r.server.Addr()
	}
	return a
}

func (r *router) Start() (err error) {
	if r.server != nil {
		return r.server.Start()
	}
	return nil
}

func (r *router) Stop() {
	if r.server != nil {
		r.server.Stop()
	}
}
