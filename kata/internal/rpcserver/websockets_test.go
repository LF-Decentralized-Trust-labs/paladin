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
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketRPCRequestResponse(t *testing.T) {

	ctx, cancelCtx := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelCtx()
	url, s, done := newTestServerWebSockets(t, &Config{})
	defer done()

	client := rpcbackend.NewWSRPCClient(&wsclient.WSConfig{WebSocketURL: url, DisableReconnect: true})
	defer client.Close()
	err := client.Connect(ctx)
	assert.NoError(t, err)

	regTestRPC(s, "stringy_method", RPCMethod2(func(ctx context.Context, p0, p1 string) (string, error) {
		assert.Equal(t, "v0", p0)
		assert.Equal(t, "v1", p1)
		return "result", nil
	}))

	var result string
	rpcErr := client.CallRPC(ctx, &result, "stringy_method", "v0", "v1")
	assert.Nil(t, rpcErr)
	assert.Equal(t, "result", result)

}

func TestWebSocketConnectionFailureHandling(t *testing.T) {
	url, s, done := newTestServerWebSockets(t, &Config{})
	defer done()

	client := rpcbackend.NewWSRPCClient(&wsclient.WSConfig{WebSocketURL: url, DisableReconnect: true})
	defer client.Close()
	err := client.Connect(context.Background())
	assert.NoError(t, err)

	var wsConn *webSocketConnection
	before := time.Now()
	for wsConn == nil {
		time.Sleep(1 * time.Millisecond)
		for _, wsConn = range s.wsConnections {
		}
		if time.Since(before) > 1*time.Second {
			panic("timed out waiting for connection")
		}
	}

	// Close the connection
	client.Close()
	<-wsConn.closing
	for !wsConn.closed {
		time.Sleep(1 * time.Microsecond)
	}

	// Run the send directly to give it an error to handle, which will make it return
	wsConn.closing = make(chan struct{})
	wsConn.send = make(chan []byte)
	go func() { wsConn.send <- ([]byte)(`{}`) }()
	wsConn.sender()

	// Give it some bad data to handle
	wsConn.sendMessage(map[bool]bool{false: true})

	// Give it some good data to discard
	wsConn.sendMessage("anything")

}

func TestWebSocketEthSubscribeUnsubscribe(t *testing.T) {
	url, s, done := newTestServerWebSockets(t, &Config{})
	defer done()

	client := rpcbackend.NewWSRPCClient(&wsclient.WSConfig{WebSocketURL: url, DisableReconnect: true})
	defer client.Close()
	err := client.Connect(context.Background())
	assert.NoError(t, err)

	var wsConn *webSocketConnection
	before := time.Now()
	for wsConn == nil {
		time.Sleep(1 * time.Millisecond)
		for _, wsConn = range s.wsConnections {
		}
		if time.Since(before) > 1*time.Second {
			panic("timed out waiting for connection")
		}
	}

	rpcErr := client.CallRPC(context.Background(), &types.RawJSON{}, "eth_subscribe")
	assert.Regexp(t, "PD011004", rpcErr.Message)
	rpcErr = client.CallRPC(context.Background(), &types.RawJSON{}, "eth_unsubscribe")
	assert.Regexp(t, "PD011004", rpcErr.Message)

	sub1, rpcErr := client.Subscribe(context.Background(), "myEvents", map[string]interface{}{"extra": "params"})
	assert.Nil(t, rpcErr)

	_, rpcErr = client.Subscribe(context.Background(), "otherEvents")
	assert.Nil(t, rpcErr)

	assert.Len(t, wsConn.subscriptions, 2)
	assert.JSONEq(t, `{"extra": "params"}`, wsConn.subscriptions[0].params.String())

	go s.EthPublish("myEvents", map[string]interface{}{"some": "thing"})

	notification := <-sub1.Notifications()
	assert.NotNil(t, notification)
	assert.JSONEq(t, `{"some": "thing"}`, notification.Result.String())

	rpcErr = sub1.Unsubscribe(context.Background())
	assert.Nil(t, rpcErr)

	assert.Len(t, wsConn.subscriptions, 1)
	assert.Equal(t, "otherEvents", wsConn.subscriptions[0].eventType)

	// Close the connection
	client.Close()
	<-wsConn.closing
	for !wsConn.closed {
		time.Sleep(1 * time.Microsecond)
	}

}
