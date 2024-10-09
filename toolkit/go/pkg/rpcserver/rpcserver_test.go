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

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServerHTTP(t *testing.T, conf *pldconf.RPCServerConfig) (string, *rpcServer, func()) {

	conf.HTTP.Address = confutil.P("127.0.0.1")
	conf.HTTP.Port = confutil.P(0)
	conf.WS.Disabled = true
	s, err := NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)
	rs := s.(*rpcServer)
	return fmt.Sprintf("http://%s", rs.HTTPAddr()), rs, s.Stop

}

func newTestServerWebSockets(t *testing.T, conf *pldconf.RPCServerConfig) (string, *rpcServer, func()) {

	conf.WS.Address = confutil.P("127.0.0.1")
	conf.WS.Port = confutil.P(0)
	conf.HTTP.Disabled = true
	s, err := NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = s.Start()
	require.NoError(t, err)
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

	_, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("::::::wrong"),
			},
		},
		WS: pldconf.RPCServerConfigWS{Disabled: true},
	})
	assert.Regexp(t, "PD020601", err)

}

func TestBadWSConfig(t *testing.T) {

	_, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Address: confutil.P("::::::wrong"),
			},
		},
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
	})
	assert.Regexp(t, "PD020601", err)

}

func TestBadHTTPMethod(t *testing.T) {

	url, _, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	res, err := http.DefaultClient.Get(url)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)

}

func TestBadWSUpgrade(t *testing.T) {

	_, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s", s.WSAddr()))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

}

func TestWSHandler(t *testing.T) {
	rpcServer, err := NewRPCServer(context.Background(), &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port: confutil.P(0),
			},
		},
	})
	require.NoError(t, err)
	defer rpcServer.Stop()

	assert.NotNil(t, rpcServer.WSHandler)
}
