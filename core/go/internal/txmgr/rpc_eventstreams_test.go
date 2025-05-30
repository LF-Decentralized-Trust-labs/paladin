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

package txmgr

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/wsclient"
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

func TestRPCReceiptListenerE2E(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)

	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("ptx_subscribe", "receipts", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	ackReady := make(chan bool)
	receipts := make(chan *pldapi.TransactionReceiptFull)
	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	go func() {
		for payload := range wsc.Receive() {
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
					subIDChan <- rpcPayload.Result.StringValue()
				case unSubReqID.Load(): // Unsubscribe reply
					unSubChan <- true
				}
			}

			if rpcPayload.Method == "ptx_subscription" {
				var batchPayload pldapi.JSONRPCSubscriptionNotification[pldapi.TransactionReceiptBatch]
				err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
				require.NoError(t, err)

				for _, r := range batchPayload.Result.Receipts {
					receipts <- r
				}
				<-ackReady // signal that we are ready to ack

				_, req := rpcTestRequest("ptx_ack", *subID.Load())
				err = wsc.Send(ctx, req)
				require.NoError(t, err)
			}

		}
	}()

	txs := make([]*components.ReceiptInput, 6)
	for i := 0; i < len(txs); i++ {
		txs[i] = &components.ReceiptInput{
			ReceiptType:   components.RT_Success,
			TransactionID: uuid.New(),
			OnChain:       randOnChain(pldtypes.RandAddress()),
		}
	}

	// Send first 3
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, txs[0:3])
	})
	require.NoError(t, err)

	subIDStr := <-subIDChan
	_, err = uuid.Parse(subIDStr)
	require.NoError(t, err)
	subID.Store(&subIDStr)
	close(ackReady) // close ackReady to signal that we are ready to receive receipts

	for i := 0; i < 3; i++ {
		require.Equal(t, txs[i].TransactionID, (<-receipts).ID)
	}

	// Send rest
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, txs[3:])
	})
	require.NoError(t, err)

	for i := 3; i < len(txs); i++ {
		require.Equal(t, txs[i].TransactionID, (<-receipts).ID)
	}

	reqID, req := rpcTestRequest("ptx_unsubscribe", subIDStr)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)
	<-unSubChan

}

func TestRPCReceiptListenerE2ENack(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("ptx_subscribe", "receipts", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	ackReady := make(chan bool)
	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	sentNack := false
	receipts := make(chan *pldapi.TransactionReceiptFull)
	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	go func() {
		for payload := range wsc.Receive() {
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
					subIDChan <- rpcPayload.Result.StringValue()
				case unSubReqID.Load(): // Unsubscribe reply
					unSubChan <- true
				}
			}

			if rpcPayload.Method == "ptx_subscription" {
				var batchPayload pldapi.JSONRPCSubscriptionNotification[pldapi.TransactionReceiptBatch]
				err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
				require.NoError(t, err)

				<-ackReady // wait for ackReady to be closed before processing receipts
				if !sentNack {
					// send nack first
					_, req := rpcTestRequest("ptx_nack", *subID.Load())
					err = wsc.Send(ctx, req)
					require.NoError(t, err)
					sentNack = true
				} else {
					// then ack
					for _, r := range batchPayload.Result.Receipts {
						receipts <- r
					}
					_, req := rpcTestRequest("ptx_ack", *subID.Load())
					err = wsc.Send(ctx, req)
					require.NoError(t, err)

				}
			}

		}
	}()

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return txm.FinalizeTransactions(ctx, dbTX, []*components.ReceiptInput{
			{
				ReceiptType:   components.RT_Success,
				TransactionID: uuid.New(),
				OnChain:       randOnChain(pldtypes.RandAddress()),
			},
		})
	})
	require.NoError(t, err)

	// Wait for subscription reply, then trigger reader’s NACK logic
	subIDStr := <-subIDChan
	require.NotEmpty(t, subIDStr)
	_, err = uuid.Parse(subIDStr)
	require.NoError(t, err)
	subID.Store(&subIDStr)
	close(ackReady) // close ackReady to signal that we are ready to receive receipts

	// We get it on redelivery
	<-receipts

	reqID, req := rpcTestRequest("ptx_unsubscribe", subIDStr)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)
	<-unSubChan

}

func TestRPCEventListenerE2E(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.blockIndexer.On("AddEventStream", mock.Anything, mock.Anything, mock.Anything).Return(&blockindexer.EventStream{
				ID: uuid.New(),
			}, nil)
			mc.blockIndexer.On("StopEventStream", mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	err := txm.CreateBlockchainEventListener(ctx, &pldapi.BlockchainEventListener{
		Name: "listener1",
		Sources: []pldapi.BlockchainEventListenerSource{{
			ABI: abi.ABI{{
				Name: "DataStored",
				Inputs: abi.ParameterArray{
					{Name: "data", Type: "uint256"},
				},
				Type: abi.Event,
			}},
		}},
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe", "blockchainevents")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var subscribeErrResponse *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &subscribeErrResponse)
	require.NoError(t, err)
	require.ErrorContains(t, subscribeErrResponse.Error, "PD012249")

	_, req = rpcTestRequest("ptx_subscribe", "blockchainevents", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload = <-wsc.Receive()

	var subscribeResponse *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &subscribeResponse)
	require.NoError(t, err)
	require.Nil(t, subscribeResponse.Error)
	subID := subscribeResponse.Result.StringValue()

	// there should be exactly one blockchain event listener- find it and send a batch of events
	require.Contains(t, txm.blockchainEventListeners, "listener1")
	el := txm.blockchainEventListeners["listener1"]
	require.NotNil(t, el)

	go func() {
		<-wsc.Receive()

		_, req := rpcTestRequest("ptx_nack", subID)
		err = wsc.Send(ctx, req)
		require.NoError(t, err)

		payload := <-wsc.Receive()

		var batchResponse *rpcclient.RPCResponse
		err = json.Unmarshal(payload, &batchResponse)
		require.NoError(t, err)

		var batchPayload pldapi.JSONRPCSubscriptionNotification[pldapi.TransactionEventBatch]
		err := json.Unmarshal(batchResponse.Params.Bytes(), &batchPayload)
		require.NoError(t, err)

		require.Len(t, batchPayload.Result.Events, 2)

		_, req = rpcTestRequest("ptx_ack", subID)
		err = wsc.Send(ctx, req)
		require.NoError(t, err)

		<-wsc.Receive()

		_, req = rpcTestRequest("ptx_unsubscribe", subID)
		err = wsc.Send(ctx, req)
		require.NoError(t, err)
	}()

	batch1 := &blockindexer.EventDeliveryBatch{
		Events: []*pldapi.EventWithData{
			{
				IndexedEvent: &pldapi.IndexedEvent{
					BlockNumber: 1,
				},
			},
			{
				IndexedEvent: &pldapi.IndexedEvent{
					BlockNumber: 2,
				},
			},
		},
	}

	err = el.handleEventBatch(ctx, batch1)
	require.ErrorContains(t, err, "PD012243")

	err = el.handleEventBatch(ctx, batch1)
	require.NoError(t, err)

	batch2 := &blockindexer.EventDeliveryBatch{
		Events: []*pldapi.EventWithData{
			{
				IndexedEvent: &pldapi.IndexedEvent{
					BlockNumber: 3,
				},
			},
		},
	}

	err = el.handleEventBatch(ctx, batch2)
	require.ErrorContains(t, err, "PD012242")

}

func TestRPCSubscribeNoType(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD020003", rpcPayload.Error.Error())

}

func TestRPCSubscribeNoListener(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe", "receipts")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012241", rpcPayload.Error.Error())

}

func TestRPCSubscribeBadListener(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)

	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe", "receipts", "unknown")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012238", rpcPayload.Error.Error())

}

func TestUnsubscribeNoSubscriptionID(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	err := txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	}, nil, nil)

	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_unsubscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcclient.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012240", rpcPayload.Error.Error())

}

func TestHandleLifecycleUnkonwn(t *testing.T) {
	ctx, _, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	res := txm.rpcEventStreams.HandleLifecycle(ctx, &rpcclient.RPCRequest{
		Method: "wrong",
		Params: []pldtypes.RawJSON{pldtypes.RawJSON(`"any"`)},
	})
	require.Regexp(t, "PD012239", res.Error.Error())

}

type mockRPCAsyncControl struct{}

func (ac *mockRPCAsyncControl) ID() string                     { return "sub1" }
func (ac *mockRPCAsyncControl) Closed()                        {}
func (ac *mockRPCAsyncControl) Send(method string, params any) {}

func TestHandleLifecycleNoBlockNack(t *testing.T) {
	ctx, _, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	ctrl := &mockRPCAsyncControl{}
	es := txm.rpcEventStreams
	es.subs["sub1"] = &listenerSubscription{
		es:        es,
		ctrl:      ctrl,
		acksNacks: make(chan *rpcAckNack),
		closed:    make(chan struct{}),
	}

	res := es.HandleLifecycle(ctx, &rpcclient.RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON("12345"),
		Method:  "ptx_nack",
		Params:  []pldtypes.RawJSON{pldtypes.RawJSON(`"sub1"`)},
	})
	require.Nil(t, res)

	es.getSubscription("sub1").ConnectionClosed()
	require.Empty(t, es.subs)

}
