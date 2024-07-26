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

	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

func (tb *testbed) initRPC() error {
	tb.rpcServer.Register(tb.stateStore.RPCModule())
	tb.rpcServer.Register(rpcserver.NewRPCModule("testbed").
		Add("testbed_configureInit", tb.rpcTestbedConfigureInit()),
	)
	return tb.rpcServer.Start()
}

func (tb *testbed) rpcTestbedConfigureInit() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		name string,
		domainConfig types.RawJSON,
	) (bool, error) {

		// First we call configure on the domain
		var configRes proto.ConfigureDomainResponse
		err := tb.syncExchangeToDomain(ctx, &proto.ConfigureDomainRequest{
			Name:       name,
			ConfigYaml: string(domainConfig),
			ChainId:    1122334455, // TODO: Get from Besu
		}, &configRes)
		if err != nil {
			return false, err
		}

		// Then we store all the new domain in the sim registry
		initReq, err := tb.registerDomain(ctx, name, configRes.DomainConfig)
		if err != nil {
			return false, err
		}
		var initRes proto.ConfigureDomainResponse
		err = tb.syncExchangeToDomain(ctx, initReq, &initRes)
		if err != nil {
			return false, err
		}

		return true, nil
	})
}
