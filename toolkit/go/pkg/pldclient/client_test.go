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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRPCServerHTTP(t *testing.T, conf *pldconf.RPCServerConfig) (string, rpcserver.RPCServer) {
	conf.HTTP.Address = confutil.P("127.0.0.1")
	conf.HTTP.Port = confutil.P(0)
	conf.HTTP.ShutdownTimeout = confutil.P("0s")
	conf.WS.Disabled = true
	rpcServer, err := rpcserver.NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = rpcServer.Start()
	require.NoError(t, err)
	return fmt.Sprintf("http://%s", rpcServer.HTTPAddr()), rpcServer
}

func newTestClientAndServerHTTP(t *testing.T) (context.Context, PaladinClient, rpcserver.RPCServer, func()) {

	ctx := context.Background()
	url, rpcServer := newTestRPCServerHTTP(t, &pldconf.RPCServerConfig{})
	c, err := New().HTTP(context.Background(), &pldconf.HTTPClientConfig{
		URL: url,
	})
	require.NoError(t, err)

	return ctx, c, rpcServer, rpcServer.Stop

}

func newTestRPCServerWebSockets(t *testing.T, conf *pldconf.RPCServerConfig) (string, rpcserver.RPCServer) {
	conf.WS.Address = confutil.P("127.0.0.1")
	conf.WS.Port = confutil.P(0)
	conf.WS.ShutdownTimeout = confutil.P("0s")
	conf.HTTP.Disabled = true
	rpcServer, err := rpcserver.NewRPCServer(context.Background(), conf)
	require.NoError(t, err)
	err = rpcServer.Start()
	require.NoError(t, err)
	return fmt.Sprintf("ws://%s", rpcServer.WSAddr()), rpcServer
}

func newTestClientAndServerWebSockets(t *testing.T) (context.Context, PaladinClient, rpcserver.RPCServer, func()) {

	ctx := context.Background()
	url, rpcServer := newTestRPCServerWebSockets(t, &pldconf.RPCServerConfig{})
	c, err := New().WebSocket(context.Background(), &pldconf.WSClientConfig{
		HTTPClientConfig: pldconf.HTTPClientConfig{
			URL: url,
		},
	})
	require.NoError(t, err)

	return ctx, c, rpcServer, func() {
		c.Close()
		rpcServer.Stop()
	}

}

func TestHTTPBadConfig(t *testing.T) {
	_, err := New().HTTP(context.Background(), &pldconf.HTTPClientConfig{})
	require.Regexp(t, "PD020501", err)
}

func TestWSBadConfig(t *testing.T) {
	_, err := New().WebSocket(context.Background(), &pldconf.WSClientConfig{})
	require.Regexp(t, "PD020500", err)
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
	ctx, c, _, done := newTestClientAndServerHTTP(t)
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
