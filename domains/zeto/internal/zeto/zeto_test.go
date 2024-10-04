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

package zeto

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	assert.NotNil(t, z)
}

func TestConfigureDomain(t *testing.T) {
	z := &Zeto{}
	dfConfig := &types.DomainFactoryConfig{
		FactoryAddress: "0x1234",
		SnarkProver: zetosigner.SnarkProverConfig{
			CircuitsDir: "circuit-dir",
		},
	}
	configBytes, err := json.Marshal(dfConfig)
	assert.NoError(t, err)
	req := &prototk.ConfigureDomainRequest{
		Name:       "z1",
		ConfigJson: "bad json",
	}
	_, err = z.ConfigureDomain(context.Background(), req)
	assert.EqualError(t, err, "failed to parse domain config json. invalid character 'b' looking for beginning of value")

	req.ConfigJson = string(configBytes)
	res, err := z.ConfigureDomain(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res)

}

func TestDecodeDomainConfig(t *testing.T) {
	config := &types.DomainInstanceConfig{
		CircuitId: "circuit-id",
		TokenName: "token-name",
	}
	configJSON, err := json.Marshal(config)
	assert.NoError(t, err)

	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)

	z := &Zeto{name: "z1"}
	decoded, err := z.decodeDomainConfig(context.Background(), encoded)
	assert.NoError(t, err)
	assert.Equal(t, config, decoded)

	assert.Equal(t, z.getAlgoZetoSnarkBJJ(), "domain:z1:snark:babyjubjub")
}

func TestInitDomain(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.InitDomainRequest{
		AbiStateSchemas: []*prototk.StateSchema{
			{
				Id: "schema1",
			},
			{
				Id: "schema2",
			},
			{
				Id: "schema3",
			},
		},
	}
	res, err := z.InitDomain(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "schema1", z.coinSchema.Id)
	assert.Equal(t, "schema2", z.merkleTreeRootSchema.Id)
	assert.Equal(t, "schema3", z.merkleTreeNodeSchema.Id)
}

func TestInitDeploy(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId:         "0x1234",
			ConstructorParamsJson: "bad json",
		},
	}
	_, err := z.InitDeploy(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init deploy parameters. invalid character 'b' looking for beginning of value")

	req.Transaction.ConstructorParamsJson = "{}"
	_, err = z.InitDeploy(context.Background(), req)
	assert.NoError(t, err)
}

func TestPrepareDeploy(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{
				{
					Name:      "testToken1",
					CircuitId: "circuit1",
				},
			},
		},
	}
	req := &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			TransactionId:         "0x1234",
			ConstructorParamsJson: "bad json",
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Verifier: "Alice",
			},
		},
	}
	_, err := z.PrepareDeploy(context.Background(), req)
	assert.EqualError(t, err, "failed to validate prepare deploy parameters. invalid character 'b' looking for beginning of value")

	req.Transaction.ConstructorParamsJson = "{}"
	_, err = z.PrepareDeploy(context.Background(), req)
	assert.EqualError(t, err, "failed to find circuit ID based on the token name. contract  not found")

	req.Transaction.ConstructorParamsJson = "{\"tokenName\":\"testToken1\"}"
	z.factoryABI = abi.ABI{}
	_, err = z.PrepareDeploy(context.Background(), req)
	assert.NoError(t, err)
}

func TestInitTransaction(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionAbiJson:    "bad json",
			FunctionParamsJson: "bad json",
			ContractConfig:     []byte("bad config"),
		},
	}
	_, err := z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to unmarshal function abi json. invalid character 'b' looking for beginning of value")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"test\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. unknown function: test")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to validate function params. invalid character 'b' looking for beginning of value")

	req.Transaction.FunctionParamsJson = "{}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to validate function params. parameter 'to' is required")

	req.Transaction.FunctionParamsJson = "{\"to\":\"Alice\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to validate function params. parameter 'amount' is required")

	req.Transaction.FunctionParamsJson = "{\"to\":\"Alice\",\"amount\":\"0\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to validate function params. parameter 'amount' must be greater than 0")

	req.Transaction.FunctionParamsJson = "{\"to\":\"Alice\",\"amount\":\"10\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. unexpected signature for function 'mint': expected=function mint(string memory to, uint256 amount) external { } actual=")

	req.Transaction.FunctionSignature = "function mint(string memory to, uint256 amount) external { }"
	_, err = z.InitTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "failed to validate init transaction spec. failed to decode domain config. FF22045: Insufficient bytes")

	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractConfig = encoded
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to decode contract address. bad address - must be 20 bytes (len=0)")

	req.Transaction.ContractAddress = "0x1234567890123456789012345678901234567890"
	res, err := z.InitTransaction(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "Alice", res.RequiredVerifiers[0].Lookup)
}

func TestAssembleTransaction(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionAbiJson:    "{\"type\":\"function\",\"name\":\"mint\"}",
			FunctionParamsJson: "{\"to\":\"Alice\",\"amount\":\"10\"}",
			ContractAddress:    "0x1234567890123456789012345678901234567890",
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025",
				Algorithm:    z.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	_, err := z.AssembleTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "failed to validate assemble transaction spec. unexpected signature for function 'mint'")

	req.Transaction.FunctionSignature = "function mint(string memory to, uint256 amount) external { }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractConfig = encoded
	_, err = z.AssembleTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestEndorseTransaction(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionParamsJson: "{\"to\":\"Alice\",\"amount\":\"10\"}",
			ContractAddress:    "0x1234567890123456789012345678901234567890",
		},
	}
	_, err := z.EndorseTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate endorse transaction spec. failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(string memory to, uint256 amount) external { }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractConfig = encoded
	_, err = z.EndorseTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestPrepareTransaction(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{
				{
					Name:      "testToken1",
					CircuitId: "circuit1",
					Abi:       "[{\"inputs\": [{\"internalType\": \"bytes32\",\"name\": \"transactionId\",\"type\": \"bytes32\"}],\"name\": \"transfer\",\"outputs\": [],\"type\": \"function\"}]",
				},
			},
		},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionParamsJson: "{\"to\":\"Alice\",\"amount\":\"10\"}",
			ContractAddress:    "0x1234567890123456789012345678901234567890",
		},
		OutputStates: []*prototk.EndorsableState{
			{
				StateDataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}",
			},
		},
	}
	_, err := z.PrepareTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate prepare transaction spec. failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(string memory to, uint256 amount) external { }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractConfig = encoded
	_, err = z.PrepareTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestFindCoins(t *testing.T) {
	testCallbacks := &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("find coins error")
		},
	}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	addr, _ := ethtypes.NewAddress("0x1234567890123456789012345678901234567890")
	_, err := z.FindCoins(context.Background(), *addr, "{}")
	assert.EqualError(t, err, "failed to find available states. find coins error")

	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"Alice\",\"ownerKey\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x0a\"}",
				},
			},
		}, nil
	}
	res, err := z.FindCoins(context.Background(), *addr, "{}")
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestHandleEventBatch(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	req := &prototk.HandleEventBatchRequest{
		JsonEvents: "bad json",
	}
	ctx := context.Background()
	_, err := z.HandleEventBatch(ctx, req)
	assert.EqualError(t, err, "failed to unmarshal events. invalid character 'b' looking for beginning of value")
}
