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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/internal/msgs"
)

// RPCHandler should not be implemented directly - use RPCMethod0 ... RPCMethod5 to implement your function
// These use generics to avoid you needing to do any messy type mapping in your functions.
type RPCHandler interface {
	Handle(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse
}

func HandlerFunc(fn func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse) RPCHandler {
	return &rpcHandlerFunc{fn: fn}
}

type rpcHandlerFunc struct {
	fn func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse
}

func (hf *rpcHandlerFunc) Handle(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
	return hf.fn(ctx, req)
}

func RPCMethod0[R any](impl func(ctx context.Context) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		code, err := parseParams(ctx, req)
		if err == nil {
			result, err = impl(ctx)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func RPCMethod1[R any, P0 any](impl func(ctx context.Context, param0 P0) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		param0 := new(P0)
		code, err := parseParams(ctx, req, param0)
		if err == nil {
			result, err = impl(ctx, *param0)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func RPCMethod2[R any, P0 any, P1 any](impl func(ctx context.Context, param0 P0, param1 P1) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		param0 := new(P0)
		param1 := new(P1)
		code, err := parseParams(ctx, req, param0, param1)
		if err == nil {
			result, err = impl(ctx, *param0, *param1)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func RPCMethod3[R any, P0 any, P1 any, P2 any](impl func(ctx context.Context, param0 P0, param1 P1, param2 P2) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		param0 := new(P0)
		param1 := new(P1)
		param2 := new(P2)
		code, err := parseParams(ctx, req, param0, param1, param2)
		if err == nil {
			result, err = impl(ctx, *param0, *param1, *param2)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func RPCMethod4[R any, P0 any, P1 any, P2 any, P3 any](impl func(ctx context.Context, param0 P0, param1 P1, param2 P2, param3 P3) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		param0 := new(P0)
		param1 := new(P1)
		param2 := new(P2)
		param3 := new(P3)
		code, err := parseParams(ctx, req, param0, param1, param2, param3)
		if err == nil {
			result, err = impl(ctx, *param0, *param1, *param2, *param3)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func RPCMethod5[R any, P0 any, P1 any, P2 any, P3 any, P4 any](impl func(ctx context.Context, param0 P0, param1 P1, param2 P2, param3 P3, param4 P4) (R, error)) RPCHandler {
	return HandlerFunc(func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
		var result R
		param0 := new(P0)
		param1 := new(P1)
		param2 := new(P2)
		param3 := new(P3)
		param4 := new(P4)
		code, err := parseParams(ctx, req, param0, param1, param2, param3, param4)
		if err == nil {
			result, err = impl(ctx, *param0, *param1, *param2, *param3, *param4)
		}
		return mapResponse(ctx, req, result, code, err)
	})
}

func parseParams(ctx context.Context, req *rpcbackend.RPCRequest, params ...interface{}) (rpcbackend.RPCCode, error) {
	if len(req.Params) != len(params) {
		return rpcbackend.RPCCodeInvalidRequest, i18n.NewError(ctx, msgs.MsgJSONRPCIncorrectParamCount, req.Method, len(params), len(req.Params))
	}
	for i := range params {
		b := req.Params[i].Bytes()
		if b == nil {
			b = ([]byte)(`null`)
		}
		if err := json.Unmarshal(b, &params[i]); err != nil {
			return rpcbackend.RPCCodeInvalidRequest, i18n.NewError(ctx, msgs.MsgJSONRPCInvalidParam, req.Method, i, err)
		}
	}
	return 0, nil
}

func mapResponse(ctx context.Context, req *rpcbackend.RPCRequest, result interface{}, code rpcbackend.RPCCode, err error) *rpcbackend.RPCResponse {
	if err == nil {
		b, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			err = i18n.NewError(ctx, msgs.MsgJSONRPCResultSerialization, req.Method, marshalErr)
		} else {
			return &rpcbackend.RPCResponse{
				JSONRpc: "2.0",
				ID:      req.ID,
				Result:  fftypes.JSONAnyPtrBytes(b),
			}
		}
	}
	if code == 0 {
		code = rpcbackend.RPCCodeInternalError
	}
	return rpcbackend.RPCErrorResponse(err, req.ID, code)
}
