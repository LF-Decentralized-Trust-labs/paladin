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

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"google.golang.org/protobuf/encoding/protojson"
	pb "google.golang.org/protobuf/proto"
)

func (tb *testbed) initRPC() error {
	tb.rpcServer.Register(tb.stateStore.RPCModule())
	tb.rpcServer.Register(rpcserver.NewRPCModule("testbed").
		Add("testbed_configureInit", tb.rpcTestbedConfigureInit()),
	)
	return tb.rpcServer.Start()
}

func (tb *testbed) rpcTestbedConfigureInit() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		domainConfig types.RawJSON,
	) (types.RawJSON, error) {
		var req proto.ConfigureDomainRequest
		var res proto.ConfigureDomainResponse
		req.ConfigYaml = string(domainConfig)
		err := tb.syncExchangeToDomain(ctx, string(req.ProtoReflect().Descriptor().FullName()), &req, &res)
		return tb.rpcProtoResponse(&res, err)
	})
}

func (tb *testbed) rpcProtoResponse(res pb.Message, err error) (types.RawJSON, error) {
	if err != nil {
		return nil, err
	}
	b, err := protojson.Marshal(res)
	log.L(tb.ctx).Infof("JSON/RPC response: %s", b)
	return b, err
}
