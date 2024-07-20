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
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

func (s *rpcServer) processRPC(ctx context.Context, rpcReq *rpcbackend.RPCRequest) (*rpcbackend.RPCResponse, bool) {
	if rpcReq.ID == nil {
		err := i18n.NewError(ctx, msgs.MsgJSONRPCMissingRequestID)
		return rpcbackend.RPCErrorResponse(err, rpcReq.ID, rpcbackend.RPCCodeInvalidRequest), false
	}

	handler := s.rpcHandlers[rpcReq.Method]
	if handler == nil {
		err := i18n.NewError(ctx, msgs.MsgJSONRPCUnsupportedMethod)
		return rpcbackend.RPCErrorResponse(err, rpcReq.ID, rpcbackend.RPCCodeInvalidRequest), false
	}

	startTime := time.Now()
	log.L(ctx).Debugf("RPC-> %s", rpcReq.Method)
	rpcRes := handler(ctx, rpcReq)
	durationMS := float64(time.Since(startTime)) / float64(time.Millisecond)
	if rpcRes.Error != nil {
		log.L(ctx).Errorf("<!RPC %s (%.2fms): %s", rpcReq.Method, durationMS, rpcRes.Error.Message)
	} else {
		log.L(ctx).Debugf("<-RPC %s (%.2fms)", rpcReq.Method, durationMS)
	}
	return rpcRes, rpcRes.Error == nil
}
