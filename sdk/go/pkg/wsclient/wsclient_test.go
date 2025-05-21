// Copyright © 2025 Kaleido, Inc.
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

package wsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWSClientE2ETLS(t *testing.T) {
	publicKeyFile, privateKeyFile := GenerateTLSCertficates(t)
	defer os.Remove(privateKeyFile.Name())
	defer os.Remove(publicKeyFile.Name())

	toServer, fromServer, url, close, err := NewTestTLSWSServer(func(req *http.Request) {
		assert.Equal(t, "in-beforeConnect", req.Header.Get("added-header"))
		assert.Equal(t, "/test/updated", req.URL.Path)
	}, publicKeyFile, privateKeyFile)
	defer close()
	require.NoError(t, err)

	first := true
	beforeConnect := func(ctx context.Context, w WSClient) error {
		w.SetHeader("added-header", "in-beforeConnect")
		if first {
			first = false
			return fmt.Errorf("first run fails")
		}
		return nil
	}

	afterConnect := func(ctx context.Context, w WSClient) error {
		return w.Send(ctx, []byte(`after connect message`))
	}

	// Init clean config
	wsConfig := &pldconf.WSClientConfig{}

	wsConfig.URL = url + "/test"
	wsConfig.HeartbeatInterval = confutil.P("50ms")
	wsConfig.InitialConnectAttempts = confutil.P(2)
	wsConfig.TLS = pldconf.TLSConfig{
		CAFile:   publicKeyFile.Name(),
		CertFile: publicKeyFile.Name(),
		KeyFile:  privateKeyFile.Name(),
	}

	wsc, err := New(context.Background(), wsConfig, beforeConnect, afterConnect)
	require.NoError(t, err)
	defer wsc.Close()

	//  Change the settings and connect
	wsc.SetURL(wsc.URL() + "/updated")
	err = wsc.Connect()
	require.NoError(t, err)

	// Test server rejects a 2nd connection
	wsc2, err := New(context.Background(), wsConfig, beforeConnect, afterConnect)
	require.NoError(t, err)
	defer wsc2.Close()

	wsc2.SetURL(wsc2.URL() + "/updated")
	err = wsc2.Connect()
	require.Error(t, err)

	// Receive the message automatically sent in afterConnect
	message1 := <-toServer
	assert.Equal(t, `after connect message`, message1)

	// Tell the unit test server to send us a reply, and confirm it
	fromServer <- `some data from server`
	reply := <-wsc.Receive()
	assert.Equal(t, `some data from server`, string(reply))

	// Send some data back
	err = wsc.Send(context.Background(), []byte(`some data to server`))
	assert.NoError(t, err)

	// Check the sevrer got it
	message2 := <-toServer
	assert.Equal(t, `some data to server`, message2)

	// Check heartbeating works
	beforePing := time.Now()
	for wsc.(*wsClient).lastPingCompleted.Before(beforePing) {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWSClientE2E(t *testing.T) {
	toServer, fromServer, url, close := NewTestWSServer(func(req *http.Request) {
		assert.Equal(t, "/test/updated", req.URL.Path)
	})
	defer close()

	first := true
	beforeConnect := func(ctx context.Context, w WSClient) error {
		if first {
			first = false
			return fmt.Errorf("first run fails")
		}
		return nil
	}
	afterConnect := func(ctx context.Context, w WSClient) error {
		return w.Send(ctx, []byte(`after connect message`))
	}

	// Init clean config
	wsConfig := &pldconf.WSClientConfig{}

	wsConfig.URL = url + "/test"
	wsConfig.HeartbeatInterval = confutil.P("50ms")
	wsConfig.InitialConnectAttempts = confutil.P(2)

	wsc, err := New(context.Background(), wsConfig, beforeConnect, afterConnect)
	require.NoError(t, err)
	defer wsc.Close()

	//  Change the settings and connect
	wsc.SetURL(wsc.URL() + "/updated")
	err = wsc.Connect()
	require.NoError(t, err)

	// Receive the message automatically sent in afterConnect
	message1 := <-toServer
	assert.Equal(t, `after connect message`, message1)

	// Tell the unit test server to send us a reply, and confirm it
	fromServer <- `some data from server`
	reply := <-wsc.Receive()
	assert.Equal(t, `some data from server`, string(reply))

	// Send some data back
	err = wsc.Send(context.Background(), []byte(`some data to server`))
	assert.NoError(t, err)

	// Check the sevrer got it
	message2 := <-toServer
	assert.Equal(t, `some data to server`, message2)

	// Check heartbeating works
	beforePing := time.Now()
	for wsc.(*wsClient).lastPingCompleted.Before(beforePing) {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWSClientBadWSURL(t *testing.T) {
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = ":::"

	_, err := New(context.Background(), wsConfig, nil, nil)
	assert.Regexp(t, "PD021100", err)
}

func TestWSClientBadTLSConfig(t *testing.T) {
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = "wss://test"
	wsConfig.TLS = pldconf.TLSConfig{
		CAFile: "badfile",
	}

	_, err := New(context.Background(), wsConfig, nil, nil)
	assert.Regexp(t, "PD020401", err)
}

func TestWSFailStartupHttp500(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(
		func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "custom value", r.Header.Get("Custom-Header"))
			assert.Equal(t, "Basic dXNlcjpwYXNz", r.Header.Get("Authorization"))
			rw.WriteHeader(500)
			_, _ = rw.Write([]byte(`{"error": "pop"}`))
		},
	))
	defer svr.Close()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = fmt.Sprintf("ws://%s", svr.Listener.Addr())
	wsConfig.HTTPHeaders = map[string]interface{}{
		"custom-header": "custom value",
	}
	wsConfig.Auth.Username = "user"
	wsConfig.Auth.Password = "pass"
	wsConfig.ConnectRetry.InitialDelay = confutil.P("1ns")
	wsConfig.InitialConnectAttempts = confutil.P(1)

	w, _ := New(context.Background(), wsConfig, nil, nil)
	err := w.Connect()
	assert.Regexp(t, "PD021103", err)
}

func TestWSFailStartupConnect(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(
		func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(500)
		},
	))
	svr.Close()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = fmt.Sprintf("ws://%s", svr.Listener.Addr())
	wsConfig.ConnectRetry.InitialDelay = confutil.P("1ns")
	wsConfig.InitialConnectAttempts = confutil.P(1)

	w, _ := New(context.Background(), wsConfig, nil, nil)
	err := w.Connect()
	assert.Regexp(t, "PD021103", err)
}

func TestWSSendClosed(t *testing.T) {
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = "ws://test:12345"

	w, err := New(context.Background(), wsConfig, nil, nil)
	assert.NoError(t, err)
	w.Close()

	err = w.Send(context.Background(), []byte(`sent after close`))
	assert.Regexp(t, "PD021102", err)
}

func TestWSSendCanceledContext(t *testing.T) {
	w := &wsClient{
		send:    make(chan []byte),
		closing: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := w.Send(ctx, []byte(`sent after close`))
	assert.Regexp(t, "PD021101", err)
}

func TestWSConnectClosed(t *testing.T) {
	w := &wsClient{
		ctx:    context.Background(),
		closed: true,
	}

	err := w.connect(false)
	assert.Regexp(t, "PD021102", err)
}

func TestWSReadLoopSendFailure(t *testing.T) {
	toServer, fromServer, url, done := NewTestWSServer(nil)
	defer done()

	wsconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	err = wsconn.WriteJSON(map[string]string{"type": "listen", "topic": "topic1"})
	require.NoError(t, err)
	<-toServer
	w := &wsClient{
		ctx:      context.Background(),
		sendDone: make(chan []byte, 1),
		wsconn:   wsconn,
	}

	// Queue a message for the receiver, then immediately close the sender channel
	fromServer <- `some data from server`
	close(w.sendDone)

	// Ensure the readLoop exits immediately
	w.readLoop()

	// Try reconnect, should fail here
	_, _, err = websocket.DefaultDialer.Dial(url, nil)
	assert.Error(t, err)
}

func TestWSReconnectFail(t *testing.T) {
	_, _, url, done := NewTestWSServer(nil)
	defer done()

	wsconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	wsconn.Close()
	ctxCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	w := &wsClient{
		ctx:     ctxCanceled,
		receive: make(chan []byte),
		send:    make(chan []byte),
		closing: make(chan struct{}),
		wsconn:  wsconn,
	}
	close(w.send) // will mean sender exits immediately

	w.receiveReconnectLoop()
}

func TestWSSendFail(t *testing.T) {
	_, _, url, done := NewTestWSServer(nil)
	defer done()

	wsconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	wsconn.Close()
	w := &wsClient{
		ctx:      context.Background(),
		receive:  make(chan []byte),
		send:     make(chan []byte, 1),
		closing:  make(chan struct{}),
		sendDone: make(chan []byte, 1),
		wsconn:   wsconn,
	}
	w.send <- []byte(`wakes sender`)
	w.sendLoop(make(chan struct{}))
	<-w.sendDone
}

func TestWSSendInstructClose(t *testing.T) {
	_, _, url, done := NewTestWSServer(nil)
	defer done()

	wsconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	wsconn.Close()
	w := &wsClient{
		ctx:      context.Background(),
		receive:  make(chan []byte),
		send:     make(chan []byte, 1),
		closing:  make(chan struct{}),
		sendDone: make(chan []byte, 1),
		wsconn:   wsconn,
	}
	receiverClosed := make(chan struct{})
	close(receiverClosed)
	w.sendLoop(receiverClosed)
	<-w.sendDone
}

func TestHeartbeatTimedout(t *testing.T) {
	now := time.Now()
	w := &wsClient{
		ctx:               context.Background(),
		sendDone:          make(chan []byte),
		heartbeatInterval: 1 * time.Microsecond,
		activePingSent:    &now,
	}

	w.sendLoop(make(chan struct{}))
}

func TestHeartbeatSendFailed(t *testing.T) {
	_, _, url, close := NewTestWSServer(func(req *http.Request) {})
	defer close()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url
	wsc, err := New(context.Background(), wsConfig, nil, func(ctx context.Context, w WSClient) error { return nil })
	require.NoError(t, err)
	defer wsc.Close()

	err = wsc.Connect()
	assert.NoError(t, err)

	// Close and use the underlying wsconn to drive a failure to send a heartbeat
	wsc.(*wsClient).wsconn.Close()
	w := &wsClient{
		ctx:               context.Background(),
		sendDone:          make(chan []byte),
		heartbeatInterval: 1 * time.Microsecond,
		wsconn:            wsc.(*wsClient).wsconn,
	}

	w.sendLoop(make(chan struct{}))
}

func TestTestServerFailsSecondConnect(t *testing.T) {
	_, _, url, done := NewTestWSServer(nil)
	defer done()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url

	wsc, err := New(context.Background(), wsConfig, nil, func(ctx context.Context, w WSClient) error { return nil })
	require.NoError(t, err)
	defer wsc.Close()

	err = wsc.Connect()
	require.NoError(t, err)

	wsc2, err := New(context.Background(), wsConfig, nil, func(ctx context.Context, w WSClient) error { return nil })
	require.NoError(t, err)
	defer wsc2.Close()

	err = wsc2.Connect()
	require.Error(t, err)
}

func TestTestTLSServerFailsBadCerts(t *testing.T) {
	filePath := path.Join(os.TempDir(), "badfile")
	err := os.WriteFile(filePath, []byte(`will be deleted`), 0644)
	assert.NoError(t, err)
	closedFile, err := os.Open(filePath)
	assert.NoError(t, err)
	err = closedFile.Close()
	assert.NoError(t, err)
	_, _, _, _, err = NewTestTLSWSServer(nil, closedFile, closedFile)
	assert.Error(t, err)
}

func TestWSClientContextClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = "ws://test"
	wsc, err := New(ctx, wsConfig, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, wsc)
	err = wsc.Send(context.Background(), []byte{})
	assert.Regexp(t, "PD021102", err)
}
