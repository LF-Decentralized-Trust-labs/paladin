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
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/require"
)

var nextReq atomic.Uint64

func rpcTestRequest(method string, params ...any) (uint64, []byte) {
	reqID := nextReq.Add(1)
	jsonParams := make([]*fftypes.JSONAny, len(params))
	for i, p := range params {
		jsonParams[i] = fftypes.JSONAnyPtr(tktypes.JSONString((p)).Pretty())
	}
	req := &rpcbackend.RPCRequest{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr(fmt.Sprintf("%d", reqID)),
		Method:  method,
		Params:  jsonParams,
	}
	return reqID, []byte(tktypes.JSONString((req)).Pretty())
}

func TestRPCEventListenerE2E(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("ptx_subscribe", "receipts", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	receipts := make(chan *pldapi.TransactionReceiptFull)
	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	go func() {
		for payload := range wsc.Receive() {
			var rpcPayload *rpcbackend.RPCResponse
			err := json.Unmarshal(payload, &rpcPayload)
			require.NoError(t, err)

			if rpcPayload.Error != nil {
				require.NoError(t, rpcPayload.Error.Error())
			}

			if !rpcPayload.ID.IsNil() {
				var rpcID uint64
				err := json.Unmarshal(rpcPayload.ID.Bytes(), &rpcID)
				require.NoError(t, err)

				switch rpcID {
				case subReqID: // Subscribe reply
					subIDChan <- rpcPayload.Result.AsString()
				case unSubReqID.Load(): // Unsubscribe reply
					unSubChan <- true
				}
			}

			if rpcPayload.Method == "ptx_receiptBatch" {
				var batchPayload pldapi.TransactionReceiptBatch
				err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
				require.NoError(t, err)

				for _, r := range batchPayload.Receipts {
					receipts <- r
				}

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
			OnChain:       randOnChain(tktypes.RandAddress()),
		}
	}

	// Send first 3
	postCommit, err := txm.FinalizeTransactions(ctx, txm.p.DB(), txs[0:3])
	require.NoError(t, err)
	postCommit()

	subIDStr := <-subIDChan
	_, err = uuid.Parse(subIDStr)
	require.NoError(t, err)
	subID.Store(&subIDStr)

	for i := 0; i < 3; i++ {
		require.Equal(t, txs[i].TransactionID, (<-receipts).ID)
	}

	// Send rest
	postCommit, err = txm.FinalizeTransactions(ctx, txm.p.DB(), txs[3:])
	require.NoError(t, err)
	postCommit()

	for i := 3; i < len(txs); i++ {
		require.Equal(t, txs[i].TransactionID, (<-receipts).ID)
	}

	reqID, req := rpcTestRequest("ptx_unsubscribe", subIDStr)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)
	<-unSubChan

}

func TestRPCEventListenerE2ENack(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	subReqID, req := rpcTestRequest("ptx_subscribe", "receipts", "listener1")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	subIDChan := make(chan string)
	unSubChan := make(chan bool)
	sentNack := false
	receipts := make(chan *pldapi.TransactionReceiptFull)
	var unSubReqID atomic.Uint64
	var subID atomic.Pointer[string]

	go func() {
		for payload := range wsc.Receive() {
			var rpcPayload *rpcbackend.RPCResponse
			err := json.Unmarshal(payload, &rpcPayload)
			require.NoError(t, err)

			if rpcPayload.Error != nil {
				require.NoError(t, rpcPayload.Error.Error())
			}

			if !rpcPayload.ID.IsNil() {
				var rpcID uint64
				err := json.Unmarshal(rpcPayload.ID.Bytes(), &rpcID)
				require.NoError(t, err)

				switch rpcID {
				case subReqID: // Subscribe reply
					subIDChan <- rpcPayload.Result.AsString()
				case unSubReqID.Load(): // Unsubscribe reply
					unSubChan <- true
				}
			}

			if rpcPayload.Method == "ptx_receiptBatch" {
				var batchPayload pldapi.TransactionReceiptBatch
				err := json.Unmarshal(rpcPayload.Params.Bytes(), &batchPayload)
				require.NoError(t, err)

				if !sentNack {
					// send nack first
					_, req := rpcTestRequest("ptx_nack", *subID.Load())
					err = wsc.Send(ctx, req)
					require.NoError(t, err)
					sentNack = true
				} else {
					// then ack
					for _, r := range batchPayload.Receipts {
						receipts <- r
					}
					_, req := rpcTestRequest("ptx_ack", *subID.Load())
					err = wsc.Send(ctx, req)
					require.NoError(t, err)

				}
			}

		}
	}()

	postCommit, err := txm.FinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{
			ReceiptType:   components.RT_Success,
			TransactionID: uuid.New(),
			OnChain:       randOnChain(tktypes.RandAddress()),
		},
	})
	require.NoError(t, err)
	postCommit()

	subIDStr := <-subIDChan
	_, err = uuid.Parse(subIDStr)
	require.NoError(t, err)
	subID.Store(&subIDStr)

	// We get it on redelivery
	<-receipts

	reqID, req := rpcTestRequest("ptx_unsubscribe", subIDStr)
	unSubReqID.Store(reqID)
	err = wsc.Send(ctx, req)
	require.NoError(t, err)
	<-unSubChan

}

func TestRPCSubscribeNoType(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcbackend.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD020003", rpcPayload.Error.Error())

}

func TestRPCSubscribeNoListener(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe", "receipts")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcbackend.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012241", rpcPayload.Error.Error())

}

func TestRPCSubscribeBadListener(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_subscribe", "receipts", "unknown")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcbackend.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012238", rpcPayload.Error.Error())

}

func TestUnsubscribeNoSubscriptionID(t *testing.T) {
	ctx, url, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	wscConf, err := rpcclient.ParseWSConfig(ctx, &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{URL: url},
	})
	require.NoError(t, err)

	err = txm.CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
		Name: "listener1",
	})
	require.NoError(t, err)

	wsc, err := wsclient.New(ctx, wscConf, nil, nil)
	require.NoError(t, err)
	err = wsc.Connect()
	require.NoError(t, err)
	defer wsc.Close()

	_, req := rpcTestRequest("ptx_unsubscribe")
	err = wsc.Send(ctx, req)
	require.NoError(t, err)

	payload := <-wsc.Receive()

	var rpcPayload *rpcbackend.RPCResponse
	err = json.Unmarshal(payload, &rpcPayload)
	require.NoError(t, err)
	require.Regexp(t, "PD012240", rpcPayload.Error.Error())

}

func TestHandleLifecycleUnkonwn(t *testing.T) {
	ctx, _, txm, done := newTestTransactionManagerWithWebSocketRPC(t)
	defer done()

	res := txm.rpcEventStreams.HandleLifecycle(ctx, &rpcbackend.RPCRequest{
		Method: "wrong",
		Params: []*fftypes.JSONAny{fftypes.JSONAnyPtr(`"any"`)},
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
	es.receiptSubs["sub1"] = &receiptListenerSubscription{
		es:        es,
		ctrl:      ctrl,
		acksNacks: make(chan *rpcAckNack),
		closed:    make(chan struct{}),
	}

	res := es.HandleLifecycle(ctx, &rpcbackend.RPCRequest{
		JSONRpc: "2.0",
		ID:      fftypes.JSONAnyPtr("12345"),
		Method:  "ptx_nack",
		Params:  []*fftypes.JSONAny{fftypes.JSONAnyPtr(`"sub1"`)},
	})
	require.Nil(t, res)

	es.getSubscription("sub1").ConnectionClosed()
	require.Empty(t, es.receiptSubs)

}
