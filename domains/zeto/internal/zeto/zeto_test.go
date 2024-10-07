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
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	protoz "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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
			ContractInfo: &prototk.ContractInfo{
				ContractConfig: []byte("bad config"),
			},
		},
	}
	_, err := z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to unmarshal function abi json. invalid character 'b' looking for beginning of value")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"test\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "failed to validate init transaction spec. failed to decode domain config. FF22045: Insufficient bytes")

	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractInfo.ContractConfig = encoded
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
	assert.EqualError(t, err, "failed to validate init transaction spec. failed to decode contract address. bad address - must be 20 bytes (len=0)")

	req.Transaction.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
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
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
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
	assert.ErrorContains(t, err, "failed to validate assemble transaction spec. failed to decode domain config. FF22045: Insufficient bytes")

	req.Transaction.FunctionSignature = "function mint(string memory to, uint256 amount) external { }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.Transaction.ContractInfo.ContractConfig = encoded
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
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
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
	req.Transaction.ContractInfo.ContractConfig = encoded
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
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
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
	req.Transaction.ContractInfo.ContractConfig = encoded
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
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	z.merkleTreeRootSchema = &prototk.StateSchema{
		Id: "merkle_tree_root",
	}
	z.merkleTreeNodeSchema = &prototk.StateSchema{
		Id: "merkle_tree_node",
	}
	z.mintSignature = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	z.transferSignature = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	z.transferWithEncSignature = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"

	ctx := context.Background()
	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				DataJson:          "bad data",
				SoliditySignature: "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)",
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfig: []byte("bad config"),
		},
	}
	_, err := z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "failed to abi decode domain instance config bytes. FF22045: Insufficient bytes")

	config := map[string]interface{}{
		"circuitId": "circuit1",
		"tokenName": "testToken1",
	}
	bytes, err := types.DomainInstanceConfigABI.EncodeABIDataValues(config)
	require.NoError(t, err)
	req.ContractInfo.ContractConfig = bytes
	req.ContractInfo.ContractAddress = "0x1234"
	_, err = z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "failed to parse contract address. bad address - must be 20 bytes (len=2)")

	req.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	res1, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res1.TransactionsComplete, 0)

	// bad transaction data for the mint event - should be logged and move on
	req.Events[0].DataJson = "{\"data\":\"0x0001\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	req.Events[0].DataJson = "{\"data\":\"0x0001ffff\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	// bad data for the transfer event - should be logged and move on
	req.Events[0].DataJson = "bad data"
	req.Events[0].SoliditySignature = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	// bad transaction data for the transfer event - should be logged and move on
	req.Events[0].DataJson = "{\"data\":\"0x0001\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	req.Events[0].DataJson = "{\"data\":\"0x0001ffff\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	// bad data for the transfer with encrypted values event - should be logged and move on
	req.Events[0].DataJson = "bad data"
	req.Events[0].SoliditySignature = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	// bad transaction data for the transfer with encrypted values event - should be logged and move on
	req.Events[0].DataJson = "{\"data\":\"0x0001\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	req.Events[0].DataJson = "{\"data\":\"0x0001ffff\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	res2, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000", res2.TransactionsComplete[0].TransactionId)

	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	res3, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000", res3.TransactionsComplete[0].TransactionId)

	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"
	res4, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000", res4.TransactionsComplete[0].TransactionId)

	config["tokenName"] = "Zeto_AnonNullifier"
	bytes, err = types.DomainInstanceConfigABI.EncodeABIDataValues(config)
	require.NoError(t, err)
	req.ContractInfo.ContractConfig = bytes
	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return nil, errors.New("find coins error")
	}
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	_, err = z.HandleEventBatch(ctx, req)
	assert.EqualError(t, err, "failed to handle events (failures=1). [0]failed to update merkle tree for the UTXOMint event. failed to create Merkle tree for smt_Zeto_AnonNullifier_0x1234567890123456789012345678901234567890: failed to find available states. find coins error")

	calls := 0
	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		if calls == 0 {
			calls++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"Alice\",\"ownerKey\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x0a\"}",
					},
				},
			}, nil
		} else {
			return &prototk.FindAvailableStatesResponse{}, nil
		}
	}
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.EqualError(t, err, "failed to handle events (failures=1). [0]failed to update merkle tree for the UTXOMint event. failed to create node index for 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff: key for the new node not inside the Finite Field")

	calls = 0
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	res5, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res5.TransactionsComplete, 1)
	assert.Len(t, res5.NewStates, 2)
	assert.Equal(t, "merkle_tree_node", res5.NewStates[0].SchemaId)
	assert.Equal(t, "merkle_tree_root", res5.NewStates[1].SchemaId)

	calls = 0
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	res6, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res6.TransactionsComplete, 1)
	assert.Len(t, res6.NewStates, 2)
	assert.Equal(t, "merkle_tree_node", res6.NewStates[0].SchemaId)
	assert.Equal(t, "merkle_tree_root", res6.NewStates[1].SchemaId)

	calls = 0
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	req.Events[0].SoliditySignature = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"
	res7, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res7.TransactionsComplete, 1)
	assert.Len(t, res7.NewStates, 2)
	assert.Equal(t, "merkle_tree_node", res7.NewStates[0].SchemaId)
	assert.Equal(t, "merkle_tree_root", res7.NewStates[1].SchemaId)
}

func TestGetVerifier(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	snarkProver, err := zetosigner.NewSnarkProver(&zetosigner.SnarkProverConfig{})
	assert.NoError(t, err)
	z.snarkProver = snarkProver
	req := &prototk.GetVerifierRequest{
		Algorithm: "bad algo",
	}
	_, err = z.GetVerifier(context.Background(), req)
	assert.ErrorContains(t, err, "failed to get verifier. 'bad algo' does not match supported algorithm")

	bytes, err := hex.DecodeString("7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	assert.NoError(t, err)
	req = &prototk.GetVerifierRequest{
		Algorithm:    z.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		PrivateKey:   bytes,
	}
	res, err := z.GetVerifier(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "0x7045538ed4083e2cffffca1e98783aa97103226d1c44f3a54e11455822469e20", res.Verifier)
}

func TestSign(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	snarkProver := zetosigner.NewTestProver(t)
	z.snarkProver = snarkProver

	req := &prototk.SignRequest{
		Algorithm: "bad algo",
	}
	_, err := z.Sign(context.Background(), req)
	assert.ErrorContains(t, err, "failed to sign. 'bad algo' does not match supported algorithm")

	bytes, err := hex.DecodeString("7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	assert.NoError(t, err)
	alice := zetosigner.NewTestKeypair()
	bob := zetosigner.NewTestKeypair()

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
	inputCommitments := []string{input1.Text(16), input2.Text(16)}

	inputValueInts := []uint64{inputValues[0].Uint64(), inputValues[1].Uint64()}
	inputSalts := []string{salt1.Text(16), salt2.Text(16)}
	outputValueInts := []uint64{outputValues[0].Uint64(), outputValues[1].Uint64()}

	alicePubKey := zetosigner.EncodeBabyJubJubPublicKey(alice.PublicKey)
	bobPubKey := zetosigner.EncodeBabyJubJubPublicKey(bob.PublicKey)

	provingReq := protoz.ProvingRequest{
		CircuitId: constants.CIRCUIT_ANON,
		Common: &protoz.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputValues:     outputValueInts,
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err := proto.Marshal(&provingReq)
	require.NoError(t, err)
	req = &prototk.SignRequest{
		Algorithm:   z.getAlgoZetoSnarkBJJ(),
		PayloadType: "domain:zeto:snark",
		PrivateKey:  bytes,
		Payload:     payload,
	}
	res, err := z.Sign(context.Background(), req)
	assert.NoError(t, err)
	assert.Len(t, res.Payload, 36)
}
