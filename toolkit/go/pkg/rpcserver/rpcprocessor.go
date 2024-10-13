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
	"strings"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tkmsgs"
)

func (s *rpcServer) processRPC(ctx context.Context, rpcReq *rpcclient.RPCRequest) (*rpcclient.RPCResponse, bool) {
	if rpcReq.ID == nil {
		// While the JSON/RPC standard does not strictly require an ID (it strongly discourages use of a null ID),
		// we choose to make an ID mandatory. We do not enforce the type - it can be a number, string, or even boolean.
		// However, it cannot be null.
		err := i18n.NewError(ctx, tkmsgs.MsgJSONRPCMissingRequestID)
		return rpcclient.NewRPCErrorResponse(err, rpcReq.ID, rpcclient.RPCCodeInvalidRequest), false
	}

	var handler RPCHandler
	group := strings.SplitN(rpcReq.Method, "_", 2)[0]
	module := s.rpcModules[group]
	if module != nil {
		handler = module.methods[rpcReq.Method]
	}
	if handler == nil {
		err := i18n.NewError(ctx, tkmsgs.MsgJSONRPCUnsupportedMethod)
		return rpcclient.NewRPCErrorResponse(err, rpcReq.ID, rpcclient.RPCCodeInvalidRequest), false
	}

	startTime := time.Now()
	log.L(ctx).Debugf("RPC-> %s", rpcReq.Method)
	rpcRes := handler.Handle(ctx, rpcReq)
	durationMS := float64(time.Since(startTime)) / float64(time.Millisecond)
	if rpcRes.Error != nil {
		log.L(ctx).Errorf("<!RPC[Server] %s (%.2fms): %s", rpcReq.Method, durationMS, rpcRes.Error.Message)
	} else {
		log.L(ctx).Debugf("<-RPC[Server] %s (%.2fms)", rpcReq.Method, durationMS)
	}
	return rpcRes, rpcRes.Error == nil
}
