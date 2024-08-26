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

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
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
		err := syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, &proto.ConfigureDomainRequest{
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
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, initReq, &initRes)
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
	) (*ethtypes.Address0xHex, error) {

		domain, err := tb.getDomainByName(domainName)
		if err != nil {
			return nil, err
		}

		txID, deployTXSpec, err := domain.validateDeploy(ctx, constructorParams)
		if err != nil {
			return nil, err
		}
		waiter := domain.txWaiter(ctx, *txID)
		defer waiter.cancel()

		// Init the deployment transaction
		var initDeployRes *proto.InitDeployTransactionResponse
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, &proto.InitDeployTransactionRequest{
			Transaction: deployTXSpec,
		}, &initDeployRes)
		if err != nil {
			return nil, err
		}

		// Resolve all the addresses locally in the testbed
		prepareReq := &proto.PrepareDeployTransactionRequest{
			Transaction:       deployTXSpec,
			ResolvedVerifiers: make([]*proto.ResolvedVerifier, len(initDeployRes.RequiredVerifiers)),
		}
		for i, v := range initDeployRes.RequiredVerifiers {
			_, verifier, err := tb.keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
			}
			prepareReq.ResolvedVerifiers[i] = &proto.ResolvedVerifier{
				Lookup:    v.Lookup,
				Algorithm: v.Algorithm,
				Verifier:  verifier,
			}
		}

		// Prepare the deployment transaction
		var prepareDeployRes *proto.PrepareDeployTransactionResponse
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, prepareReq, &prepareDeployRes)
		if err != nil {
			return nil, err
		}

		// Do the deploy - we wait for the transaction here to cover revert failures
		if prepareDeployRes.Deploy != nil && prepareDeployRes.Transaction == nil {
			err = tb.execBaseLedgerDeployTransaction(ctx, domain.factoryContractABI, prepareDeployRes.SigningAddress, prepareDeployRes.Deploy)
		} else if prepareDeployRes.Transaction != nil && prepareDeployRes.Deploy == nil {
			err = tb.execBaseLedgerTransaction(ctx, domain.factoryContractABI, domain.factoryContractAddress, prepareDeployRes.SigningAddress, prepareDeployRes.Transaction)
		} else {
			err = fmt.Errorf("must return a transaction to invoke, or a transaction to deploy")
		}
		if err != nil {
			return nil, err
		}

		// Rather than just inspecting the TX - we wait for the domain to index the event, such that
		// we know it's in the map by the time we return.
		psc, err := waiter.wait(ctx)
		if err != nil {
			return nil, err
		}
		return psc.address, nil
	})
}

func (tb *testbed) rpcTestbedInvoke() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		invocation types.PrivateContractInvoke,
	) (bool, error) {

		/// TODO: Work out testbed simulation of sequence
		seq := uuid.New()

		psc := tb.getDomainContract(invocation.To.Address0xHex())
		if psc == nil {
			return false, fmt.Errorf("smart contract %s unknown", &invocation.To)
		}

		txID, txSpec, err := psc.validateInvoke(ctx, &invocation)
		if err != nil {
			return false, err
		}
		waiter := psc.domain.txWaiter(ctx, *txID)
		defer waiter.cancel()

		// First we call init on the smart contract to:
		// - validate the transaction ABI is understood by the contract
		// - get an initial list of verifiers that need to be resolved
		var initTXRes *proto.InitTransactionResponse
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, &proto.InitTransactionRequest{
			Transaction: txSpec,
		}, &initTXRes)
		if err != nil {
			return false, err
		}

		// Gather the addresses - in the testbed we assume these all to be local
		assembleReq := &proto.AssembleTransactionRequest{
			Transaction:       txSpec,
			ResolvedVerifiers: make([]*proto.ResolvedVerifier, len(initTXRes.RequiredVerifiers)),
		}
		for i, v := range initTXRes.RequiredVerifiers {
			_, verifier, err := tb.keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm)
			if err != nil {
				return false, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
			}
			assembleReq.ResolvedVerifiers[i] = &proto.ResolvedVerifier{
				Lookup:    v.Lookup,
				Algorithm: v.Algorithm,
				Verifier:  verifier,
			}
		}

		// Now call assemble
		var assembleTXRes *proto.AssembleTransactionResponse
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, assembleReq, &assembleTXRes)
		if err != nil {
			return false, err
		}

		// The testbed only handles the OK result
		switch assembleTXRes.AssemblyResult {
		case proto.AssembleTransactionResponse_OK:
		default:
			return false, fmt.Errorf("assemble result was %s", assembleTXRes.AssemblyResult)
		}

		// Validate and write the states
		spentStateRefs := assembleTXRes.AssembledTransaction.SpentStates
		newStates, _, err := psc.validateAndWriteStates(seq, assembleTXRes.AssembledTransaction.NewStates)
		if err != nil {
			return false, err
		}

		// Gather signatures
		attestations := []*proto.AttestationResult{}
		signatures, err := psc.gatherSignatures(ctx, assembleTXRes.AttestationPlan)
		if err != nil {
			return false, err
		}
		attestations = append(attestations, signatures...)

		// Gather endorsements - now logically this would be a distributed activity across nodes
		endorserSubmitConstraint, inputStates, endorsements, err :=
			psc.gatherEndorsements(ctx, txSpec, signatures, spentStateRefs, newStates, assembleReq.ResolvedVerifiers, assembleTXRes.AttestationPlan)
		if err != nil {
			return false, err
		}
		attestations = append(attestations, endorsements...)

		log.L(ctx).Infof("Assembled and endorsed inputs=%d outputs=%d signatures=%d endorsements=%d", len(inputStates), len(newStates), len(signatures), len(endorsements))

		// Pick the signer for the base ledger transaction - now logically we're picking which node would do the prepare + submit phases
		signer, err := psc.determineSubmitterIdentity(txSpec, endorserSubmitConstraint, endorsements)
		if err != nil {
			return false, err
		}

		finalSpentStates := make([]*proto.EndorsableState, len(inputStates))
		for i, state := range inputStates {
			finalSpentStates[i] = &proto.EndorsableState{
				SchemaId:      state.Schema.String(),
				HashId:        state.ID.String(),
				StateDataJson: state.Data.String(),
			}
		}
		finalNewStates := make([]*proto.EndorsableState, len(newStates))
		for i, state := range newStates {
			finalNewStates[i] = &proto.EndorsableState{
				SchemaId:      state.Schema.String(),
				HashId:        state.ID.String(),
				StateDataJson: state.Data.String(),
			}
		}

		// Prepare the transaction
		prepareTXReq := &proto.PrepareTransactionRequest{
			Transaction: txSpec,
			FinalizedTransaction: &proto.FinalizedTransaction{
				TransactionId: txSpec.TransactionId,
				SpentStates:   finalSpentStates,
				NewStates:     finalNewStates,
			},
			AttestationResult: attestations,
		}
		var prepareTXRes *proto.PrepareTransactionResponse
		err = syncExchange(ctx, tb, tb.destToDomain, tb.destFromDomain, prepareTXReq, &prepareTXRes)
		if err != nil {
			return false, err
		}

		err = tb.execBaseLedgerTransaction(ctx, psc.domain.privateContractABI, psc.address, signer, prepareTXRes.Transaction)
		if err != nil {
			return false, err
		}

		// TODO: state confirmation by TXID
		return true, nil
	})
}
