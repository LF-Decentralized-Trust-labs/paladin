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

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketRPCRequestResponse(t *testing.T) {

	ctx, cancelCtx := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelCtx()
	url, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url
	client := rpcclient.WrapWSConfig(wsConfig)
	defer client.Close()
	err := client.Connect(ctx)
	require.NoError(t, err)

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
	url, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	wsConfig := &pldconf.WSClientConfig{}
	wsConfig.URL = url
	client := rpcclient.WrapWSConfig(wsConfig)
	defer client.Close()
	err := client.Connect(context.Background())
	require.NoError(t, err)

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
