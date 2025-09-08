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

package httpserver

import (
	"context"
	"net/http/pprof"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/gorilla/mux"
)

type DebugServer interface {
	Server
	Router() *mux.Router
}

type debugServer struct {
	Server
	r *mux.Router
}

func (ds *debugServer) Router() *mux.Router {
	return ds.r
}

func NewDebugServer(ctx context.Context, debugServerConf *pldconf.HTTPServerConfig) (_ DebugServer, err error) {
	r := mux.NewRouter()
	r.PathPrefix("/debug/pprof/cmdline").HandlerFunc(pprof.Cmdline)
	r.PathPrefix("/debug/pprof/profile").HandlerFunc(pprof.Profile)
	r.PathPrefix("/debug/pprof/symbol").HandlerFunc(pprof.Symbol)
	r.PathPrefix("/debug/pprof/trace").HandlerFunc(pprof.Trace)
	r.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	server, err := NewServer(ctx, "debug", debugServerConf, r)
	if err != nil {
		return nil, err
	}
	log.L(ctx).Infof("Debug server running on %s", server.Addr())
	return &debugServer{Server: server, r: r}, nil
}
