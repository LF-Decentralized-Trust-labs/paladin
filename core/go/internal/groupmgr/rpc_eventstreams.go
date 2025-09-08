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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
)

type rpcEventStreams struct {
	gm          *groupManager
	subLock     sync.Mutex
	receiptSubs map[string]*receiptListenerSubscription
}

func newRPCEventStreams(tm *groupManager) *rpcEventStreams {
	es := &rpcEventStreams{
		gm:          tm,
		receiptSubs: make(map[string]*receiptListenerSubscription),
	}
	return es
}

func (es *rpcEventStreams) StartMethod() string {
	return "pgroup_subscribe"
}

func (es *rpcEventStreams) LifecycleMethods() []string {
	return []string{"pgroup_unsubscribe", "pgroup_ack", "pgroup_nack"}
}

type rpcAckNack struct {
	ack bool
}

type receiptListenerSubscription struct {
	es        *rpcEventStreams
	pgmrc     components.PrivacyGroupMessageReceiverCloser
	ctrl      rpcserver.RPCAsyncControl
	acksNacks chan *rpcAckNack
	closed    chan struct{}
}

func (es *rpcEventStreams) HandleStart(ctx context.Context, req *rpcclient.RPCRequest, ctrl rpcserver.RPCAsyncControl) (rpcserver.RPCAsyncInstance, *rpcclient.RPCResponse) {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	var eventType pldtypes.Enum[pldapi.PGroupEventType]
	if len(req.Params) >= 1 {
		eventType = pldtypes.Enum[pldapi.PGroupEventType](req.Params[0].StringValue())
	}
	if _, err := eventType.Validate(); err != nil {
		return nil, rpcclient.NewRPCErrorResponse(err, req.ID, rpcclient.RPCCodeInvalidRequest)
	}

	// Only one type right now
	if len(req.Params) < 2 {
		return nil, rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgPGroupsListenerNameRequired), req.ID, rpcclient.RPCCodeInvalidRequest)
	}
	sub := &receiptListenerSubscription{
		es:        es,
		ctrl:      ctrl,
		acksNacks: make(chan *rpcAckNack, 1),
		closed:    make(chan struct{}),
	}
	es.receiptSubs[ctrl.ID()] = sub
	var err error
	sub.pgmrc, err = es.gm.AddMessageReceiver(ctx, req.Params[1].StringValue(), sub)
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

	sub := es.receiptSubs[subID]
	if sub != nil {
		es.cleanupLocked(sub)
	}
}

func (es *rpcEventStreams) getSubscription(subID string) *receiptListenerSubscription {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	return es.receiptSubs[subID]
}

func (es *rpcEventStreams) HandleLifecycle(ctx context.Context, req *rpcclient.RPCRequest) *rpcclient.RPCResponse {

	if len(req.Params) < 1 {
		return rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgPGroupsSubIDRequired), req.ID, rpcclient.RPCCodeInvalidRequest)
	}
	subID := req.Params[0].StringValue()
	sub := es.getSubscription(subID)
	switch req.Method {
	case "pgroup_ack", "pgroup_nack":
		if sub != nil {
			select {
			case sub.acksNacks <- &rpcAckNack{ack: (req.Method == "pgroup_ack")}:
				log.L(ctx).Infof("ack/nack received for subID %s ack=%t", subID, req.Method == "pgroup_ack")
			default:
			}
		}
		return nil // no reply to acks/nacks - we just send more messages
	case "pgroup_unsubscribe":
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
		return rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, msgs.MsgPGroupsLifecycleMethodUnknown, req.Method), req.ID, rpcclient.RPCCodeInvalidRequest)
	}

}

func (sub *receiptListenerSubscription) DeliverMessageBatch(ctx context.Context, batchID uint64, messages []*pldapi.PrivacyGroupMessage) error {
	log.L(ctx).Infof("Delivering receipt batch %d to subscription %s over JSON/RPC", batchID, sub.ctrl.ID())

	// Note we attempt strong consistency with etH_subscribe semantics here, as described in https://geth.ethereum.org/docs/interacting-with-geth/rpc/pubsub
	// However, we have layered acks on top - so we're not 100%.
	// We also end up with quite a bit of nesting doing this:
	// { "jsonrpc": "2.0", "method": "pgroup_subscription",
	//    "params": {
	//       "subscription": "0xcd0c3e8af590364c09d0fa6a1210faf5",
	//       "result": {
	//         "batchId": 12345,
	//         "messages": [ ... interesting stuff ]
	//       }
	//     }
	// }
	sub.ctrl.Send("pgroup_subscription", &pldapi.JSONRPCSubscriptionNotification[pldapi.PrivacyGroupMessageBatch]{
		Subscription: sub.ctrl.ID(),
		Result: pldapi.PrivacyGroupMessageBatch{
			BatchID:  batchID,
			Messages: messages,
		},
	})
	select {
	case ackNack := <-sub.acksNacks:
		if !ackNack.ack {
			log.L(ctx).Warnf("Batch %d negatively acknowledged by subscription %s over JSON/RPC", batchID, sub.ctrl.ID())
			return i18n.NewError(ctx, msgs.MsgPGroupsJSONRPCSubscriptionNack, sub.ctrl.ID())
		}
		log.L(ctx).Infof("Batch %d acknowledged by subscription %s over JSON/RPC", batchID, sub.ctrl.ID())
		return nil
	case <-sub.closed:
		return i18n.NewError(ctx, msgs.MsgPGroupsJSONRPCSubscriptionClosed, sub.ctrl.ID())
	}
}

func (sub *receiptListenerSubscription) ConnectionClosed() {
	sub.es.cleanupSubscription(sub.ctrl.ID())
}

func (es *rpcEventStreams) cleanupLocked(sub *receiptListenerSubscription) {
	delete(sub.es.receiptSubs, sub.ctrl.ID())
	if sub.pgmrc != nil {
		sub.pgmrc.Close()
	}
	close(sub.closed)
}

func (es *rpcEventStreams) stop() {
	es.subLock.Lock()
	defer es.subLock.Unlock()

	for _, sub := range es.receiptSubs {
		es.cleanupLocked(sub)
	}

}
