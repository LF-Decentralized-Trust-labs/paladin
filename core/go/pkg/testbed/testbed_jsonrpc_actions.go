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

package testbed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

func (tb *testbed) initRPC() {
	tb.rpcModule = rpcserver.NewRPCModule("testbed").

		// Deploy a smart contract and get the deployed address
		Add("testbed_listDomains", tb.rpcListDomains()).

		// Deploy a smart contract and get the deployed address
		Add("testbed_deployBytecode", tb.rpcDeployBytecode()).

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
		Add("testbed_invoke", tb.rpcTestbedInvoke()).

		// Prepares a privacy preserving smart contract invocation, but
		// does not actually invoke.
		// Returns an ABI encoded function call.
		Add("testbed_prepare", tb.rpcTestbedPrepare()).

		// Performs identity resolution (which in the case of the testbed is just local identities)
		Add("testbed_resolveVerifier", tb.rpcResolveVerifier())
}

func (tb *testbed) rpcListDomains() rpcserver.RPCHandler {
	return rpcserver.RPCMethod0(func(ctx context.Context) ([]string, error) {
		res := []string{}
		for name := range tb.c.DomainManager().ConfiguredDomains() {
			res = append(res, name)
		}
		return res, nil
	})
}

func (tb *testbed) rpcDeployBytecode() rpcserver.RPCHandler {
	return rpcserver.RPCMethod4(func(ctx context.Context,
		from string,
		abi abi.ABI,
		bytecode tktypes.HexBytes,
		params tktypes.RawJSON,
	) (*ethtypes.Address0xHex, error) {

		var constructor ethclient.ABIFunctionClient
		ec := tb.c.EthClientFactory().HTTPClient()
		abic, err := ec.ABI(ctx, abi)
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
			tx, err = tb.c.BlockIndexer().WaitForTransactionSuccess(ctx, *txHash, abi)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to send transaction: %s", err)
		}

		return tx.ContractAddress.Address0xHex(), nil
	})
}

func (tb *testbed) rpcTestbedDeploy() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		domainName string,
		constructorParams tktypes.RawJSON,
	) (*tktypes.EthAddress, error) {

		domain, err := tb.c.DomainManager().GetDomainByName(ctx, domainName)
		if err != nil {
			return nil, err
		}

		tx := &components.PrivateContractDeploy{
			ID:     uuid.New(),
			Domain: domain.Name(),
			Inputs: constructorParams,
		}
		err = domain.InitDeploy(ctx, tx)
		if err != nil {
			return nil, err
		}

		keyMgr := tb.c.KeyManager()
		tx.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.RequiredVerifiers))
		for i, v := range tx.RequiredVerifiers {
			_, verifier, err := keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm, v.VerifierType)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
			}
			tx.Verifiers[i] = &prototk.ResolvedVerifier{
				Lookup:       v.Lookup,
				Algorithm:    v.Algorithm,
				Verifier:     verifier,
				VerifierType: v.VerifierType,
			}
		}

		// Prepare the deployment transaction
		err = domain.PrepareDeploy(ctx, tx)
		if err != nil {
			return nil, err
		}

		// Do the deploy - we wait for the transaction here to cover revert failures
		if tx.DeployTransaction != nil && tx.InvokeTransaction == nil {
			err = tb.execBaseLedgerDeployTransaction(ctx, tx.Signer, tx.DeployTransaction)
		} else if tx.InvokeTransaction != nil && tx.DeployTransaction == nil {
			err = tb.execBaseLedgerTransaction(ctx, tx.Signer, tx.InvokeTransaction)
		} else {
			err = fmt.Errorf("must return a transaction to invoke, or a transaction to deploy")
		}
		if err != nil {
			return nil, err
		}

		// Rather than just inspecting the TX - we wait for the domain to index the event, such that
		// we know it's in the map by the time we return.
		psc, err := tb.c.DomainManager().WaitForDeploy(ctx, tx.ID)
		if err != nil {
			return nil, err
		}
		addr := psc.Address()
		return &addr, nil
	})
}

func (tb *testbed) prepareTransaction(ctx context.Context, invocation tktypes.PrivateContractInvoke, intent prototk.TransactionSpecification_Intent) (*components.PrivateTransaction, error) {
	psc, err := tb.c.DomainManager().GetSmartContractByAddress(ctx, invocation.To)
	if err != nil {
		return nil, err
	}

	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Function: &invocation.Function,
			Domain:   psc.Domain().Name(),
			From:     invocation.From,
			To:       psc.Address(),
			Inputs:   invocation.Inputs,
			Intent:   intent,
		},
	}

	// First we call init on the smart contract to:
	// - validate the transaction ABI is understood by the contract
	// - get an initial list of verifiers that need to be resolved
	if err := psc.InitTransaction(ctx, tx); err != nil {
		return nil, err
	}

	// Gather the addresses - in the testbed we assume these all to be local
	keyMgr := tb.c.KeyManager()
	tx.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.PreAssembly.RequiredVerifiers))
	for i, v := range tx.PreAssembly.RequiredVerifiers {
		_, verifier, err := keyMgr.ResolveKey(ctx, v.Lookup, v.Algorithm, v.VerifierType)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
		}
		tx.PreAssembly.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			Verifier:     verifier,
			VerifierType: v.VerifierType,
		}
	}

	// Now call assemble
	if err := psc.AssembleTransaction(ctx, tx); err != nil {
		return nil, err
	}

	// The testbed only handles the OK result
	switch tx.PostAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
	default:
		return nil, fmt.Errorf("assemble result was %s", tx.PostAssembly.AssemblyResult)
	}

	// The testbed always chooses to take the assemble output and progress to endorse
	// (no complex sequence selection routine that might result in abandonment).
	// So just write the states
	if err := psc.WritePotentialStates(ctx, tx); err != nil {
		return nil, err
	}

	// Gather signatures
	if err = tb.gatherSignatures(ctx, tx); err != nil {
		return nil, err
	}

	// Gather endorsements (this would be a distributed activity across nodes in the real engine)
	if err := tb.gatherEndorsements(ctx, psc, tx); err != nil {
		return nil, err
	}

	log.L(ctx).Infof("Assembled and endorsed inputs=%d outputs=%d signatures=%d endorsements=%d",
		len(tx.PostAssembly.InputStates), len(tx.PostAssembly.OutputStates), len(tx.PostAssembly.Signatures), len(tx.PostAssembly.Endorsements))

	// Pick the signer for the base ledger transaction - now logically we're picking which node would do the prepare + submit phases
	if err := psc.ResolveDispatch(ctx, tx); err != nil {
		return nil, err
	}

	// Prepare the transaction
	if err := psc.PrepareTransaction(ctx, tx); err != nil {
		return nil, err
	}

	return tx, nil
}

func (tb *testbed) mapTransaction(tx *components.PrivateTransaction) (*tktypes.PrivateContractTransaction, error) {
	inputStates := make([]*tktypes.FullState, len(tx.PostAssembly.InputStates))
	for i, state := range tx.PostAssembly.InputStates {
		inputStates[i] = &tktypes.FullState{
			ID:     state.ID,
			Schema: state.Schema,
			Data:   []byte(state.Data),
		}
	}
	outputStates := make([]*tktypes.FullState, len(tx.PostAssembly.OutputStates))
	for i, state := range tx.PostAssembly.OutputStates {
		outputStates[i] = &tktypes.FullState{
			ID:     state.ID,
			Schema: state.Schema,
			Data:   []byte(state.Data),
		}
	}
	readStates := make([]*tktypes.FullState, len(tx.PostAssembly.ReadStates))
	for i, state := range tx.PostAssembly.ReadStates {
		readStates[i] = &tktypes.FullState{
			ID:     state.ID,
			Schema: state.Schema,
			Data:   []byte(state.Data),
		}
	}
	paramsJSON, err := tx.PreparedTransaction.Inputs.JSON()
	if err != nil {
		return nil, err
	}
	return &tktypes.PrivateContractTransaction{
		FunctionABI:  tx.PreparedTransaction.FunctionABI,
		To:           tx.PreparedTransaction.To,
		ParamsJSON:   paramsJSON,
		InputStates:  inputStates,
		OutputStates: outputStates,
		ReadStates:   readStates,
		ExtraData:    tx.PostAssembly.ExtraData,
	}, nil
}

func (tb *testbed) rpcTestbedInvoke() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		invocation tktypes.PrivateContractInvoke,
		waitForCompletion bool,
	) (*tktypes.PrivateContractTransaction, error) {

		tx, err := tb.prepareTransaction(ctx, invocation, prototk.TransactionSpecification_SUBMIT)
		if err != nil {
			return nil, err
		}
		err = tb.execBaseLedgerTransaction(ctx, tx.Signer, tx.PreparedTransaction)
		if err != nil {
			return nil, err
		}

		// Wait for the domain to index the transaction events
		if waitForCompletion {
			err = tb.c.DomainManager().WaitForTransaction(ctx, tx.ID)
			if err != nil {
				return nil, err
			}
		}

		return tb.mapTransaction(tx)
	})
}

func (tb *testbed) rpcTestbedPrepare() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		invocation tktypes.PrivateContractInvoke,
	) (*tktypes.PrivateContractTransaction, error) {

		tx, err := tb.prepareTransaction(ctx, invocation, prototk.TransactionSpecification_PREPARE)
		if err != nil {
			return nil, err
		}
		return tb.mapTransaction(tx)
	})
}

func (tb *testbed) rpcResolveVerifier() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		lookup string,
		algorithm string,
		verifierType string,
	) (verifier string, _ error) {
		identifier, err := tktypes.PrivateIdentityLocator(lookup).Identity(ctx)
		if err == nil {
			_, verifier, err = tb.c.KeyManager().ResolveKey(ctx, identifier, algorithm, verifierType)
		}
		if err != nil {
			return "", err
		}
		return verifier, err
	})
}
