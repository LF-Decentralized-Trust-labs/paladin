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

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/blockindexer"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

func (tb *testbed) initRPC() error {
	tb.rpcServer.Register(tb.stateStore.RPCModule())
	tb.rpcServer.Register(rpcserver.NewRPCModule("testbed").

		// Deploy a smart contract and get the deployed address
		Add("testbed_deployBytecode", tb.rpcDeployBytecode()).

		// A simulated configure + init step in one synchronous call
		Add("testbed_configureInit", tb.rpcTestbedConfigureInit()).

		// A performs a base ethereum transaction deploy using the
		// simple testbed transaction and key management.
		// Blocks until the Ethereum transaction is successfully confirmed
		// and returns the address emitted by the factory function
		// according to the Paladin spec
		Add("testbed_deploy", tb.rpcTestbedDeploy()),
	)
	return tb.rpcServer.Start()
}

func (tb *testbed) rpcDeployBytecode() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		from string,
		bytecode ethtypes.HexBytes0xPrefix,
	) (*ethtypes.Address0xHex, error) {

		tx, err := tb.simpleTXEstimateSignSubmitAndWait(ctx, from, nil, bytecode)
		if err != nil {
			return nil, err
		}

		return tx.ContractAddress.Address0xHex(), nil
	})
}

func (tb *testbed) rpcTestbedConfigureInit() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		name string,
		domainConfig types.RawJSON,
	) (bool, error) {

		// First we call configure on the domain
		var configRes *proto.ConfigureDomainResponse
		err := syncExchangeToDomain(ctx, tb, &proto.ConfigureDomainRequest{
			Name:       name,
			ConfigYaml: string(domainConfig),
			ChainId:    tb.chainID,
		}, &configRes)
		if err != nil {
			return false, err
		}

		// Then we store all the new domain in the sim registry
		initReq, err := tb.registerDomain(ctx, name, configRes.DomainConfig)
		if err != nil {
			return false, err
		}
		var initRes *proto.InitDomainResponse
		err = syncExchangeToDomain(ctx, tb, initReq, &initRes)
		if err != nil {
			return false, err
		}

		return true, nil
	})
}

func (tb *testbed) rpcTestbedDeploy() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		name string,
		constructorParams types.RawJSON,
	) (*blockindexer.IndexedEvent, error) {

		domain, err := tb.getDomain(name)
		if err != nil {
			return nil, err
		}

		prepareDeployReq, err := tb.validateDeploy(ctx, domain, constructorParams)
		if err != nil {
			return nil, err
		}

		// Prepare the deployment transaction
		var prepareDeployRes *proto.PrepareDeployTransactionResponse
		err = syncExchangeToDomain(ctx, tb, prepareDeployReq, &prepareDeployRes)
		if err != nil {
			return nil, err
		}

		// Do the deploy
		return tb.deployPrivateSmartContract(ctx, domain, prepareDeployRes.Transaction)
	})
}
