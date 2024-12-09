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
	"strings"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
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
		Add("testbed_resolveVerifier", tb.rpcResolveVerifier()).

		// Performs a call directly against the domain
		Add("testbed_call", tb.rpcTestbedCall())

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

		receipt, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type: pldapi.TransactionTypePublic.Enum(),
				From: from,
				Data: params,
			},
			ABI:      abi,
			Bytecode: tktypes.HexBytes(bytecode),
		})
		if err != nil {
			return nil, err
		}
		return receipt.ContractAddress.Address0xHex(), nil
	})
}

func (tb *testbed) resolveVerifiers(ctx context.Context, requiredVerifiers []*prototk.ResolveVerifierRequest) ([]*prototk.ResolvedVerifier, error) {
	verifiers := make([]*prototk.ResolvedVerifier, len(requiredVerifiers))
	for i, v := range requiredVerifiers {
		resolvedKey, err := tb.ResolveKey(ctx, v.Lookup, v.Algorithm, v.VerifierType)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
		}
		verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			Verifier:     resolvedKey.Verifier.Verifier,
			VerifierType: v.VerifierType,
		}
	}
	return verifiers, nil
}

func (tb *testbed) rpcTestbedDeploy() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		domainName string,
		from string,
		constructorParams tktypes.RawJSON,
	) (*tktypes.EthAddress, error) {

		domain, err := tb.c.DomainManager().GetDomainByName(ctx, domainName)
		if err != nil {
			return nil, err
		}

		tx := &components.PrivateContractDeploy{
			ID:     uuid.New(),
			Domain: domain.Name(),
			From:   from,
			Inputs: constructorParams,
		}
		err = domain.InitDeploy(ctx, tx)
		if err != nil {
			return nil, err
		}

		if tx.Verifiers, err = tb.resolveVerifiers(ctx, tx.RequiredVerifiers); err != nil {
			return nil, err
		}

		// Prepare the deployment transaction
		err = domain.PrepareDeploy(ctx, tx)
		if err != nil {
			return nil, err
		}

		// Rather than just inspecting the TX - we wait for the domain to index the event, such that
		// we know it's in the map by the time we return.
		psc, err := tb.c.DomainManager().ExecDeployAndWait(ctx, tx.ID, func() error {
			// Do the deploy - we wait for the transaction here to cover revert failures
			if tx.DeployTransaction != nil && tx.InvokeTransaction == nil {
				_, err = tb.execBaseLedgerDeployTransaction(ctx, tx.Signer, tx.DeployTransaction)
			} else if tx.InvokeTransaction != nil && tx.DeployTransaction == nil {
				_, err = tb.execBaseLedgerTransaction(ctx, tx.Signer, tx.InvokeTransaction)
			} else {
				err = fmt.Errorf("must return a transaction to invoke, or a transaction to deploy")
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		addr := psc.Address()
		return &addr, nil
	})
}

func (tb *testbed) newPrivateTransaction(ctx context.Context, invocation *pldapi.TransactionInput, intent prototk.TransactionSpecification_Intent) (components.DomainSmartContract, *components.PrivateTransaction, error) {
	psc, err := tb.c.DomainManager().GetSmartContractByAddress(ctx, tb.c.Persistence().DB(), *invocation.To)
	if err != nil {
		return nil, nil, err
	}

	fn, err := tb.resolveFunction(invocation)
	if err != nil {
		return nil, nil, err
	}

	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Function: fn,
			Domain:   psc.Domain().Name(),
			From:     invocation.From,
			To:       psc.Address(),
			Inputs:   invocation.Data,
			Intent:   intent,
		},
	}
	return psc, tx, err
}

// Very simplified version of the real logic in TX manager
func (tb *testbed) resolveFunction(invocation *pldapi.TransactionInput) (*abi.Entry, error) {
	if invocation.ABIReference != nil {
		return nil, fmt.Errorf("Testbed does not support ABIReference")
	}
	if invocation.ABI == nil {
		return nil, fmt.Errorf("Testbed requires ABI to be passed in on each call")
	}
	for _, entry := range invocation.ABI {
		if entry.Name == invocation.Function {
			return entry, nil
		}
	}
	if invocation.Function == "" && len(invocation.ABI) == 1 {
		return invocation.ABI[0], nil
	}
	return nil, fmt.Errorf("Could not find function '%s' in provided ABI", invocation.Function)
}

func (tb *testbed) resolveTXSigner(tx *components.PrivateTransaction) error {
	// The testbed implements much simpler checking here than the full private TX manager
	// on whether the ENDORSER_MUST_SUBMIT constraint clashes with the config on the contract.
	for _, ar := range tx.PostAssembly.Endorsements {
		for _, c := range ar.Constraints {
			if c == prototk.AttestationResult_ENDORSER_MUST_SUBMIT {
				if tx.Signer != "" {
					// Multiple endorsers claiming it is an error
					return fmt.Errorf("multiple endorsers claimed submit")
				}
				tx.Signer = ar.Verifier.Lookup
			}
		}
	}
	// If there isn't an ENDORSER_MUST_SUBMIT constraint, we just use a one-time key
	if tx.Signer == "" {
		tx.Signer = fmt.Sprintf("testbed.onetime.%s", uuid.New())
	}
	return nil
}

func (tb *testbed) execPrivateTransaction(ctx context.Context, psc components.DomainSmartContract, tx *components.PrivateTransaction) error {

	if !strings.Contains(tx.Inputs.From, "@") {
		// Transaction manager normally does the full version of this
		tx.Inputs.From = fmt.Sprintf("%s@%s", tx.Inputs.From, tb.c.TransportManager().LocalNodeName())
	}

	// Testbed just uses a domain context for the duration of the TX, and flushes before returning
	dCtx := tb.c.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
	defer dCtx.Close()

	// First we call init on the smart contract to:
	// - validate the transaction ABI is understood by the contract
	// - get an initial list of verifiers that need to be resolved
	if err := psc.InitTransaction(ctx, tx); err != nil {
		return err
	}

	// Gather the addresses - in the testbed we assume these all to be local
	tx.PreAssembly.Verifiers = make([]*prototk.ResolvedVerifier, len(tx.PreAssembly.RequiredVerifiers))
	for i, v := range tx.PreAssembly.RequiredVerifiers {
		resolvedKey, err := tb.ResolveKey(ctx, v.Lookup, v.Algorithm, v.VerifierType)
		if err != nil {
			return fmt.Errorf("failed to resolve key %q: %s", v.Lookup, err)
		}
		tx.PreAssembly.Verifiers[i] = &prototk.ResolvedVerifier{
			Lookup:       v.Lookup,
			Algorithm:    v.Algorithm,
			Verifier:     resolvedKey.Verifier.Verifier,
			VerifierType: v.VerifierType,
		}
	}

	// Now call assemble
	if err := psc.AssembleTransaction(dCtx, tb.c.Persistence().DB(), tx); err != nil {
		return err
	}

	// The testbed only handles the OK result
	switch tx.PostAssembly.AssemblyResult {
	case prototk.AssembleTransactionResponse_OK:
	default:
		return fmt.Errorf("assemble result was %s", tx.PostAssembly.AssemblyResult)
	}

	// The testbed always chooses to take the assemble output and progress to endorse
	// (no complex sequence selection routine that might result in abandonment).
	// So just write the states
	if err := psc.WritePotentialStates(dCtx, tb.c.Persistence().DB(), tx); err != nil {
		return err
	}

	// Gather signatures
	if err := tb.gatherSignatures(ctx, tx); err != nil {
		return err
	}

	// Gather endorsements (this would be a distributed activity across nodes in the real engine)
	if err := tb.gatherEndorsements(dCtx, psc, tx); err != nil {
		return err
	}

	log.L(ctx).Infof("Assembled and endorsed inputs=%d outputs=%d signatures=%d endorsements=%d",
		len(tx.PostAssembly.InputStates), len(tx.PostAssembly.OutputStates), len(tx.PostAssembly.Signatures), len(tx.PostAssembly.Endorsements))

	// Pick the signer for the base ledger transaction (we are always the coordinator in the testbed so this logic is much simplified from the private TX manager)
	if err := tb.resolveTXSigner(tx); err != nil {
		return err
	}

	// Prepare the transaction
	if err := psc.PrepareTransaction(dCtx, tb.c.Persistence().DB(), tx); err != nil {
		return err
	}

	// Build any nullifiers
	if err := tb.writeNullifiersToContext(dCtx, tx); err != nil {
		return err
	}

	// Flush the context
	dbTXResultCB, err := dCtx.Flush(tb.c.Persistence().DB())
	if err != nil {
		return err
	}
	dbTXResultCB(nil)

	// If preparing only, stop here
	if tx.Inputs.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION {
		return nil
	}

	if tx.PreparedPrivateTransaction != nil && tx.PreparedPrivateTransaction.To != nil {
		// Private transaction
		nextContract, err := tb.c.DomainManager().GetSmartContractByAddress(ctx, tb.c.Persistence().DB(), *tx.PreparedPrivateTransaction.To)
		if err != nil {
			return err
		}
		nextTX := mapDirectlyToInternalPrivateTX(tx.PreparedPrivateTransaction, tx.Inputs.Intent)
		return tb.execPrivateTransaction(ctx, nextContract, nextTX)
	} else {
		// Public transaction
		if tx.Inputs.Intent == prototk.TransactionSpecification_CALL {
			var ignored any // TODO: return the output in some way
			return tb.ExecBaseLedgerCall(ctx, &ignored, &pldapi.TransactionCall{
				TransactionInput: *tx.PreparedPublicTransaction,
				PublicCallOptions: pldapi.PublicCallOptions{
					Block: "latest",
				},
			})
		}
		_, err := tb.ExecTransactionSync(ctx, tx.PreparedPublicTransaction)
		return err
	}
}

func mapDirectlyToInternalPrivateTX(etx *pldapi.TransactionInput, intent prototk.TransactionSpecification_Intent) *components.PrivateTransaction {
	return &components.PrivateTransaction{
		ID: uuid.New(),
		Inputs: &components.TransactionInputs{
			Domain:   etx.Domain,
			From:     etx.From,
			To:       *etx.To,
			Function: etx.ABI[0],
			Inputs:   etx.Data,
			Intent:   intent,
		},
	}
}

func mapTransactionState(state *components.FullState, tx *components.TransactionInputs) *pldapi.StateEncoded {
	return &pldapi.StateEncoded{
		DomainName:      tx.Domain,
		ContractAddress: tx.To,
		ID:              state.ID,
		Schema:          state.Schema,
		Data:            state.Data.Bytes(),
	}
}

func (tb *testbed) mapTransaction(ctx context.Context, tx *components.PrivateTransaction) (*TransactionResult, error) {
	inputStates := make([]*pldapi.StateEncoded, len(tx.PostAssembly.InputStates))
	for i, state := range tx.PostAssembly.InputStates {
		inputStates[i] = mapTransactionState(state, tx.Inputs)
	}
	outputStates := make([]*pldapi.StateEncoded, len(tx.PostAssembly.OutputStates))
	for i, state := range tx.PostAssembly.OutputStates {
		outputStates[i] = mapTransactionState(state, tx.Inputs)
	}
	readStates := make([]*pldapi.StateEncoded, len(tx.PostAssembly.ReadStates))
	for i, state := range tx.PostAssembly.ReadStates {
		readStates[i] = mapTransactionState(state, tx.Inputs)
	}
	infoStates := make([]*pldapi.StateEncoded, len(tx.PostAssembly.InfoStates))
	for i, state := range tx.PostAssembly.InfoStates {
		infoStates[i] = mapTransactionState(state, tx.Inputs)
	}

	preparedTransaction := tx.PreparedPublicTransaction
	if tx.PreparedPublicTransaction == nil {
		preparedTransaction = tx.PreparedPrivateTransaction
	}

	encodedCall, err := preparedTransaction.ABI[0].EncodeCallDataJSONCtx(ctx, preparedTransaction.Data)
	if err != nil {
		return nil, err
	}

	var domainData []byte
	if tx.PostAssembly.DomainData != nil {
		domainData = []byte(*tx.PostAssembly.DomainData)
	}

	return &TransactionResult{
		ID:                  tx.ID,
		EncodedCall:         encodedCall,
		PreparedTransaction: preparedTransaction,
		PreparedMetadata:    tx.PreparedMetadata,
		InputStates:         inputStates,
		OutputStates:        outputStates,
		ReadStates:          readStates,
		InfoStates:          infoStates,
		DomainData:          domainData,
	}, nil
}

func (tb *testbed) rpcTestbedInvoke() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		invocation pldapi.TransactionInput,
		waitForCompletion bool,
	) (*TransactionResult, error) {
		psc, tx, err := tb.newPrivateTransaction(ctx, &invocation, prototk.TransactionSpecification_SEND_TRANSACTION)
		if err != nil {
			return nil, err
		}
		doExec := func() error {
			return tb.execPrivateTransaction(ctx, psc, tx)
		}
		if waitForCompletion {
			err = tb.c.DomainManager().ExecAndWaitTransaction(ctx, tx.ID, doExec)
		} else {
			err = doExec()
		}
		if err != nil {
			return nil, err
		}
		return tb.mapTransaction(ctx, tx)

	})
}

func (tb *testbed) rpcTestbedPrepare() rpcserver.RPCHandler {
	return rpcserver.RPCMethod1(func(ctx context.Context,
		invocation pldapi.TransactionInput,
	) (*TransactionResult, error) {

		psc, tx, err := tb.newPrivateTransaction(ctx, &invocation, prototk.TransactionSpecification_PREPARE_TRANSACTION)
		if err != nil {
			return nil, err
		}
		err = tb.execPrivateTransaction(ctx, psc, tx)
		if err != nil {
			return nil, err
		}
		return tb.mapTransaction(ctx, tx)
	})
}

func (tb *testbed) rpcResolveVerifier() rpcserver.RPCHandler {
	return rpcserver.RPCMethod3(func(ctx context.Context,
		lookup string,
		algorithm string,
		verifierType string,
	) (verifier string, _ error) {
		resolvedKey, err := tb.ResolveKey(ctx, lookup, algorithm, verifierType)
		if err != nil {
			return "", err
		}
		return resolvedKey.Verifier.Verifier, err
	})
}

func (tb *testbed) rpcTestbedCall() rpcserver.RPCHandler {
	return rpcserver.RPCMethod2(func(ctx context.Context,
		invocation *pldapi.TransactionInput,
		dataFormat tktypes.JSONFormatOptions,
	) (tktypes.RawJSON, error) {
		psc, tx, err := tb.newPrivateTransaction(ctx, invocation, prototk.TransactionSpecification_CALL)
		if err != nil {
			return nil, err
		}

		requiredVerifiers, err := psc.InitCall(ctx, tx.Inputs)
		if err != nil {
			return nil, err
		}

		resolvedVerifiers, err := tb.resolveVerifiers(ctx, requiredVerifiers)
		if err != nil {
			return nil, err
		}

		dCtx := tb.c.StateManager().NewDomainContext(ctx, psc.Domain(), psc.Address())
		defer dCtx.Close()

		cv, err := psc.ExecCall(dCtx, tb.c.Persistence().DB(), tx.Inputs, resolvedVerifiers)
		if err != nil {
			return nil, err
		}

		serializer, err := dataFormat.GetABISerializer(ctx)
		if err != nil {
			return nil, err
		}

		return serializer.SerializeJSONCtx(ctx, cv)
	})
}
