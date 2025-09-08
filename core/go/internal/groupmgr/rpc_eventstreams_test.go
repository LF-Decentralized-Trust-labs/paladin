/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package groupmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/wsclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var nextReq atomic.Uint64

func rpcTestRequest(method string, params ...any) (uint64, []byte) {
	reqID := nextReq.Add(1)
	jsonParams := make([]pldtypes.RawJSON, len(params))
	for i, p := range params {
		jsonParams[i] = pldtypes.JSONString(p)
	}
	req := &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(fmt.Sprintf("%d", reqID)),
		Method:  method,
		Params:  jsonParams,
	}
	return reqID, []byte(pldtypes.JSONString((req)).Pretty())
}

func newTestGroupManagerWithWebSocketRPC(t *testing.T, init ...func(mc *mockComponents, conf *pldconf.GroupManagerConfig)) (context.Context, string, *groupManager, *mockComponents, func()) {
	ctx, gm, mc, gmDone := newTestGroupManager(t, true, &pldconf.GroupManagerConfig{}, init...)

	rpcServer, err := rpcserver.NewRPCServer(ctx, &pldconf.RPCServerConfig{
		HTTP: pldconf.RPCServerConfigHTTP{Disabled: true},
		WS: pldconf.RPCServerConfigWS{
			HTTPServerConfig: pldconf.HTTPServerConfig{
				Port:            confutil.P(0),
				ShutdownTimeout: confutil.P("0"),
			},
		},
	})
	require.NoError(t, err)

	rpcServer.Register(gm.rpcModule)

	err = rpcServer.Start()
	require.NoError(t, err)

	return ctx, fmt.Sprintf("ws://%s", rpcServer.WSAddr()), gm, mc, func() {
		gmDone()
		rpcServer.Stop()
	}

}

func TestRPCEventListenerE2E(t *testing.T) {
	ctx, url, gm, mc, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

	mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
		return rm[0].MessageType.V() == pldapi.RMTPrivacyGroupMessage
	})).Return(nil)

	groupIDs := createTestGroups(t, ctx, mc, gm,
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
	)
	groupID := groupIDs[0]

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener1",
		Options: pldapi.PrivacyGroupMessageListenerOptions{
			ExcludeLocal: false,
		},
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("pgroup_subscribe", "messages", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	// channels for coordination
	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	messages := make(chan *pldapi.PrivacyGroupMessage)
	ackReady := make(chan struct{})          // will be closed once we have subID
	subscriptionReady := make(chan struct{}) // signal when subscription is fully set up

	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	go func() {
		defer close(messages)
		for {
			select {
			case payload, ok := <-wsc.Receive():
				if !ok {
					return
				}

				var rpcPayload *rpcclient.RPCResponse
				err := json.Unmarshal(payload, &rpcPayload)
				require.NoError(t, err)

				if rpcPayload.Error != nil {
					require.NoError(t, rpcPayload.Error)
				}

				if !rpcPayload.ID.IsNil() {
					var rpcID uint64
					err := json.Unmarshal(rpcPayload.ID.Bytes(), &rpcID)
					require.NoError(t, err)

					switch rpcID {
					case subReqID: // Subscribe reply
						subIDStr := rpcPayload.Result.StringValue()
						subID.Store(&subIDStr)
						subIDChan <- subIDStr
						// Signal that subscription is fully ready
						close(subscriptionReady)
					case unSubReqID.Load(): // Unsubscribe reply
						unSubChan <- true
						return
					}
				}

				if rpcPayload.Method == "pgroup_subscription" {
					var batchPayload pldapi.JSONRPCSubscriptionNotification[pldapi.PrivacyGroupMessageBatch]
					err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
					require.NoError(t, err)

					for _, r := range batchPayload.Result.Messages {
						select {
						case messages <- r:
						case <-ctx.Done():
							return
						}
					}
					// wait until main test has stored subID and closed ackReady
					<-ackReady
					if storedSubID := subID.Load(); storedSubID != nil {
						_, req := rpcTestRequest("pgroup_ack", *storedSubID)
						require.NoError(t, wsc.Send(ctx, req))
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for subscription with timeout
	var subIDStr string
	select {
	case subIDStr = <-subIDChan:
		_, err = uuid.Parse(subIDStr)
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Subscription timeout")
	}

	// Wait for subscription to be fully ready before sending messages
	select {
	case <-subscriptionReady:
		// Subscription is ready
	case <-time.After(5 * time.Second):
		t.Fatal("Subscription ready timeout")
	}

	testMsgs := make([]*pldapi.PrivacyGroupMessageInput, 6)
	testMsgIDs := make([]uuid.UUID, len(testMsgs))
	for i := range testMsgs {
		testMsgs[i] = &pldapi.PrivacyGroupMessageInput{
			Domain: "domain1",
			Group:  groupID,
			Data:   pldtypes.JSONString(fmt.Sprintf("message %d", i)),
			Topic:  "my/topic",
		}
	}

	// Send first 3
	err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for i := range 3 {
			msgID, err := gm.SendMessage(ctx, dbTX, testMsgs[i])
			require.NoError(t, err)
			testMsgIDs[i] = *msgID
		}
		return nil
	})
	require.NoError(t, err)

	close(ackReady)

	// Wait for first 3 messages to be processed
	receivedMsgs := 0
	timeout := time.After(10 * time.Second)
	for receivedMsgs < 3 {
		select {
		case msg := <-messages:
			if msg != nil {
				require.Contains(t, testMsgIDs, msg.ID)
				receivedMsgs++
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for messages, received %d of 3", receivedMsgs)
		}
	}

	// Send remaining 3 messages
	err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		for i := 3; i < len(testMsgs); i++ {
			msgID, err := gm.SendMessage(ctx, dbTX, testMsgs[i])
			require.NoError(t, err)
			testMsgIDs[i] = *msgID
		}
		return nil
	})
	require.NoError(t, err)

	// Wait for next all messages
	timeout = time.After(10 * time.Second)
	for receivedMsgs < len(testMsgs) {
		select {
		case msg := <-messages:
			if msg != nil {
				require.Contains(t, testMsgIDs, msg.ID)
				receivedMsgs++
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for messages, received %d of %d", receivedMsgs, len(testMsgs))
		}
	}

	// Unsubscribe
	reqID, req := rpcTestRequest("pgroup_unsubscribe", subIDStr)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	select {
	case <-unSubChan:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Unsubscribe timeout")
	}
}

func TestRPCEventListenerE2ENack(t *testing.T) {
	ctx, url, gm, mc, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	mc.registryManager.On("GetNodeTransports", mock.Anything, "node2").
		Return([]*components.RegistryNodeTransportEntry{ /* contents not checked */ }, nil)

	mc.transportManager.On("SendReliable", mock.Anything, mock.Anything, mock.MatchedBy(func(rm []*pldapi.ReliableMessage) bool {
		return rm[0].MessageType.V() == pldapi.RMTPrivacyGroupMessage
	})).Return(nil)

	groupIDs := createTestGroups(t, ctx, mc, gm,
		&pldapi.PrivacyGroupInput{
			Domain:  "domain1",
			Members: []string{"me@node1", "you@node2"},
		},
	)
	groupID := groupIDs[0]

	err := gm.CreateMessageListener(ctx, &pldapi.PrivacyGroupMessageListener{
		Name: "listener1",
		Options: pldapi.PrivacyGroupMessageListenerOptions{
			ExcludeLocal: false,
		},
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("pgroup_subscribe", "messages", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	// Channels for coordination
	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	subscriptionReady := make(chan struct{}) // Signal when subscription is fully set up
	sentNack := false
	messages := make(chan *pldapi.PrivacyGroupMessage)
	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	// Reader goroutine: handles subscribe‐reply, subscription notifications,
	// sends exactly one NACK then one ACK, and forwards redelivered message.
	go func() {
		defer close(messages)
		for {
			select {
			case payload, ok := <-wsc.Receive():
				if !ok {
					return
				}

				var rpcPayload *rpcclient.RPCResponse
				err := json.Unmarshal(payload, &rpcPayload)
				require.NoError(t, err)

				if rpcPayload.Error != nil {
					require.NoError(t, rpcPayload.Error)
				}

				if !rpcPayload.ID.IsNil() {
					var rpcID uint64
					err := json.Unmarshal(rpcPayload.ID.Bytes(), &rpcID)
					require.NoError(t, err)

					switch rpcID {
					case subReqID: // Subscribe reply
						subIDStr := rpcPayload.Result.StringValue()
						_, err = uuid.Parse(subIDStr)
						require.NoError(t, err)
						subID.Store(&subIDStr)
						subIDChan <- subIDStr
						// Signal that subscription is fully ready
						close(subscriptionReady)
					case unSubReqID.Load(): // Unsubscribe reply
						unSubChan <- true
					}
				}

				if rpcPayload.Method == "pgroup_subscription" {
					var batchPayload pldapi.JSONRPCSubscriptionNotification[pldapi.PrivacyGroupMessageBatch]
					err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
					require.NoError(t, err)

					if !sentNack {
						// send nack first
						if storedSubID := subID.Load(); storedSubID != nil {
							_, req := rpcTestRequest("pgroup_nack", *storedSubID)
							require.NoError(t, wsc.Send(ctx, req))
							sentNack = true
						}
					} else {
						// then ack and forward messages
						for _, r := range batchPayload.Result.Messages {
							select {
							case messages <- r:
							case <-ctx.Done():
								return
							}
						}
						if storedSubID := subID.Load(); storedSubID != nil {
							_, req := rpcTestRequest("pgroup_ack", *storedSubID)
							require.NoError(t, wsc.Send(ctx, req))
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for subscription reply to ensure everything is set up
	var sid string
	select {
	case sid = <-subIDChan:
		require.NotEmpty(t, sid)
	case <-time.After(5 * time.Second):
		t.Fatal("Subscription timeout")
	}

	// Wait for subscription to be fully ready before sending message
	select {
	case <-subscriptionReady:
		// Subscription is ready
	case <-time.After(5 * time.Second):
		t.Fatal("Subscription ready timeout")
	}

	// Now send the message after subscription is confirmed and ready
	var sentMsgID uuid.UUID
	err = gm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		id, err := gm.SendMessage(ctx, dbTX, &pldapi.PrivacyGroupMessageInput{
			Domain: "domain1",
			Group:  groupID,
			Data:   pldtypes.JSONString("some data"),
			Topic:  "my/topic",
		})
		if err == nil {
			sentMsgID = *id
		}
		return err
	})
	require.NoError(t, err)

	// The reader goroutine will first NACK and then, upon redelivery, push into `messages`
	select {
	case redelivered := <-messages:
		require.Equal(t, sentMsgID, redelivered.ID)
	case <-time.After(15 * time.Second):
		t.Fatal("Did not receive redelivered message")
	}

	// Finally unsubscribe
	reqID, req := rpcTestRequest("pgroup_unsubscribe", sid)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	select {
	case <-unSubChan:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatalf("Unsubscribe timeout - connection may have been closed")
	}
}

func TestRPCSubscribeNoType(t *testing.T) {
	ctx, url, _, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("pgroup_subscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD020003", rpcPayload.Error.Error())

}

func TestRPCSubscribeNoListener(t *testing.T) {
	ctx, url, _, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("pgroup_subscribe", "messages")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012519", rpcPayload.Error.Error())

}

func TestRPCSubscribeBadListener(t *testing.T) {
	ctx, url, _, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("pgroup_subscribe", "messages", "unknown")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012508", rpcPayload.Error.Error())

}

func TestUnsubscribeNoSubscriptionID(t *testing.T) {
	ctx, url, _, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("pgroup_unsubscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012518", rpcPayload.Error.Error())

}

func TestHandleLifecycleUnknown(t *testing.T) {
	ctx, _, gm, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	res := gm.rpcEventStreams.HandleLifecycle(ctx, &rpcclient.RPCRequest{
		Method: "wrong",
		Params: []pldtypes.RawJSON{pldtypes.RawJSON(`"any"`)},
	})
	require.Regexp(t, "PD012517", res.Error.Error())

}

type mockRPCAsyncControl struct{}

func (ac *mockRPCAsyncControl) ID() string                     { return "sub1" }
func (ac *mockRPCAsyncControl) Closed()                        {}
func (ac *mockRPCAsyncControl) Send(method string, params any) {}

func TestHandleLifecycleNoBlockNack(t *testing.T) {
	ctx, _, gm, _, done := newTestGroupManagerWithWebSocketRPC(t)
	defer done()

	ctrl := &mockRPCAsyncControl{}
	es := gm.rpcEventStreams
	es.receiptSubs["sub1"] = &receiptListenerSubscription{
		es:        es,
		ctrl:      ctrl,
		acksNacks: make(chan *rpcAckNack),
		closed:    make(chan struct{}),
	}

	res := es.HandleLifecycle(ctx, &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON("12345"),
		Method:  "pgroup_nack",
		Params:  []pldtypes.RawJSON{pldtypes.RawJSON(`"sub1"`)},
	})
	require.Nil(t, res)

	es.getSubscription("sub1").ConnectionClosed()
	require.Empty(t, es.receiptSubs)

}
