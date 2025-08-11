// Copyright © 2022 Kaleido, Inc.
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
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, conf *pldconf.HTTPServerConfig, handler http.HandlerFunc) (string, *httpServer, func()) {

	conf.Address = confutil.P("127.0.0.1")
	conf.Port = confutil.P(0)
	s, err := NewServer(context.Background(), "unittest", conf, handler)
	require.NoError(t, err)
	hs := s.(*httpServer)
	err = s.Start()
	require.NoError(t, err)

	return fmt.Sprintf("http://%s", s.Addr()), hs, s.Stop

}

func TestMissingPort(t *testing.T) {
	_, err := NewServer(context.Background(), "unittest", &pldconf.HTTPServerConfig{}, nil)
	assert.Regexp(t, "PD020601", err)
}

func TestBadTLSConfig(t *testing.T) {
	_, err := NewServer(context.Background(), "unittest", &pldconf.HTTPServerConfig{
		Port: confutil.P(0),
		TLS: pldconf.TLSConfig{
			Enabled: true,
			CAFile:  "!!!!!badness",
		},
	}, nil)
	assert.Regexp(t, "PD020401", err)
}

func TestBadAddress(t *testing.T) {

	_, err := NewServer(context.Background(), "unittest", &pldconf.HTTPServerConfig{
		Port:    confutil.P(0),
		Address: confutil.P(":::::badness"),
	}, nil)
	assert.Regexp(t, "PD020600", err)

}

func TestServeOK(t *testing.T) {
	url, _, done := newTestServer(t, &pldconf.HTTPServerConfig{}, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Method, http.MethodPut)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, err := w.Write(([]byte)(`{"some":"data"}`))
		require.NoError(t, err)
	})
	defer done()

	req, err := http.NewRequest(http.MethodPut, url, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"some":"data"}`, (string)(data))
}

func TestForceShutdown(t *testing.T) {
	requestStarted := make(chan struct{})
	url, _, done := newTestServer(t, &pldconf.HTTPServerConfig{
		ShutdownTimeout: confutil.P("1ns"),
	}, func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	})

	req, err := http.NewRequest(http.MethodPut, url, nil)
	require.NoError(t, err)

	returned := make(chan error)
	go func() {
		_, err := http.DefaultClient.Do(req)
		returned <- err
	}()
	<-requestStarted

	done()
	assert.Regexp(t, "EOF", <-returned)
}

func TestServeCustomTimeout(t *testing.T) {
	url, _, done := newTestServer(t, &pldconf.HTTPServerConfig{}, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusRequestTimeout)
	})
	defer done()

	req, err := http.NewRequest(http.MethodPut, url, nil)
	require.NoError(t, err)
	req.Header.Set("Request-Timeout", "1ns")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusRequestTimeout, res.StatusCode)
}

func TestParseCustomTimeout(t *testing.T) {
	url, s, done := newTestServer(t, &pldconf.HTTPServerConfig{}, func(w http.ResponseWriter, r *http.Request) {})
	defer done()

	req, err := http.NewRequest(http.MethodPut, url, nil)
	require.NoError(t, err)

	req.Header.Set("Request-Timeout", "1")
	assert.Equal(t, 1*time.Second, s.calcRequestTimeout(req, 10*time.Second, 20*time.Second))

	req.Header.Set("Request-Timeout", "1ms")
	assert.Equal(t, 1*time.Millisecond, s.calcRequestTimeout(req, 10*time.Second, 20*time.Second))

	req.Header.Set("Request-Timeout", "30")
	assert.Equal(t, 20*time.Second, s.calcRequestTimeout(req, 10*time.Second, 20*time.Second))

	req.Header.Set("Request-Timeout", "wrongness")
	assert.Equal(t, 10*time.Second, s.calcRequestTimeout(req, 10*time.Second, 20*time.Second))
}

func TestOnlyAcceptsTLSConnectionsWhenTLSConfigProvided(t *testing.T) {
	tlsConfig := pldconf.TLSConfig{
		Enabled: true,
	}
	config := &pldconf.HTTPServerConfig{
		TLS: tlsConfig,
	}

	url, _, done := newTestServer(t, config, func(w http.ResponseWriter, r *http.Request) {})
	defer done()

	// Make a HTTP request to the server and check that we get a 400 (client speaking HTTP to a HTTPS server)
	req, err := http.NewRequest(http.MethodPut, url, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, 400, res.StatusCode)
}

type mockResponseWriter struct{}

func (*mockResponseWriter) Header() http.Header { return nil }

func (*mockResponseWriter) Write(data []byte) (int, error) { return -1, nil }

func (*mockResponseWriter) WriteHeader(statusCode int) {}

func TestLogCaptureBadWSResponseWriter(t *testing.T) {
	_, _, err := (&logCapture{res: &mockResponseWriter{}}).Hijack()
	assert.Regexp(t, "PD020602", err)
}

func TestWSUpgradeSupported(t *testing.T) {
	url, _, done := newTestServer(t, &pldconf.HTTPServerConfig{}, func(w http.ResponseWriter, r *http.Request) {
		wsUpgrader := websocket.Upgrader{}
		_, err := wsUpgrader.Upgrade(w, r, r.Header)
		require.NoError(t, err)
	})
	defer done()

	c, _, err := websocket.DefaultDialer.Dial(strings.Replace(url, "http", "ws", 1), http.Header{})
	require.NoError(t, err)
	_ = c.Close()
}
