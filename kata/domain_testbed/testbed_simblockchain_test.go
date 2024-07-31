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
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/httpserver"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/stretchr/testify/assert"
)

// simBlockchain just confirms transactions immediately, and emits events when told
// Allows unit tests to validate the testbed function (while the testbed speaks proper
// JSON/RPC TX to the underlying chain)
type simBlockchain struct {
	rpcServer rpcserver.Server
}

func newSimBlockchain(t *testing.T) (*simBlockchain, func()) {
	rs, err := rpcserver.NewServer(context.Background(), &rpcserver.Config{
		HTTP: rpcserver.HTTPEndpointConfig{Disabled: true},
		WS: rpcserver.WSEndpointConfig{
			Config: httpserver.Config{
				Port: confutil.P(0),
			},
		},
	})
	assert.NoError(t, err)
	rs.Register(
		rpcserver.NewRPCModule("eth").
			Add("eth_subscribe", func(ctx context.Context, req *rpcbackend.RPCRequest) *rpcbackend.RPCResponse {
				// Just mock this up, as we send to everyone
			}),
	)
	sb := &simBlockchain{}
	return sb, func() {
		sb.rpcServer.Stop()
	}
}
