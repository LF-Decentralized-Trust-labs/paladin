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

package pldclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/wsclient"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRPCHandler func(rpcReq *rpcclient.RPCRequest) (int, *rpcclient.RPCResponse)

type testRPCMethod struct {
	name    string
	handler testRPCHandler
}

func errorResponse(id pldtypes.RawJSON, err error) (int, *rpcclient.RPCResponse) {
	return 500, &rpcclient.RPCResponse{
		JSONRpc: "2.0",
		ID:      id,
		Error: &rpcclient.RPCError{
			Code:    123,
			Message: err.Error(),
		},
	}
}

func successResponse(id pldtypes.RawJSON, result pldtypes.RawJSON) (int, *rpcclient.RPCResponse) {
	return 200, &rpcclient.RPCResponse{
		JSONRpc: "2.0",
		ID:      id,
		Result:  result,
	}
}

func newTestRPCServerHTTP(t *testing.T, methods ...testRPCMethod) (ctx context.Context, url string, done func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var rpcReq *rpcclient.RPCRequest
		err := json.NewDecoder(r.Body).Decode(&rpcReq)
		assert.NoError(t, err)

		var status int
		var rpcRes *rpcclient.RPCResponse
		for _, method := range methods {
			if method.name == rpcReq.Method {
				status, rpcRes = method.handler(rpcReq)
				break
			}
		}
		if rpcRes == nil {
			err := i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnsupportedMethod, rpcReq.Method)
			rpcRes = rpcclient.NewRPCErrorResponse(err, rpcReq.ID, rpcclient.RPCCodeInvalidRequest)
			status = 400
		}
		require.NotNil(t, rpcRes, "No handler for method %s", rpcReq.Method)

		b := []byte(`{}`)
		if rpcRes != nil {
			b, err = json.Marshal(rpcRes)
			assert.NoError(t, err)
		}
		w.Header().Add("Content-Type", "application/json")
		w.Header().Add("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(status)
		_, _ = w.Write(b)

	}))

	return ctx, fmt.Sprintf("http://%s", server.Listener.Addr()), func() {
		cancelCtx()
		server.Close()
	}
}

func newTestClientAndServerHTTP(t *testing.T, methods ...testRPCMethod) (ctx context.Context, client PaladinClient, done func()) {
	ctx, url, done := newTestRPCServerHTTP(t, methods...)
	c, err := New().HTTP(context.Background(), &pldconf.HTTPClientConfig{
		URL: url,
	})
	require.NoError(t, err)

	return ctx, c, done
}

func newTestRPCServerWebSockets(t *testing.T, methods ...testRPCMethod) (ctx context.Context, url string, done func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	toServer, fromServer, url, close := wsclient.NewTestWSServer(func(req *http.Request) {})

	wg.Add(1)
	go func() {
		defer wg.Done()
		listen := true
		for listen {
			select {
			case msg, ok := <-toServer:
				if !ok {
					listen = false
					continue
				}
				var req rpcclient.RPCRequest
				err := json.Unmarshal([]byte(msg), &req)
				require.NoError(t, err)

				var res *rpcclient.RPCResponse
				for _, method := range methods {
					if method.name == req.Method {
						_, res = method.handler(&req)
						break
					}
				}
				if res == nil {
					err := i18n.NewError(ctx, pldmsgs.MsgJSONRPCUnsupportedMethod, req.Method)
					res = rpcclient.NewRPCErrorResponse(err, req.ID, rpcclient.RPCCodeInvalidRequest)
				}
				b, err := json.Marshal(res)
				require.NoError(t, err)
				select {
				case fromServer <- string(b):
				case <-ctx.Done():
					listen = false
				}

			case <-ctx.Done():
				listen = false
			}
		}
	}()

	return ctx, url, func() {
		cancelCtx()
		wg.Wait()
		close()
	}
}

func newTestClientAndServerWebSockets(t *testing.T, methods ...testRPCMethod) (ctx context.Context, client PaladinClient, done func()) {
	ctx, url, close := newTestRPCServerWebSockets(t, methods...)
	c, err := New().WebSocket(context.Background(), &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{
			URL: url,
		},
	})
	require.NoError(t, err)

	return ctx, c, func() {
		c.Close()
		close()
	}
}

func TestWrapRestyClient(t *testing.T) {
	WrapRestyClient(resty.New())
}

func TestHTTPBadConfig(t *testing.T) {
	_, err := New().HTTP(context.Background(), &pldconf.HTTPClientConfig{})
	require.Regexp(t, "PD020501", err)
}

func TestWSBadConfig(t *testing.T) {
	_, err := New().WebSocket(context.Background(), &pldconf.WSClientConfig{})
	require.Regexp(t, "PD021100", err)
}

func TestInfoNotFoundNil(t *testing.T) {
	require.Nil(t, (&rpcModuleInfo{methodInfo: map[string]RPCMethodInfo{}}).MethodInfo("unknown"))
}

// The JSON/RPC client tests do not validate the implementation of the JSON/RPC function in the server,
// instead they validate that each function we declare on the method info:
// - Is implemented with function naming exactly according to the standards
// - Declares the right number of inputs and outputs on the function
// - When invoked, it invokes the JSON/RPC method it says it does
//
// There is a lot of synergy with the reflect code here, and the reflect code in the reference package that
// depends on this to build the documentation correctly.
func testRPCModule(t *testing.T, getMod func(c PaladinClient) RPCModule) {
	ctx, c, done := newTestClientAndServerHTTP(t)
	defer done()

	mod := getMod(c)
	apiGroupType := reflect.TypeOf(mod)
	groupPrefix := mod.Group()

	for _, methodName := range mod.Methods() {

		requiredFnName, hasPrefix := strings.CutPrefix(methodName, groupPrefix+"_")
		require.True(t, hasPrefix, "RPC method '%s' does not start with prefix %s", requiredFnName, groupPrefix)
		requiredFnName = strings.ToUpper(requiredFnName[0:1]) + requiredFnName[1:]

		var method *reflect.Method
		for i := 0; i < apiGroupType.NumMethod(); i++ {
			m := apiGroupType.Method(i)
			if m.Name == requiredFnName {
				method = &m
			}
		}
		require.NotNil(t, method, "Implementation method '%s' for RPC method '%s' does not exist on interface %T", requiredFnName, methodName, mod)

		methodType := method.Type
		info := mod.MethodInfo(methodName)
		require.Equal(t, len(info.Inputs), methodType.NumIn()-2 /* fn pointer, and ctx */, "Implementation method '%s' info declares it has %d parameters, excluding the 'ctx' parameter: %v", method.Name, len(info.Inputs), info.Inputs)
		for i, inputName := range info.Inputs {
			require.NotEmpty(t, inputName, "input %d on function %s must be named", i, method.Name)
		}
		require.Equal(t, 2, methodType.NumOut(), "Implementation method '%s' must have one output, plus the error", method.Name)
		require.NotEmpty(t, info.Output, "output on function %s must be named", method.Name)

		// invoke the method, and check it does the thing it says it does
		inputs := make([]reflect.Value, methodType.NumIn())
		for i := range inputs {
			switch i {
			case 0:
				inputs[i] = reflect.ValueOf(mod)
			case 1:
				inputs[i] = reflect.ValueOf(ctx)
			default:
				inputs[i] = reflect.New(methodType.In(i)).Elem()
			}
		}
		outputs := method.Func.Call(inputs)
		err := outputs[methodType.NumOut()-1].Interface().(error)

		// This error proves:
		// 1. We did a JSON/RPC call
		// 2. We called the function we declared we would in the info
		assert.Regexp(t, fmt.Sprintf("PD020702.*%s$", methodName), err)

	}
}
