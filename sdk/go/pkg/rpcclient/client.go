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
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldresty"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/sirupsen/logrus"
)

type RPCCode int64

const (
	RPCCodeParseError     RPCCode = -32700
	RPCCodeInvalidRequest RPCCode = -32600
	RPCCodeInternalError  RPCCode = -32603
)

// NewRPCClient Constructor
func NewHTTPClient(ctx context.Context, conf *pldconf.HTTPClientConfig) (Client, error) {
	rc, err := pldresty.New(ctx, conf)
	if err != nil {
		return nil, err
	}
	return WrapRestyClient(rc), nil
}

func WrapRestyClient(rc *resty.Client) Client {
	return &rpcClient{client: rc}
}

type Byteable interface {
	Bytes() []byte
}

func NewRPCErrorResponse(err error, id Byteable, code RPCCode) *RPCResponse {
	var byteID []byte
	if id != nil {
		byteID = id.Bytes()
	}
	return &RPCResponse{
		JSONRpc: "2.0",
		ID:      pldtypes.RawJSON(byteID),
		Error: &RPCError{
			Code:    int64(code),
			Message: err.Error(),
		},
	}
}

type ErrorRPC interface {
	error
	RPCError() *RPCError
}

type Client interface {
	CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) ErrorRPC
}

type SubscriptionConfig struct {
	SubscribeMethod    string
	UnsubscribeMethod  string
	NotificationMethod string
	AckMethod          string
	NackMethod         string
}

func EthSubscribeConfig() SubscriptionConfig {
	return SubscriptionConfig{
		SubscribeMethod:    "eth_subscribe",
		UnsubscribeMethod:  "eth_unsubscribe",
		NotificationMethod: "eth_subscription",
	}
}

type WSClient interface {
	Client
	Subscribe(ctx context.Context, conf SubscriptionConfig, params ...interface{}) (Subscription, ErrorRPC)
	Subscriptions() []Subscription
	UnsubscribeAll(ctx context.Context) ErrorRPC
	Connect(ctx context.Context) error
	Close()
}

type rpcClient struct {
	client         *resty.Client
	requestCounter int64
}

type RPCClientOptions struct {
	MaxConcurrentRequest int64
}

type RPCRequest struct {
	JSONRpc string             `json:"jsonrpc"`
	ID      pldtypes.RawJSON   `json:"id"`
	Method  string             `json:"method"`
	Params  []pldtypes.RawJSON `json:"params,omitempty"`
}

type RPCError struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    pldtypes.RawJSON `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

func (e *RPCError) RPCError() *RPCError {
	return e
}

type RPCResponse struct {
	JSONRpc string           `json:"jsonrpc"`
	ID      pldtypes.RawJSON `json:"id"`
	Result  pldtypes.RawJSON `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
	// Only for subscription notifications
	Method string           `json:"method,omitempty"`
	Params pldtypes.RawJSON `json:"params,omitempty"`
}

func (r *RPCResponse) Message() string {
	if r.Error != nil {
		return r.Error.Error()
	}
	return ""
}

func (rc *rpcClient) allocateRequestID(req *RPCRequest) string {
	reqID := fmt.Sprintf(`%.9d`, atomic.AddInt64(&rc.requestCounter, 1))
	req.ID = pldtypes.RawJSON(`"` + reqID + `"`)
	return reqID
}

func (rc *rpcClient) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) ErrorRPC {
	rpcReq, rpcErr := buildRequest(ctx, method, params)
	if rpcErr != nil {
		return rpcErr
	}
	res, err := rc.SyncRequest(ctx, rpcReq)
	if err != nil {
		if res != nil && res.Error != nil && res.Error.RPCError().Code != 0 {
			return res.Error
		}
		return &RPCError{Code: int64(RPCCodeInternalError), Message: err.Error()}
	}
	err = json.Unmarshal(res.Result.Bytes(), &result)
	if err != nil {
		err = i18n.NewError(ctx, pldmsgs.MsgRPCClientResultParseFailed, result, err)
		return &RPCError{Code: int64(RPCCodeParseError), Message: err.Error()}
	}
	return nil
}

// SyncRequest sends an individual RPC request to the backend (always over HTTP currently),
// and waits synchronously for the response, or an error.
//
// In all return paths *including error paths* the RPCResponse is populated
// so the caller has an RPC structure to send back to the front-end caller.
func (rc *rpcClient) SyncRequest(ctx context.Context, rpcReq *RPCRequest) (rpcRes *RPCResponse, err error) {

	// We always set the back-end request ID - as we need to support requests coming in from
	// multiple concurrent clients on our front-end that might use clashing IDs.
	var beReq = *rpcReq
	beReq.JSONRpc = "2.0"
	rpcTraceID := rc.allocateRequestID(&beReq)
	if rpcReq.ID != nil {
		// We're proxying a request with front-end RPC ID - log that as well
		rpcTraceID = fmt.Sprintf("%s->%s", rpcReq.ID, rpcTraceID)
	}

	rpcRes = new(RPCResponse)

	log.L(ctx).Debugf("RPC[%s] --> %s", rpcTraceID, rpcReq.Method)
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		jsonInput, _ := json.Marshal(rpcReq)
		log.L(ctx).Tracef("RPC[%s] INPUT: %s", rpcTraceID, jsonInput)
	}
	rpcStartTime := time.Now()
	res, err := rc.client.R().
		SetContext(ctx).
		SetBody(beReq).
		SetResult(&rpcRes).
		SetError(rpcRes).
		Post("")

	// Restore the original ID
	rpcRes.ID = rpcReq.ID
	if err != nil {
		err := i18n.NewError(ctx, pldmsgs.MsgRPCClientRequestFailed, err)
		log.L(ctx).Errorf("RPC[%s] <-- ERROR: %s", rpcTraceID, err)
		rpcRes = RPCErrorResponse(err, rpcReq.ID, RPCCodeInternalError)
		return rpcRes, err
	}
	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		jsonOutput, _ := json.Marshal(rpcRes)
		log.L(ctx).Tracef("RPC[%s] OUTPUT: %s", rpcTraceID, jsonOutput)
	}
	// JSON/RPC allows errors to be returned with a 200 status code, as well as other status codes
	if res.IsError() || rpcRes.Error != nil && rpcRes.Error.RPCError().Code != 0 {
		rpcMsg := rpcRes.Message()
		errLog := rpcMsg
		if rpcMsg == "" {
			// Log the raw result in the case of JSON parse error etc. (note that Resty no longer
			// returns this as an error - rather the body comes back raw)
			errLog = string(res.Body())
			rpcMsg = i18n.NewError(ctx, pldmsgs.MsgRPCClientRequestFailed, res.Status()).Error()
		}
		log.L(ctx).Errorf("RPC[%s] <-- [%d]: %s", rpcTraceID, res.StatusCode(), errLog)
		err := errors.New(rpcMsg)
		return rpcRes, err
	}
	log.L(ctx).Infof("RPC[%s] <-- %s [%d] OK (%.2fms)", rpcTraceID, rpcReq.Method, res.StatusCode(), float64(time.Since(rpcStartTime))/float64(time.Millisecond))
	return rpcRes, nil
}

func RPCErrorResponse(err error, id pldtypes.RawJSON, code RPCCode) *RPCResponse {
	return &RPCResponse{
		JSONRpc: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    int64(code),
			Message: err.Error(),
		},
	}
}

func buildRequest(ctx context.Context, method string, params []interface{}) (*RPCRequest, ErrorRPC) {
	req := &RPCRequest{
		JSONRpc: "2.0",
		Method:  method,
		Params:  make([]pldtypes.RawJSON, len(params)),
	}
	for i, param := range params {
		b, err := json.Marshal(param)
		if err != nil {
			return nil, NewRPCError(ctx, RPCCodeInvalidRequest, pldmsgs.MsgRPCClientInvalidParam, i, method, err)
		}
		req.Params[i] = pldtypes.RawJSON(b)
	}
	return req, nil
}

func NewRPCError(ctx context.Context, code RPCCode, msg i18n.ErrorMessageKey, inserts ...interface{}) *RPCError {
	return &RPCError{Code: int64(code), Message: i18n.NewError(ctx, msg, inserts...).Error()}
}

func WrapRPCError(code RPCCode, err error) *RPCError {
	return &RPCError{Code: int64(code), Message: err.Error()}
}
