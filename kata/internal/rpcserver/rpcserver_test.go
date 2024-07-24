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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/httpserver"
	"github.com/stretchr/testify/assert"
)

func newTestServerHTTP(t *testing.T, conf *Config) (string, *rpcServer, func()) {

	conf.HTTP.Address = confutil.P("127.0.0.1")
	conf.HTTP.Port = confutil.P(0)
	conf.WS.Disabled = true
	s, err := NewServer(context.Background(), conf)
	assert.NoError(t, err)
	err = s.Start()
	assert.NoError(t, err)
	rs := s.(*rpcServer)
	return fmt.Sprintf("http://%s", rs.HTTPAddr()), rs, s.Stop

}

func newTestServerWebSockets(t *testing.T, conf *Config) (string, *rpcServer, func()) {

	conf.WS.Address = confutil.P("127.0.0.1")
	conf.WS.Port = confutil.P(0)
	conf.HTTP.Disabled = true
	s, err := NewServer(context.Background(), conf)
	assert.NoError(t, err)
	err = s.Start()
	assert.NoError(t, err)
	rs := s.(*rpcServer)
	return fmt.Sprintf("ws://%s", rs.WSAddr()), rs, s.Stop

}

func regTestRPC(s *rpcServer, method string, handler RPCHandler) {
	group := strings.SplitN(method, "_", 2)[0]
	module := s.rpcModules[group]
	if module == nil {
		module = NewRPCModule(group)
		s.Register(module)
	}
	module.Add(method, handler)
}

func TestBadHTTPConfig(t *testing.T) {

	_, err := NewServer(context.Background(), &Config{
		HTTP: HTTPEndpointConfig{
			Config: httpserver.Config{
				Address: confutil.P("::::::wrong"),
			},
		},
		WS: WSEndpointConfig{Disabled: true},
	})
	assert.Regexp(t, "PD010801", err)

}

func TestBadWSConfig(t *testing.T) {

	_, err := NewServer(context.Background(), &Config{
		WS: WSEndpointConfig{
			Config: httpserver.Config{
				Address: confutil.P("::::::wrong"),
			},
		},
		HTTP: HTTPEndpointConfig{Disabled: true},
	})
	assert.Regexp(t, "PD010801", err)

}

func TestBadHTTPMethod(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &Config{})
	defer done()

	res, err := http.DefaultClient.Get(url)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)

}

func TestBadWSUpgrade(t *testing.T) {

	_, s, done := newTestServerWebSockets(t, &Config{})
	defer done()

	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s", s.WSAddr()))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

}
