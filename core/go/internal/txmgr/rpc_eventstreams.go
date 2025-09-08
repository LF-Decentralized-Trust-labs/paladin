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
	"fmt"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
)

type rpcEventStreams struct {
	tm      *txManager
	subLock sync.Mutex
	subs    map[string]*listenerSubscription
}

func newRPCEventStreams(tm *txManager) *rpcEventStreams {
	es := &rpcEventStreams{
		tm:   tm,
		subs: make(map[string]*listenerSubscription),
	}
	return es
}

func (es *rpcEventStreams) StartMethod() string {
	return "ptx_subscribe"
}

func (es *rpcEventStreams) LifecycleMethods() []string {
	return []string{"ptx_unsubscribe", "ptx_ack", "ptx_nack"}
}

type rpcAckNack struct {
	ack bool
}

type listenerSubscription struct {
	es        *rpcEventStreams
	rrc       components.ReceiverCloser
	ctrl      rpcserver.RPCAsyncControl
	acksNacks chan *rpcAckNack
	closed    chan struct{}
}

func (es *rpcEventStreams) HandleStart(ctx context.Context, req *rpcclient.RPCRequest, ctrl rpcserver.RPCAsyncControl) (rpcserver.RPCAsyncInstance, *rpcclient.RPCResponse) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	var eventType pldtypes.Enum[pldapi.PTXEventType]
	if len(req.Params) >= 1 {
		eventType = pldtypes.Enum[pldapi.PTXEventType](req.Params[0].StringValue())
	}
	if _, err := eventType.Validate(); err != nil {
		return nil, rpcclient.NewRPCErrorResponse(err, req.ID, rpcclient.RPCCodeInvalidRequest)
	}

	if len(req.Params) < 2 {
		if eventType == pldapi.PTXEventTypeEvents.Enum() {
			return nil, rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgTxMgrBlockchainEventListenerNameRequired), req.ID, rpcclient.RPCCodeInvalidRequest)
		} else {
			return nil, rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgTxMgrReceiptListenerNameRequired), req.ID, rpcclient.RPCCodeInvalidRequest)
		}
	}
	sub := &listenerSubscription{
		es:        es,
		ctrl:      ctrl,
		acksNacks: make(chan *rpcAckNack, 1),
		closed:    make(chan struct{}),
	}
	es.subs[ctrl.ID()] = sub
	var err error
	if eventType == pldapi.PTXEventTypeEvents.Enum() {
		sub.rrc, err = es.tm.AddBlockchainEventReceiver(ctx, req.Params[1].StringValue(), sub)
	} else {
		sub.rrc, err = es.tm.AddReceiptReceiver(ctx, req.Params[1].StringValue(), sub)
	}
	if err != nil {
		return nil, rpcclient.NewRPCErrorResponse(err, req.ID, rpcclient.RPCCodeInvalidRequest)
	}

	return sub, &rpcclient.RPCResponse{
		JSONRpc: "2.0",
		ID:      req.ID,
		Result:  pldtypes.JSONString(ctrl.ID()),
	}
}

func (es *rpcEventStreams) cleanupSubscription(subID string) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	sub := es.subs[subID]
	if sub != nil {
		es.cleanupLocked(sub)
	}
}

func (es *rpcEventStreams) getSubscription(subID string) *listenerSubscription {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	return es.subs[subID]
}

func (es *rpcEventStreams) HandleLifecycle(ctx context.Context, req *rpcclient.RPCRequest) *rpcclient.RPCResponse {

	if len(req.Params) < 1 {
		return rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgTxMgrSubIDRequired), req.ID, rpcclient.RPCCodeInvalidRequest)
	}
	subID := req.Params[0].StringValue()
	sub := es.getSubscription(subID)
	switch req.Method {
	case "ptx_ack", "ptx_nack":
		if sub != nil {
			select {
			case sub.acksNacks <- &rpcAckNack{ack: (req.Method == "ptx_ack")}:
				log.L(ctx).Infof("ack/nack received for subID %s ack=%t", subID, req.Method == "ptx_ack")
			default:
			}
		}
		return nil // no reply to acks/nacks - we just send more messages
	case "ptx_unsubscribe":
		if sub != nil {
			sub.ctrl.Closed()
			es.cleanupSubscription(subID)
		}
		return &rpcclient.RPCResponse{
			JSONRpc: "2.0",
			ID:      req.ID,
			Result:  pldtypes.JSONString(sub != nil),
		}
	default:
		return rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgTxMgrLifecycleMethodUnknown, req.Method), req.ID, rpcclient.RPCCodeInvalidRequest)
	}

}

func (sub *listenerSubscription) DeliverReceiptBatch(ctx context.Context, batchID uint64, receipts []*pldapi.TransactionReceiptFull) error {
	log.L(ctx).Infof("Delivering receipt batch %d to subscription %s over JSON/RPC", batchID, sub.ctrl.ID())

	// Note we attempt strong consistency with etH_subscribe semantics here, as described in https://geth.ethereum.org/docs/interacting-with-geth/rpc/pubsub
	// However, we have layered acks on top - so we're not 100%.
	// We also end up with quite a bit of nesting doing this:
	// { "jsonrpc": "2.0", "method": "ptx_subscription",
	//    "params": {
	//       "subscription": "0xcd0c3e8af590364c09d0fa6a1210faf5",
	//       "result": {
	//         "batchId": 12345,
	//         "receipts": [ ... interesting stuff ]
	//       }
	//     }
	// }
	sub.ctrl.Send("ptx_subscription", &pldapi.JSONRPCSubscriptionNotification[pldapi.TransactionReceiptBatch]{
		Subscription: sub.ctrl.ID(),
		Result: pldapi.TransactionReceiptBatch{
			BatchID:  batchID,
			Receipts: receipts,
		},
	})
	return sub.WaitForAck(ctx, fmt.Sprintf("%d", batchID))
}

func (sub *listenerSubscription) DeliverBlockchainEventBatch(ctx context.Context, batchID uuid.UUID, events []*pldapi.EventWithData) error {
	log.L(ctx).Infof("Delivering event batch %d to subscription %s over JSON/RPC", batchID, sub.ctrl.ID())

	sub.ctrl.Send("ptx_subscription", &pldapi.JSONRPCSubscriptionNotification[pldapi.TransactionEventBatch]{
		Subscription: sub.ctrl.ID(),
		Result: pldapi.TransactionEventBatch{
			BatchID: batchID,
			Events:  events,
		},
	})
	return sub.WaitForAck(ctx, batchID.String())
}

func (sub *listenerSubscription) WaitForAck(ctx context.Context, batchID string) error {
	select {
	case ackNack := <-sub.acksNacks:
		if !ackNack.ack {
			log.L(ctx).Warnf("Batch %s negatively acknowledged by subscription %s over JSON/RPC", batchID, sub.ctrl.ID())
			return i18n.NewError(ctx, msgs.MsgTxMgrJSONRPCSubscriptionNack, sub.ctrl.ID())
		}
		log.L(ctx).Infof("Batch %s acknowledged by subscription %s over JSON/RPC", batchID, sub.ctrl.ID())
		return nil
	case <-sub.closed:
		return i18n.NewError(ctx, msgs.MsgTxMgrJSONRPCSubscriptionClosed, sub.ctrl.ID())
	}
}

func (sub *listenerSubscription) ConnectionClosed() {
	sub.es.cleanupSubscription(sub.ctrl.ID())
}

func (es *rpcEventStreams) cleanupLocked(sub *listenerSubscription) {
	delete(sub.es.subs, sub.ctrl.ID())
	if sub.rrc != nil {
		sub.rrc.Close()
	}
	close(sub.closed)
}

func (es *rpcEventStreams) stop() {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	for _, sub := range es.subs {
		es.cleanupLocked(sub)
	}

}
