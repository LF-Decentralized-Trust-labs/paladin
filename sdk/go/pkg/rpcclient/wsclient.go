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

package rpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/wsclient"
	"github.com/sirupsen/logrus"
)

func NewWSClient(ctx context.Context, conf *pldconf.WSClientConfig) (WSClient, error) {
	err := ValidateWSClientConfig(ctx, conf)
	if err != nil {
		return nil, err
	}

	return WrapWSConfig(conf), nil
}

func WrapWSConfig(conf *pldconf.WSClientConfig) WSClient {
	return &wsRPCClient{
		wsConf:              *conf,
		calls:               make(map[string]chan *RPCResponse),
		configuredSubs:      make(map[uuid.UUID]*sub),
		pendingSubsByReqID:  make(map[string]*sub),
		activeSubsBySubID:   make(map[string]*sub),
		notificationMethods: make(map[string]bool),
	}
}

func ValidateWSClientConfig(ctx context.Context, conf *pldconf.WSClientConfig) error {
	_, _, err := wsclient.ValidateConfig(ctx, conf)
	return err
}

type Subscription interface {
	LocalID() uuid.UUID // does not change through reconnects
	Notifications() chan RPCSubscriptionNotification
	Unsubscribe(ctx context.Context) ErrorRPC
}

type RPCSubscriptionNotification interface {
	Ack(ctx context.Context) ErrorRPC
	Nack(ctx context.Context) ErrorRPC
	GetCurrentSubID() string
	GetResult() pldtypes.RawJSON
}

type rpcSubscriptionNotification struct {
	wsc          *wsRPCClient
	sub          *sub
	CurrentSubID string // will change on each reconnect
	Result       pldtypes.RawJSON
}

func (n *rpcSubscriptionNotification) Ack(ctx context.Context) ErrorRPC {
	id, req := n.wsc.newAsyncReq(n.sub.AckMethod, pldtypes.JSONString(n.CurrentSubID))
	return n.wsc.sendRPC(ctx, id, req)
}

func (n *rpcSubscriptionNotification) Nack(ctx context.Context) ErrorRPC {
	id, req := n.wsc.newAsyncReq(n.sub.NackMethod, pldtypes.JSONString(n.CurrentSubID))
	return n.wsc.sendRPC(ctx, id, req)
}

func (n *rpcSubscriptionNotification) GetCurrentSubID() string {
	return n.CurrentSubID
}

func (n *rpcSubscriptionNotification) GetResult() pldtypes.RawJSON {
	return n.Result
}

type wsRPCClient struct {
	mux                 sync.Mutex
	wsConf              pldconf.WSClientConfig
	client              wsclient.WSClient
	requestCounter      int64
	connected           chan struct{}
	calls               map[string]chan *RPCResponse
	configuredSubs      map[uuid.UUID]*sub
	pendingSubsByReqID  map[string]*sub
	activeSubsBySubID   map[string]*sub
	notificationMethods map[string]bool
}

type sub struct {
	SubscriptionConfig
	localID        uuid.UUID
	rc             *wsRPCClient
	ctx            context.Context
	cancelCtx      context.CancelFunc
	params         []interface{}
	pendingReqID   string
	currentSubID   string
	newSubResponse chan ErrorRPC
	notifications  chan RPCSubscriptionNotification
}

func (rc *wsRPCClient) Connect(ctx context.Context) (err error) {
	rc.client, err = wsclient.New(ctx, &rc.wsConf, nil, rc.handleReconnect)
	if err != nil {
		return err
	}
	go rc.receiveLoop(log.WithLogField(ctx, "role", "rpc_websocket"))

	// Wait until the afterConnect hook has been driven
	connected := make(chan struct{})
	rc.connected = connected
	if err := rc.client.Connect(); err != nil {
		return err
	}
	return rc.waitConnected(ctx, connected)
}

func (rc *wsRPCClient) waitConnected(ctx context.Context, connected chan struct{}) error {
	select {
	case <-connected:
	case <-ctx.Done():
		return i18n.NewError(ctx, pldmsgs.MsgContextCanceled)
	}
	return nil
}

func (rc *wsRPCClient) Close() {
	if rc.client != nil {
		rc.client.Close()
	}
}

func (rc *wsRPCClient) handleReconnect(ctx context.Context, w wsclient.WSClient) error {
	calls, subs := rc.clearActiveReturnConfiguredSubs()
	for rpcID, c := range calls {
		rc.deliverCallResponse(c, &RPCResponse{
			ID:    pldtypes.RawJSON(`"` + rpcID + `"`),
			Error: NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientWebSocketReconnected),
		})
	}
	for _, s := range subs {
		log.L(ctx).Infof("Resubscribing %s after WebSocket reconnect", s.localID)
		_, rpcErr := s.sendSubscribe(ctx)
		if rpcErr != nil {
			log.L(ctx).Errorf("Failed to send resubscribe: %s", rpcErr)
			return rpcErr
		}
	}

	if rc.connected != nil {
		close(rc.connected)
		rc.connected = nil
	}
	return nil
}

func (rc *wsRPCClient) newAsyncReq(method string, params ...pldtypes.RawJSON) (string, *RPCRequest) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	rc.requestCounter++
	reqID := fmt.Sprintf(`"%.9d"`, rc.requestCounter)
	return reqID, &RPCRequest{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(reqID),
		Method:  method,
		Params:  params,
	}
}

func (rc *wsRPCClient) addInflightRequest(req *RPCRequest) (string, chan *RPCResponse) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	rc.requestCounter++
	reqID := fmt.Sprintf("%.9d", rc.requestCounter)
	req.ID = pldtypes.RawJSON(`"` + reqID + `"`)
	resChl := make(chan *RPCResponse, 1)
	rc.calls[reqID] = resChl
	return reqID, resChl
}

func (rc *wsRPCClient) addInflightSub(s *sub) string {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	rc.requestCounter++
	s.pendingReqID = fmt.Sprintf("%.9d", rc.requestCounter)
	s.currentSubID = ""
	rc.pendingSubsByReqID[s.pendingReqID] = s
	return s.pendingReqID
}

func (rc *wsRPCClient) popInflight(rpcID string) (*sub, chan *RPCResponse) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	s, ok := rc.pendingSubsByReqID[rpcID]
	if ok {
		s.pendingReqID = ""
		delete(rc.pendingSubsByReqID, rpcID)
		return s, nil
	}
	inflightCall, ok := rc.calls[rpcID]
	if ok {
		delete(rc.calls, rpcID)
		return nil, inflightCall
	}
	return nil, nil
}

func (rc *wsRPCClient) addActiveSub(s *sub, subscriptionID string) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	s.currentSubID = subscriptionID
	rc.activeSubsBySubID[s.currentSubID] = s
}

func (rc *wsRPCClient) getActiveSub(subID string) *sub {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.activeSubsBySubID[subID]
}

func (rc *wsRPCClient) removeSubscription(s *sub) string {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	// Removes from configured, pending and active lists
	delete(rc.configuredSubs, s.localID)
	if s.currentSubID != "" {
		delete(rc.activeSubsBySubID, s.currentSubID)
	}
	if s.pendingReqID != "" {
		delete(rc.pendingSubsByReqID, s.pendingReqID)
	}
	s.cancelCtx() // we unblock the receiver if it was previously trying to dispatch to this subscription, before invoking unsubscribe
	return s.currentSubID
}

func (rc *wsRPCClient) addConfiguredSub(ctx context.Context, conf SubscriptionConfig, params []interface{}) (*sub, chan ErrorRPC) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	s := &sub{
		SubscriptionConfig: conf,
		rc:                 rc,
		localID:            uuid.New(),
		params:             params,
		newSubResponse:     make(chan ErrorRPC, 1),
		notifications:      make(chan RPCSubscriptionNotification), // blocking channel for these, but Unsubscribe will unblock by cancelling ctx
	}
	s.ctx, s.cancelCtx = context.WithCancel(ctx)
	rc.configuredSubs[s.localID] = s
	rc.notificationMethods[conf.NotificationMethod] = true // trigger notification handling for this method type
	// need to return newSubResponse because it's a use-once thing (not on reconnect)
	// and will be nilled out (under lock) when the first creation response comes in
	return s, s.newSubResponse
}

func (rc *wsRPCClient) removeConfiguredSub(id uuid.UUID) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	delete(rc.configuredSubs, id)
}

func (rc *wsRPCClient) clearActiveReturnConfiguredSubs() (map[string]chan *RPCResponse, map[uuid.UUID]*sub) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	// Return a copy of all the in-flight RPC calls, before we clear those (as they will all be defunct now)
	calls := rc.calls
	rc.calls = make(map[string]chan *RPCResponse)
	// Clear the active state as considered now invalid after a reconnect
	rc.activeSubsBySubID = make(map[string]*sub)
	rc.pendingSubsByReqID = make(map[string]*sub)
	// Return all the configured ones so we can re-establish them on the new connecti
	subs := make(map[uuid.UUID]*sub)
	for id, s := range rc.configuredSubs {
		s.currentSubID = ""
		s.pendingReqID = ""
		subs[id] = s
	}
	return calls, subs
}

func (rc *wsRPCClient) getAllSubs() []*sub {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	subs := make([]*sub, 0)
	for _, s := range rc.configuredSubs {
		subs = append(subs, s)
	}
	return subs
}

func (rc *wsRPCClient) removeInflightRequest(reqID string) {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	delete(rc.calls, reqID)
}

func (rc *wsRPCClient) Subscribe(ctx context.Context, conf SubscriptionConfig, params ...interface{}) (sub Subscription, error ErrorRPC) {

	s, newSubResponse := rc.addConfiguredSub(ctx, conf, params)

	reqID, rpcErr := s.sendSubscribe(ctx)
	if rpcErr != nil {
		rc.removeConfiguredSub(s.localID)
		return nil, rpcErr
	}

	select {
	case rpcErr := <-newSubResponse:
		return s, rpcErr
	case <-ctx.Done():
		rc.removeConfiguredSub(s.localID)
		return nil, NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgContextCanceled, reqID)
	}
}

func (s *sub) sendSubscribe(ctx context.Context) (string, ErrorRPC) {
	rpcReq, rpcErr := buildRequest(ctx, s.SubscribeMethod, s.params)
	if rpcErr != nil {
		return "", rpcErr
	}
	reqID := s.rc.addInflightSub(s)
	rpcReq.ID = pldtypes.RawJSON(`"` + reqID + `"`)

	return reqID, s.rc.sendRPC(ctx, s.pendingReqID, rpcReq)
}

func (s *sub) LocalID() uuid.UUID {
	return s.localID
}

func (s *sub) Notifications() chan RPCSubscriptionNotification {
	return s.notifications
}

func (s *sub) Unsubscribe(ctx context.Context) ErrorRPC {
	currentSubID := s.rc.removeSubscription(s)
	var resultBool bool
	if currentSubID != "" {
		// If currently active, we need to unsubscribe
		rpcErr := s.rc.CallRPC(ctx, &resultBool, s.UnsubscribeMethod, currentSubID)
		if rpcErr != nil {
			return rpcErr
		}
	}
	log.L(ctx).Infof("Unsubscribed %s (subid=%s,result=%t)", s.localID, currentSubID, resultBool)
	close(s.notifications)
	return nil
}

func (rc *wsRPCClient) UnsubscribeAll(ctx context.Context) (lastErr ErrorRPC) {
	for _, s := range rc.getAllSubs() {
		if lastErr = s.Unsubscribe(ctx); lastErr != nil {
			log.L(ctx).Errorf("Failed to unsubscribe %s: %s", s.localID, lastErr)
		}
	}
	return lastErr
}

func (rc *wsRPCClient) Subscriptions() []Subscription {
	subs := rc.getAllSubs()
	iSubs := make([]Subscription, len(subs))
	for i, s := range subs {
		iSubs[i] = s
	}
	return iSubs
}

func (rc *wsRPCClient) sendRPC(ctx context.Context, reqID string, rpcReq *RPCRequest) ErrorRPC {
	jsonInput, err := json.Marshal(rpcReq)
	if err == nil {
		log.L(ctx).Debugf("RPC[%s] --> %s", reqID, rpcReq.Method)
		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			log.L(ctx).Tracef("RPC[%s] INPUT: %s", reqID, jsonInput)
		}
		err = rc.client.Send(ctx, jsonInput)
	}
	if err != nil {
		rpcErr := NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientRequestFailed, err)
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, err)
		return rpcErr
	}
	return nil
}

func (rc *wsRPCClient) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) ErrorRPC {
	rpcReq, rpcErr := buildRequest(ctx, method, params)
	if rpcErr != nil {
		return rpcErr
	}

	reqID, resChannel := rc.addInflightRequest(rpcReq)
	defer rc.removeInflightRequest(reqID)

	rpcStartTime := time.Now()
	if rpcErr = rc.sendRPC(ctx, reqID, rpcReq); rpcErr != nil {
		return rpcErr
	}
	return rc.waitResponse(ctx, result, reqID, rpcReq, rpcStartTime, resChannel)
}

func (rc *wsRPCClient) waitResponse(ctx context.Context, result interface{}, reqID string, rpcReq *RPCRequest, rpcStartTime time.Time, resChannel chan *RPCResponse) ErrorRPC {
	var rpcRes *RPCResponse
	select {
	case rpcRes = <-resChannel:
	case <-ctx.Done():
		rpcErr := NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgContextCanceled, reqID)
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, rpcErr)
		return rpcErr
	}
	if rpcRes.Error != nil && rpcRes.Error.RPCError().Code != 0 {
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", reqID, rpcRes.Message())
		return rpcRes.Error
	}
	log.L(ctx).Infof("RPC[%s] <-- %s OK (%.2fms)", reqID, rpcReq.Method, float64(time.Since(rpcStartTime))/float64(time.Millisecond))
	if result != nil {
		if err := json.Unmarshal(rpcRes.Result.Bytes(), &result); err != nil {
			err = i18n.NewError(ctx, pldmsgs.MsgRPCClientResultParseFailed, result, err)
			return &RPCError{Code: int64(RPCCodeParseError), Message: err.Error()}
		}
	}
	return nil
}

func (rc *wsRPCClient) handleSubscriptionNotification(ctx context.Context, rpcRes *RPCResponse) {
	type rpcSubscriptionParams struct {
		Subscription string           `json:"subscription"` // probably hex, but not protocol assured
		Result       pldtypes.RawJSON `json:"result,omitempty"`
	}
	var subParams rpcSubscriptionParams
	if rpcRes.Params != nil {
		_ = json.Unmarshal(rpcRes.Params.Bytes(), &subParams)
	}
	if len(subParams.Subscription) == 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Unable to extract subscription id from notification: %s", rpcRes.ID.StringValue(), rpcRes.Params)
		return
	}

	s := rc.getActiveSub(subParams.Subscription)
	if s == nil {
		log.L(ctx).Warnf("RPC[%s] <-- Notification for unknown subscription '%s'", rpcRes.ID.StringValue(), subParams.Subscription)
		return
	}

	// This is a notification that should match an active subscription
	log.L(ctx).Debugf("RPC[%s] <-- Notification for subscription %s (serverId=%s)", rpcRes.ID.StringValue(), s.localID, s.currentSubID)
	select {
	case s.notifications <- &rpcSubscriptionNotification{
		sub:          s,
		wsc:          rc,
		CurrentSubID: s.currentSubID,
		Result:       subParams.Result,
	}:
	case <-s.ctx.Done():
		// The subscription has been unsubscribed, or we're closing
		log.L(ctx).Warnf("RPC[%s] <-- Received subscription event after unsubscribe/close %s (serverId=%s)", rpcRes.ID.StringValue(), s.localID, s.currentSubID)
	}
}

func (rc *wsRPCClient) handleSubscriptionConfirm(ctx context.Context, inflightSub *sub, rpcRes *RPCResponse) {
	resChl := inflightSub.newSubResponse
	inflightSub.newSubResponse = nil // we only dispatch once (it's only new once, on reconnect it's old and there's nobody to tell if we fail)
	if rpcRes.Error != nil && rpcRes.Error.RPCError().Code != 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Error creating subscription %s: %s", rpcRes.ID.StringValue(), inflightSub.localID, rpcRes.Params)
		if resChl != nil {
			resChl <- rpcRes.Error
		}
		return
	}
	var subscriptionID string // we know it's probably hex, but we cannot rely on that being guaranteed
	if rpcRes.Result != nil {
		_ = json.Unmarshal(rpcRes.Result.Bytes(), &subscriptionID)
	}
	if len(subscriptionID) == 0 {
		log.L(ctx).Warnf("RPC[%s] <-- Unable to extract subscription id from eth_subscribe response: %s", rpcRes.ID.StringValue(), rpcRes.Params)
		if resChl != nil {
			resChl <- NewRPCError(ctx, RPCCodeInternalError, pldmsgs.MsgRPCClientSubscribeResponseInvalid)
		}
		return
	}
	log.L(ctx).Infof("Subscribed %s with server subscription ID '%s'", inflightSub.localID, subscriptionID)
	rc.addActiveSub(inflightSub, subscriptionID)
	// all was good, if someone is waiting to be told, notify them
	if resChl != nil {
		resChl <- nil
	}
}

func (rc *wsRPCClient) deliverCallResponse(inflightCall chan *RPCResponse, rpcRes *RPCResponse) {
	select {
	case inflightCall <- rpcRes:
	default:
		// only considered for the very edge case of reconnect - the inflight response should only be
		// in the map until it's sent a single response, and there's a slot to ensure it never blocks
	}
}

func (rc *wsRPCClient) isNotificationMethod(method string) bool {
	rc.mux.Lock()
	defer rc.mux.Unlock()
	return rc.notificationMethods[method]
}

func (rc *wsRPCClient) receiveLoop(ctx context.Context) {
	for {
		bytes, ok := <-rc.client.Receive()
		if !ok {
			log.L(ctx).Debugf("WebSocket closed")
			return
		}
		rpcRes := RPCResponse{}
		err := json.Unmarshal(bytes, &rpcRes)
		switch {
		case err != nil:
			log.L(ctx).Errorf("RPC <-- ERROR invalid data '%s': %s", bytes, err)
		case rc.isNotificationMethod(rpcRes.Method):
			rc.handleSubscriptionNotification(ctx, &rpcRes)
		default:
			// ID should match a request we sent
			inflightSub, inflightCall := rc.popInflight(rpcRes.ID.StringValue())
			switch {
			case inflightSub != nil:
				rc.handleSubscriptionConfirm(ctx, inflightSub, &rpcRes)
			case inflightCall != nil:
				rc.deliverCallResponse(inflightCall, &rpcRes)
			default:
				log.L(ctx).Warnf("RPC[%s] <-- Received unexpected RPC response: %+v", rpcRes.ID.StringValue(), rpcRes)
			}
		}
	}
}
