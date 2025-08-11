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
	"net/http/httptest"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockServer implements the Server interface for testing
type MockServer struct {
	mock.Mock
}

func (m *MockServer) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockServer) Stop() {
	m.Called()
}

func (m *MockServer) Addr() net.Addr {
	args := m.Called()
	return args.Get(0).(net.Addr)
}

func TestNewRouter(t *testing.T) {
	ctx := context.Background()
	conf := &pldconf.HTTPServerConfig{Address: confutil.P("127.0.0.1"), Port: confutil.P(0)}

	router, err := NewRouter(ctx, "test server", conf)
	assert.NoError(t, err)
	assert.NotNil(t, router)
	assert.NotNil(t, router.server)
}

func TestRouterHandleFunc(t *testing.T) {
	r := &router{router: mux.NewRouter()}
	handlerCalled := false
	r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.router.ServeHTTP(w, req)

	assert.True(t, handlerCalled)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestRouterPathPrefixHandleFunc(t *testing.T) {
	r := &router{router: mux.NewRouter()}
	handlerCalled := false
	r.PathPrefixHandleFunc("/prefix", func(w http.ResponseWriter, req *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/prefix/test", nil)
	w := httptest.NewRecorder()
	r.router.ServeHTTP(w, req)

	assert.True(t, handlerCalled)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestRouterStart(t *testing.T) {
	ctx := context.Background()

	mockServer := new(MockServer)
	mockServer.On("Start").Return(nil)
	router := &router{
		ctx:    ctx,
		server: mockServer,
	}

	err := router.Start()
	assert.NoError(t, err)
	mockServer.AssertCalled(t, "Start")
}

func TestRouterStartNil(t *testing.T) {
	ctx := context.Background()
	router := &router{
		ctx: ctx,
	}

	err := router.Start()
	assert.NoError(t, err)
}

func TestRouterStop(t *testing.T) {
	mockServer := new(MockServer)
	mockServer.On("Stop").Return()

	router := &router{server: mockServer}
	router.Stop()
	mockServer.AssertCalled(t, "Stop")
}

func TestRouterAddr(t *testing.T) {
	expectedAddr := &net.IPAddr{IP: net.ParseIP("127.0.0.1")}
	mockServer := new(MockServer)
	mockServer.On("Addr").Return(expectedAddr)

	router := &router{server: mockServer}
	addr := router.Addr()

	assert.Equal(t, expectedAddr, addr)
	mockServer.AssertCalled(t, "Addr")
}
