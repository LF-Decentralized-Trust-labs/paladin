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
	"net/http"

	"github.com/gorilla/mux"
)

type Server interface {
	Start() error
	Stop()
	WaitStop() error
}

func NewServer(ctx context.Context) (ss Server, err error) {

	s := &rpcServer{
		httpServerDone: make(chan error),
	}
	s.ctx, s.cancelCtx = context.WithCancel(ctx)

	return s, err
}

type rpcServer struct {
	ctx            context.Context
	cancelCtx      func()
	httpServerDone chan error
	started        bool
}

func (s *rpcServer) router() *mux.Router {
	mux := mux.NewRouter()
	mux.Path("/").Methods(http.MethodPost).Handler(http.HandlerFunc(s.rpcHandler))
	return mux
}

func (s *rpcServer) runAPIServer() {
	// TODO
}

func (s *rpcServer) Start() error {
	go s.runAPIServer()
	s.started = true
	return nil
}

func (s *rpcServer) Stop() {
	s.cancelCtx()
}

func (s *rpcServer) WaitStop() (err error) {
	if s.started {
		s.started = false
		err = <-s.httpServerDone
	}
	return err
}
