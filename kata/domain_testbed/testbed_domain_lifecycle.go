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
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/blockindexer"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/internal/types"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"golang.org/x/crypto/sha3"
)

type testbedDomain struct {
	tb                     *testbed
	name                   string
	schemas                []*statestore.Schema
	constructorABI         *abi.Entry
	factoryContractAddress *ethtypes.Address0xHex
	factoryContractABI     abi.ABI
}

func (tb *testbed) registerDomain(ctx context.Context, name string, config *proto.DomainConfig) (*proto.InitDomainRequest, error) {

	abiSchemas := make([]*abi.Parameter, len(config.AbiStateSchemasJson))
	for i, schemaJSON := range config.AbiStateSchemasJson {
		if err := json.Unmarshal([]byte(schemaJSON), &abiSchemas[i]); err != nil {
			return nil, fmt.Errorf("bad ABI state schema %d: %s", i, err)
		}
	}
	domain := &testbedDomain{tb: tb, name: name}

	err := json.Unmarshal(([]byte)(config.ConstructorAbiJson), &domain.constructorABI)
	if err != nil {
		return nil, fmt.Errorf("bad constructor ABI function definition: %s", err)
	}
	if domain.constructorABI.Type != abi.Constructor {
		return nil, fmt.Errorf("bad constructor ABI function definition: type not 'constructor'")
	}

	if err := json.Unmarshal(([]byte)(config.FactoryContractAbiJson), &domain.factoryContractABI); err != nil {
		return nil, fmt.Errorf("bad factory contract ABI: %s", err)
	}

	domain.factoryContractAddress, err = ethtypes.NewAddress(config.FactoryContractAddress)
	if err != nil {
		return nil, fmt.Errorf("bad factory contract address: %s", err)
	}

	flushed := make(chan struct{})
	err = tb.stateStore.RunInDomainContext(name, func(ctx context.Context, dsi statestore.DomainStateInterface) (err error) {
		domain.schemas, err = dsi.EnsureABISchemas(abiSchemas)
		if err == nil {
			err = dsi.Flush(func(ctx context.Context, dsi statestore.DomainStateInterface) error {
				close(flushed)
				return nil
			})
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	select {
	case <-flushed:
	case <-ctx.Done():
		return nil, fmt.Errorf("flush timed out")
	}

	schemaIDs := make([]string, len(domain.schemas))
	for i, s := range domain.schemas {
		schemaIDs[i] = s.Signature
	}

	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	tb.domainRegistry[name] = domain
	return &proto.InitDomainRequest{
		AbiStateSchemaIds: schemaIDs,
	}, nil
}

func (tb *testbed) validateDeploy(ctx context.Context, domain *testbedDomain, constructorParams types.RawJSON) (*proto.PrepareDeployTransactionRequest, error) {

	contructorValues, err := domain.constructorABI.Inputs.ParseJSONCtx(ctx, constructorParams)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters for constructor: %s", err)
	}

	paladinTxID := uuid.New().String()
	constructorABIJSON, _ := json.Marshal(domain.constructorABI)
	constructorParamsJSON, _ := types.StandardABISerializer().SerializeJSONCtx(ctx, contructorValues)

	return &proto.PrepareDeployTransactionRequest{
		TransactionId:         paladinTxID,
		ConstructorAbi:        string(constructorABIJSON),
		ConstructorParamsJson: string(constructorParamsJSON),
	}, nil
}

func (tb *testbed) setupChainID() error {
	var chainID ethtypes.HexUint64
	if rpcErr := tb.blockchainRPC.CallRPC(tb.ctx, &chainID, "eth_chainId"); rpcErr != nil {
		return fmt.Errorf("eth_chainId failed: %s", rpcErr.Error())
	}
	tb.chainID = int64(chainID.Uint64())
	return nil
}

// We do a simplistic deploy here, assuming that all will go well on the blockchain
// (obviously the Paladin engine does a proper TX Management at this point)
func (tb *testbed) simpleTXEstimateSignSubmitAndWait(ctx context.Context, from string, to *ethtypes.Address0xHex, callData ethtypes.HexBytes0xPrefix) (*blockindexer.IndexedTransaction, error) {

	// Resolve the key (directly with the signer - we have no key manager here in the testbed)
	resolvedKey, err := tb.signer.Resolve(ctx, &proto.ResolveKeyRequest{
		Algorithms: []string{signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES},
		Path: []*proto.KeyPathSegment{
			{Name: from},
		},
	})
	if err != nil {
		return nil, err
	}
	fromAddr := resolvedKey.Identifiers[0].Identifier

	// Trivial nonce management in the testbed - just get the current nonce for this key, from the local node mempool, for each TX
	var nonce ethtypes.HexInteger
	if rpcErr := tb.blockchainRPC.CallRPC(ctx, &nonce, "eth_getTransactionCount", fromAddr, "latest"); rpcErr != nil {
		return nil, fmt.Errorf("eth_getTransactionCount for %s failed: %s", fromAddr, rpcErr.Error())
	}

	// Construct a simple transaction with the specified data for a permissioned zero-gas-price chain
	tx := ethsigner.Transaction{
		From: json.RawMessage(fmt.Sprintf(`"%s"`, fromAddr)), // for estimation of gas we need the from address
		To:   to,
		Data: callData,
	}

	// Estimate gas before submission
	var gasEstimate ethtypes.HexInteger
	if rpcErr := tb.blockchainRPC.CallRPC(ctx, &gasEstimate, "eth_estimateGas", tx); rpcErr != nil {
		return nil, fmt.Errorf("eth_estimateGas failed: %+v", rpcErr)
	}

	// If that went well, so submission with twice that gas as the limit.
	tx.GasLimit = ethtypes.NewHexInteger(new(big.Int).Mul(gasEstimate.BigInt(), big.NewInt(2)))
	tx.Nonce = &nonce

	// Sign
	sigPayload := tx.SignaturePayloadEIP1559(tb.chainID)
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write(sigPayload.Bytes())
	signature, err := tb.signer.Sign(ctx, &proto.SignRequest{
		Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		KeyHandle: resolvedKey.KeyHandle,
		Payload:   ethtypes.HexBytes0xPrefix(hash.Sum(nil)),
	})
	var sig *secp256k1.SignatureData
	if err == nil {
		sig, err = signer.DecodeCompactRSV(ctx, signature.Payload)
	}
	var rawTX []byte
	if err == nil {
		rawTX, err = tx.FinalizeEIP1559WithSignature(sigPayload, sig)
	}
	if err != nil {
		return nil, fmt.Errorf("signing failed with keyHandle %s (addr=%s): %s", resolvedKey.KeyHandle, fromAddr, err)
	}

	// Submit
	var txHash ethtypes.HexBytes0xPrefix
	if rpcErr := tb.blockchainRPC.CallRPC(ctx, &txHash, "eth_sendRawTransaction", ethtypes.HexBytes0xPrefix(rawTX)); rpcErr != nil {
		addr, decodedTX, err := ethsigner.RecoverRawTransaction(ctx, rawTX, tb.chainID)
		if err != nil {
			log.L(ctx).Errorf("Invalid transaction build during signing: %s", err)
		} else {
			log.L(ctx).Errorf("Rejected TX (from=%s): %+v", addr, logJSON(decodedTX.Transaction))
		}
		return nil, fmt.Errorf("eth_sendRawTransaction failed: %+v", rpcErr)
	}

	// Wait for the TX to go through - with a time limit
	withTimeout, cancelTimeout := context.WithTimeout(ctx, 10*time.Second)
	defer cancelTimeout()
	return tb.blockindexer.WaitForTransaction(withTimeout, txHash.String())

}

func (tb *testbed) deployPrivateSmartContract(ctx context.Context, domain *testbedDomain, txInstruction *proto.BaseLedgerTransaction) (*blockindexer.IndexedEvent, error) {

	abiFunc := domain.factoryContractABI.Functions()[txInstruction.FunctionName]
	if abiFunc == nil {
		return nil, fmt.Errorf("function %q does not exist on base ledger ABI", txInstruction.FunctionName)
	}
	callData, err := abiFunc.EncodeCallDataJSONCtx(ctx, []byte(txInstruction.ParamsJson))
	if err != nil {
		return nil, fmt.Errorf("encoding to function %q failed: %s", txInstruction.FunctionName, err)
	}

	tx, err := tb.simpleTXEstimateSignSubmitAndWait(ctx, txInstruction.SigningAddress, domain.factoryContractAddress, callData)
	if err != nil {
		return nil, err
	}

	events, err := tb.blockindexer.GetTransactionEventsByHash(ctx, tx.Hash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction events for deploy: %s", err)
	}

	if len(events) != 1 {
		return nil, fmt.Errorf("expected exactly one event from deploy function (received=%d)", len(events))
	}

	return events[0], nil
}

func (tb *testbed) getDomain(name string) (*testbedDomain, error) {
	tb.domainLock.Lock()
	defer tb.domainLock.Unlock()
	domain := tb.domainRegistry[name]
	if domain == nil {
		return nil, fmt.Errorf("domain %q not found", name)
	}
	return domain, nil

}
