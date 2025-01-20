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
	"encoding/json"
	"io"
	"unicode"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type handlerResult struct {
	sendRes bool
	isOK    bool
	res     any
}

func (s *rpcServer) rpcHandler(ctx context.Context, r io.Reader, wsc *webSocketConnection) handlerResult {

	b, err := io.ReadAll(r)
	if err != nil {
		return s.replyRPCParseError(ctx, b, err)
	}

	log.L(ctx).Infof("RPC[Server] --> %s", b)

	if s.sniffFirstByte(b) == '[' {
		var rpcArray []*rpcclient.RPCRequest
		err := json.Unmarshal(b, &rpcArray)
		if err != nil || len(rpcArray) == 0 {
			log.L(ctx).Errorf("Bad RPC array received %s", b)
			return s.replyRPCParseError(ctx, b, err)
		}
		batchRes, isOK := s.handleRPCBatch(ctx, rpcArray, wsc)
		return handlerResult{isOK: isOK, sendRes: true, res: batchRes}
	}

	var rpcRequest rpcclient.RPCRequest
	err = json.Unmarshal(b, &rpcRequest)
	if err != nil {
		return s.replyRPCParseError(ctx, b, err)
	}
	res, isOK := s.processRPC(ctx, &rpcRequest, wsc)
	return handlerResult{isOK: isOK, sendRes: res != nil, res: res}

}

func (s *rpcServer) replyRPCParseError(ctx context.Context, b []byte, err error) handlerResult {
	log.L(ctx).Errorf("Request could not be parsed (err=%v): %s", err, b)
	return handlerResult{
		isOK:    false,
		sendRes: true,
		res: rpcclient.NewRPCErrorResponse(
			i18n.NewError(ctx, tkmsgs.MsgJSONRPCInvalidRequest),
			tktypes.RawJSON(`"1"`),
			rpcclient.RPCCodeInvalidRequest,
		),
	}
}

func (s *rpcServer) sniffFirstByte(data []byte) byte {
	sniffLen := len(data)
	if sniffLen > 100 {
		sniffLen = 100
	}
	for _, b := range data[0:sniffLen] {
		if !unicode.IsSpace(rune(b)) {
			return b
		}
	}
	return 0x00
}

func (s *rpcServer) handleRPCBatch(ctx context.Context, rpcArray []*rpcclient.RPCRequest, wsc *webSocketConnection) ([]*rpcclient.RPCResponse, bool) {

	// Kick off a routine to fill in each
	rpcResponses := make([]*rpcclient.RPCResponse, len(rpcArray))
	results := make(chan bool)
	for i, r := range rpcArray {
		responseNumber := i
		rpcReq := r
		go func() {
			var ok bool
			rpcResponses[responseNumber], ok = s.processRPC(ctx, rpcReq, wsc)
			results <- ok
		}()
	}
	failCount := 0
	for range rpcResponses {
		ok := <-results
		if !ok {
			failCount++
		}
	}
	// Only return a failure response code if all the requests in the batch failed
	return rpcResponses, failCount != len(rpcArray)
}
