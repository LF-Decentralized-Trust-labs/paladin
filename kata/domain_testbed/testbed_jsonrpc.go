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

package main

import (
	"context"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
)

func (tb *testbed) initRPC() {
	tb.rpcServer.Register(tb.stateStore.RPCModule())
	tb.rpcServer.Register(rpcserver.NewRPCModule("testbed").
		Add("testbed_simulateDeploy", tb.rpcTestbedSimulateTransaction()),
	)
}

func (tb *testbed) rpcTestbedSimulateTransaction() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		domain string,
		function abi.Entry,
	) (string, error) {
		return "", nil
	})
}
