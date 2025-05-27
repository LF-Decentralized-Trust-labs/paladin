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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reference implementation of the async interface for the simple fire-and-forget publication
// interface described in https://geth.ethereum.org/docs/interacting-with-geth/rpc/pubsub

type EthSubscribe struct {
	subLock         sync.Mutex
	subsByEventType map[string]map[string]*ethSubscription
}

func NewEthSubscribe() *EthSubscribe {
	es := &EthSubscribe{
		subsByEventType: make(map[string]map[string]*ethSubscription),
	}
	return es
}

func (es *EthSubscribe) RPCAsyncHandler() RPCAsyncHandler {
	return es
}

func (es *EthSubscribe) Publish(eventType string, result any) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	for _, sub := range es.subsByEventType[eventType] {
		sub.ctrl.Send("eth_subscription", map[string]any{
			"subscription": sub.ctrl.ID(),
			"result":       result,
		})
	}
}

func (es *EthSubscribe) StartMethod() string {
	return "eth_subscribe"
}

func (es *EthSubscribe) LifecycleMethods() []string {
	return []string{"eth_unsubscribe"}
}

type ethSubscription struct {
	es        *EthSubscribe
	ctrl      RPCAsyncControl
	eventType string
	params    []pldtypes.RawJSON
}

func (es *EthSubscribe) HandleStart(ctx context.Context, req *rpcclient.RPCRequest, ctrl RPCAsyncControl) (RPCAsyncInstance, *rpcclient.RPCResponse) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	if len(req.Params) < 1 {
		return nil, rpcclient.NewRPCErrorResponse(fmt.Errorf("eth_subscribe requires a type parameter"), req.ID, rpcclient.RPCCodeInvalidRequest)
	}
	eventType := req.Params[0].StringValue() // additional validation recommended here
	subMap := es.subsByEventType[eventType]
	if subMap == nil {
		subMap = make(map[string]*ethSubscription)
		es.subsByEventType[eventType] = subMap
	}
	sub := &ethSubscription{
		es:        es,
		ctrl:      ctrl,
		eventType: eventType,
		params:    req.Params[1:],
	}
	subMap[ctrl.ID()] = sub
	return sub, &rpcclient.RPCResponse{
		JSONRpc: "2.0",
		ID:      req.ID,
		Result:  pldtypes.JSONString(ctrl.ID()),
	}
}

func (es *EthSubscribe) popSubForUnsubscribe(subID string) *ethSubscription {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	for _, forType := range es.subsByEventType {
		sub := forType[subID]
		if sub != nil {
			delete(forType, subID)
			return sub
		}
	}

	return nil
}

func (es *EthSubscribe) HandleLifecycle(ctx context.Context, req *rpcclient.RPCRequest) *rpcclient.RPCResponse {

	if req.Method != "eth_unsubscribe" {
		return rpcclient.NewRPCErrorResponse(fmt.Errorf("method %s unknown", req.Method), req.ID, rpcclient.RPCCodeInvalidRequest)
	}

	if len(req.Params) != 1 {
		return rpcclient.NewRPCErrorResponse(fmt.Errorf("eth_unsubscribe requires single parameter"), req.ID, rpcclient.RPCCodeInvalidRequest)
	}
	sub := es.popSubForUnsubscribe(req.Params[0].StringValue())
	if sub != nil {
		sub.ctrl.Closed()
	}
	return &rpcclient.RPCResponse{
		JSONRpc: "2.0",
		ID:      req.ID,
		Result:  pldtypes.JSONString(sub != nil),
	}

}

func (sub *ethSubscription) ConnectionClosed() {
	sub.es.cleanupSub(sub)
}

func (es *EthSubscribe) cleanupSub(sub *ethSubscription) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	subMap := es.subsByEventType[sub.eventType]
	if subMap != nil {
		delete(subMap, sub.ctrl.ID())
	}
}

func TestWebSocketEthSubscribeUnsubscribe(t *testing.T) {
	url, s, done := newTestServerWebSockets(t, &pldconf.RPCServerConfig{})
	defer done()

	ethSubs := NewEthSubscribe()
	s.Register(NewRPCModule("eth").AddAsync(ethSubs.RPCAsyncHandler()))

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

	rpcErr := client.CallRPC(context.Background(), &pldtypes.RawJSON{}, "eth_subscribe")
	assert.Regexp(t, "eth_subscribe requires a type parameter", rpcErr)
	rpcErr = client.CallRPC(context.Background(), &pldtypes.RawJSON{}, "eth_unsubscribe")
	assert.Regexp(t, "eth_unsubscribe requires single parameter", rpcErr)

	sub1, rpcErr := client.Subscribe(context.Background(), rpcclient.EthSubscribeConfig(), "myEvents", map[string]interface{}{"extra": "params"})
	assert.Nil(t, rpcErr)

	_, rpcErr = client.Subscribe(context.Background(), rpcclient.EthSubscribeConfig(), "otherEvents")
	assert.Nil(t, rpcErr)

	assert.Len(t, ethSubs.subsByEventType["myEvents"], 1)
	assert.Len(t, ethSubs.subsByEventType["otherEvents"], 1)
	for _, sub := range ethSubs.subsByEventType["myEvents"] {
		assert.JSONEq(t, `{"extra": "params"}`, sub.params[0].String())
	}

	go ethSubs.Publish("myEvents", map[string]interface{}{"some": "thing"})

	notification := <-sub1.Notifications()
	assert.NotNil(t, notification)
	assert.JSONEq(t, `{"some": "thing"}`, notification.GetResult().String())

	rpcErr = sub1.Unsubscribe(context.Background())
	assert.Nil(t, rpcErr)

	assert.Len(t, ethSubs.subsByEventType["otherEvents"], 1)

	// Close the connection
	client.Close()
	<-wsConn.closing
	for !wsConn.closed {
		time.Sleep(1 * time.Microsecond)
	}

}

func TestEthSubscribeNonWS(t *testing.T) {
	url, s, done := newTestServerHTTP(t, &pldconf.RPCServerConfig{})
	defer done()

	ethSubs := NewEthSubscribe()
	s.Register(NewRPCModule("eth").AddAsync(ethSubs.RPCAsyncHandler()))

	client := rpcclient.WrapRestyClient(resty.New().SetBaseURL(url))

	var res any
	rpcErr := client.CallRPC(context.Background(), &res, "eth_subscribe")
	assert.Regexp(t, "PD020706", rpcErr)

}
