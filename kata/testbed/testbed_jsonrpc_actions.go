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
	"fmt"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/rpcserver"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

func (tb *testbed) initRPC() error {
	tb.rpcServer.Register(tb.stateStore.RPCModule())
	tb.rpcServer.Register(rpcserver.NewRPCModule("testbed").

		// Deploy a smart contract and get the deployed address
		Add("testbed_deployBytecode", tb.rpcDeployBytecode()).

		// A simulated configure + init step in one synchronous call
		Add("testbed_configureInit", tb.rpcTestbedConfigureInit()).

		// Performs a base ethereum transaction deploy using the
		// simple testbed transaction and key management.
		// Blocks until the Ethereum transaction is successfully confirmed
		// and returns the address emitted by the factory function
		// according to the Paladin spec.
		// Domain phases invoked prior to deploy on-chain:
		// - INIT_DEPLOY
		// - PREPARE_DEPLOY
		Add("testbed_deploy", tb.rpcTestbedDeploy()).

		// Performs a privacy preserving smart contract invoke.
		// Selecting the private states required for the transaction,
		// coordinating the required endorsements/signatures,
		// and picking a suitably anonymous signing identity to
		// submit the transaction.
		// Domain phases invoked prior to on-chain transaction:
		// - INIT_TRANSACTION     (sender node)
		// - ASSEMBLE_TRANSACTION (sender node)
		// - PREPARE_TRANSACTION  (submitter node)
		Add("testbed_invoke", tb.rpcTestbedInvoke()),
	)
	return tb.rpcServer.Start()
}

func (tb *testbed) rpcDeployBytecode() rpcserver.RPCHandler {
	return rpcserver.RPCMethod4(func(ctx context.Context,
		from string,
		abi abi.ABI,
		bytecode ethtypes.HexBytes0xPrefix,
		params types.RawJSON,
	) (*ethtypes.Address0xHex, error) {

		var constructor ethclient.ABIFunctionClient
		abic, err := tb.ethClient.ABI(ctx, abi)
		if err == nil {
			constructor, err = abic.Constructor(ctx, bytecode)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to build client for constructor: %s", err)
		}

		var tx *blockindexer.IndexedTransaction
		txHash, err := constructor.R(ctx).
			Signer(from).
			Input(params).
			SignAndSend()
		if err == nil {
			tx, err = tb.blockindexer.WaitForTransaction(ctx, txHash.String())
		}
		if err != nil {
			return nil, fmt.Errorf("failed to send transaction: %s", err)
		}

		return tx.ContractAddress.Address0xHex(), nil
	})
}

func (tb *testbed) rpcTestbedConfigureInit() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		domainName string,
		domainConfig types.RawJSON,
	) (bool, error) {

		// First we call configure on the domain
		var configRes *proto.ConfigureDomainResponse
		err := syncExchangeToDomain(ctx, tb, &proto.ConfigureDomainRequest{
			Name:       domainName,
			ConfigYaml: string(domainConfig),
			ChainId:    tb.ethClient.ChainID(),
		}, &configRes)
		if err != nil {
			return false, err
		}

		// Then we store all the new domain in the sim registry
		initReq, err := tb.registerDomain(ctx, domainName, configRes.DomainConfig)
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
		domainName string,
		constructorParams types.RawJSON,
	) (*blockindexer.IndexedEvent, error) {

		domain, err := tb.getDomain(domainName)
		if err != nil {
			return nil, err
		}

		deployTXSpec, err := tb.validateDeploy(ctx, domain, constructorParams)
		if err != nil {
			return nil, err
		}

		// Init the deployment transaction
		var initDeployRes *proto.InitDeployTransactionResponse
		err = syncExchangeToDomain(ctx, tb, &proto.InitDeployTransactionRequest{
			Transaction: deployTXSpec,
		}, &initDeployRes)
		if err != nil {
			return nil, err
		}

		// Resolve all the addresses locally in the testbed
		prepareReq := &proto.PrepareDeployTransactionRequest{
			Transaction: deployTXSpec,
			Verifiers:   make([]*proto.ResolvedVerifier, len(initDeployRes.RequiredVerifiers)),
		}
		for i, v := range initDeployRes.RequiredVerifiers {
			_, verifier, err := tb.keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
			}
			prepareReq.Verifiers[i] = &proto.ResolvedVerifier{
				Lookup:    v.Lookup,
				Algorithm: v.Algorithm,
				Verifier:  verifier,
			}
		}

		// Prepare the deployment transaction
		var prepareDeployRes *proto.PrepareDeployTransactionResponse
		err = syncExchangeToDomain(ctx, tb, prepareReq, &prepareDeployRes)
		if err != nil {
			return nil, err
		}

		// Do the deploy
		return tb.deployPrivateSmartContract(ctx, domain, prepareDeployRes.Transaction)
	})
}

func (tb *testbed) rpcTestbedInvoke() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		invocation types.PrivateContractInvoke,
		inputs types.RawJSON,
	) (*blockindexer.IndexedEvent, error) {
		return nil, fmt.Errorf("TODO")
	})
}
