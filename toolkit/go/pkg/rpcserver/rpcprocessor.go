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
	"strings"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
)

// getAuthenticationResults retrieves authentication results from either HTTP context or WebSocket connection
func getAuthenticationResults(ctx context.Context, wsc *webSocketConnection) []string {
	if wsc == nil {
		// HTTP request: get from context
		if results, ok := ctx.Value(authResultKey).([]string); ok {
			return results
		}
		return nil
	}
	// WebSocket request: get from connection (which was populated from context during upgrade)
	return wsc.getAuthenticationResults()
}

func (s *rpcServer) processRPC(ctx context.Context, rpcReq *rpcclient.RPCRequest, wsc *webSocketConnection) (*rpcclient.RPCResponse, bool) {
	if rpcReq.ID == nil {
		// While the JSON/RPC standard does not strictly require an ID (it strongly discourages use of a null ID),
		// we choose to make an ID mandatory. We do not enforce the type - it can be a number, string, or even boolean.
		// However, it cannot be null.
		err := i18n.NewError(ctx, pldmsgs.MsgJSONRPCMissingRequestID)
		return rpcclient.NewRPCErrorResponse(err, rpcReq.ID, rpcclient.RPCCodeInvalidRequest), false
	}

	// Check authorizers if configured
	if len(s.authorizers) > 0 {
		// Get authentication results (from context for HTTP, from connection for WebSocket)
		authenticationResults := getAuthenticationResults(ctx, wsc)
		if len(authenticationResults) == 0 {
			// This shouldn't happen if auth is required and authentication succeeded
			// But handle gracefully
			log.L(ctx).Errorf("Request without stored authentication results")
			return rpcclient.NewRPCErrorResponse(
				i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnauthorized),
				rpcReq.ID,
				rpcclient.RPCCodeUnauthorized,
			), false
		}

		// Authorize through chain - stop on first failure
		payload, _ := json.Marshal(rpcReq)
		for i, auth := range s.authorizers {
			if i >= len(authenticationResults) {
				log.L(ctx).Errorf("Mismatch: authorizer index %d exceeds authentication results count %d", i, len(authenticationResults))
				return rpcclient.NewRPCErrorResponse(
					i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnauthorized),
					rpcReq.ID,
					rpcclient.RPCCodeUnauthorized,
				), false
			}

			authorized := auth.Authorize(ctx, authenticationResults[i], rpcReq.Method, payload)
			if !authorized {
				log.L(ctx).Errorf("Unauthorized request to %s at authorizer %d", rpcReq.Method, i)
				return rpcclient.NewRPCErrorResponse(
					i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnauthorized),
					rpcReq.ID,
					rpcclient.RPCCodeUnauthorized,
				), false
			}
		}
	}

	var mh *rpcMethodEntry
	group := strings.SplitN(rpcReq.Method, "_", 2)[0]
	module := s.rpcModules[group]
	if module != nil {
		mh = module.methods[rpcReq.Method]
	}
	if mh == nil {
		err := i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnsupportedMethod, rpcReq.Method)
		return rpcclient.NewRPCErrorResponse(err, rpcReq.ID, rpcclient.RPCCodeInvalidRequest), false
	}

	var rpcRes *rpcclient.RPCResponse
	if mh.methodType == rpcMethodTypeMethod {
		rpcRes = mh.handler.Handle(ctx, rpcReq)
	} else {
		if wsc == nil {
			return rpcclient.NewRPCErrorResponse(i18n.NewError(ctx, pldmsgs.MsgJSONRPCAysncNonWSConn, rpcReq.Method), rpcReq.ID, rpcclient.RPCCodeInvalidRequest), false
		}
		if mh.methodType == rpcMethodTypeAsyncStart {
			rpcRes = wsc.handleNewAsync(ctx, rpcReq, mh.async)
		} else {
			rpcRes = wsc.handleLifecycle(ctx, rpcReq, mh.async)
		}
	}
	isOK := true
	if rpcRes != nil {
		isOK = rpcRes.Error == nil
	}
	return rpcRes, isOK
}
