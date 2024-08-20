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
	"strings"
)

type RPCModule struct {
	group   string
	methods map[string]RPCHandler
}

func NewRPCModule(prefix string) *RPCModule {
	return &RPCModule{
		group:   strings.SplitN(prefix, "_", 2)[0],
		methods: map[string]RPCHandler{},
	}
}

// While this rpcserver is generally unopinionated on what is implemented in the JSON/RPC methods,
// (leaving that to other code) it does enforce a convention of "group_function" naming,
// where first segment is a group of functions all implemented by the same module.
//
// This is inspired by strong adoption of this convention in the Ethereum ecosystem, although
// it is not part of the JSON/RPC 2.0 standard.
func (m *RPCModule) Add(method string, handler RPCHandler) *RPCModule {
	prefix := m.group + "_"
	if !strings.HasPrefix(method, prefix) {
		panic(fmt.Sprintf("invalid prefix %s (expected=%s)", method, prefix))
	}
	if m.methods[method] != nil {
		panic(fmt.Sprintf("duplicate method: %s", method))
	}
	m.methods[method] = handler
	return m
}
