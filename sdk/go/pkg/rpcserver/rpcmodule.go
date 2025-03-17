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
	"fmt"
	"sort"
	"strings"
)

type RPCModule struct {
	group   string
	methods map[string]*rpcMethodEntry
}

type rpcMethodType int

const (
	rpcMethodTypeMethod = iota
	rpcMethodTypeAsyncStart
	rpcMethodTypeAsyncLifecycle
)

type rpcMethodEntry struct {
	methodType rpcMethodType
	handler    RPCHandler
	async      RPCAsyncHandler
}

func NewRPCModule(prefix string) *RPCModule {
	return &RPCModule{
		group:   strings.SplitN(prefix, "_", 2)[0],
		methods: map[string]*rpcMethodEntry{},
	}
}

func (m *RPCModule) validateMethod(method string) {
	prefix := m.group + "_"
	if !strings.HasPrefix(method, prefix) {
		panic(fmt.Sprintf("invalid prefix %s (expected=%s)", method, prefix))
	}
	if m.methods[method] != nil {
		panic(fmt.Sprintf("duplicate method: %s", method))
	}
}

// While this rpcserver is generally unopinionated on what is implemented in the JSON/RPC methods,
// (leaving that to other code) it does enforce a convention of "group_function" naming,
// where first segment is a group of functions all implemented by the same module.
//
// This is inspired by strong adoption of this convention in the Ethereum ecosystem, although
// it is not part of the JSON/RPC 2.0 standard.
func (m *RPCModule) Add(method string, handler RPCHandler) *RPCModule {
	m.validateMethod(method)
	m.methods[method] = &rpcMethodEntry{methodType: rpcMethodTypeMethod, handler: handler}
	return m
}

func (m *RPCModule) AddAsync(handler RPCAsyncHandler) *RPCModule {
	startMethod := handler.StartMethod()
	m.validateMethod(startMethod)
	m.methods[startMethod] = &rpcMethodEntry{
		methodType: rpcMethodTypeAsyncStart,
		async:      handler,
	}
	for _, lifecycleMethod := range handler.LifecycleMethods() {
		m.validateMethod(lifecycleMethod)
		m.methods[lifecycleMethod] = &rpcMethodEntry{
			methodType: rpcMethodTypeAsyncLifecycle,
			async:      handler,
		}
	}
	return m
}

func (m *RPCModule) MethodNames() []string {
	names := make([]string, 0, len(m.methods))
	for n := range m.methods {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
