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
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/signer"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	protoz "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
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
		SnarkProver: zetosignerapi.SnarkProverConfig{
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
	assert.EqualError(t, err, "PD210002: Failed to parse domain config json. invalid character 'b' looking for beginning of value")

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
	assert.EqualError(t, err, "PD210005: Failed to validate init deploy parameters. invalid character 'b' looking for beginning of value")

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
	assert.EqualError(t, err, "PD210006: Failed to validate prepare deploy parameters. invalid character 'b' looking for beginning of value")

	req.Transaction.ConstructorParamsJson = "{}"
	_, err = z.PrepareDeploy(context.Background(), req)
	assert.EqualError(t, err, "PD210007: Failed to find circuit ID based on the token name. PD210000: Contract '' not found")

	req.Transaction.ConstructorParamsJson = "{\"tokenName\":\"testToken1\"}"
	_, err = z.PrepareDeploy(context.Background(), req)
	assert.NoError(t, err)
}

func TestInitContract(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.InitContractRequest{
		ContractConfig: []byte("bad config"),
	}
	res, err := z.InitContract(context.Background(), req)
	assert.NoError(t, err) // so we don't block the indexing
	require.False(t, res.Valid)

	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, _ := json.Marshal(conf)
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSON(configJSON)
	assert.NoError(t, err)
	req.ContractConfig = encoded
	res, err = z.InitContract(context.Background(), req)
	assert.NoError(t, err)
	require.True(t, res.Valid)
	require.JSONEq(t, `{
		"circuitId": "circuit1",
		"tokenName": "testToken1"
	}`, res.ContractConfig.ContractConfigJson)
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
				ContractConfigJson: `{!!! bad`,
			},
		},
	}
	_, err := z.InitTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "PD210008")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"test\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "PD210008")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"test\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "PD210008")

	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	configJSON, err := json.Marshal(conf)
	assert.NoError(t, err)
	req.Transaction.ContractInfo.ContractConfigJson = string(configJSON)
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210014: Unknown function: test")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210015: Failed to validate function params. invalid character 'b' looking for beginning of value")

	req.Transaction.FunctionParamsJson = "{}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210015: Failed to validate function params. PD210024: No transfer parameters provided")

	req.Transaction.FunctionParamsJson = "{\"mints\":[{}]}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210015: Failed to validate function params. PD210025: Parameter 'to' is required (index=0)")

	req.Transaction.FunctionParamsJson = "{\"mints\":[{\"to\":\"Alice\"}]}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210015: Failed to validate function params. PD210026: Parameter 'amount' is required (index=0)")

	req.Transaction.FunctionParamsJson = "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"0\"}]}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210015: Failed to validate function params. PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)")

	req.Transaction.FunctionParamsJson = "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210016: Unexpected signature for function 'mint': expected='function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; }', actual=''")

	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; }"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210017: Failed to decode contract address. bad address - must be 20 bytes (len=0)")

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

	assert.Equal(t, "z1", z.Name())
	assert.Equal(t, "coin", z.CoinSchemaID())

	req := &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionAbiJson:    "{\"type\":\"function\",\"name\":\"mint\"}",
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025",
				Algorithm:    z.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	_, err := z.AssembleTransaction(context.Background(), req)
	assert.ErrorContains(t, err, "PD210009")

	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = tktypes.JSONString(conf).Pretty()
	_, err = z.AssembleTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestEndorseTransaction(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	req := &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
	}
	_, err := z.EndorseTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210010: Failed to validate endorse transaction spec. PD210012: Failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = tktypes.JSONString(conf).Pretty()
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
				},
			},
		},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		OutputStates: []*prototk.EndorsableState{
			{
				StateDataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}",
			},
		},
	}
	_, err := z.PrepareTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210011: Failed to validate prepare transaction spec. PD210012: Failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; }"
	conf := types.DomainInstanceConfig{
		CircuitId: "circuit1",
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = tktypes.JSONString(conf).Pretty()
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
	useNullifiers := false
	addr, _ := tktypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	_, err := findCoins(context.Background(), z, useNullifiers, addr, "{}")
	assert.EqualError(t, err, "find coins error")

	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x0a\"}",
				},
			},
		}, nil
	}
	res, err := findCoins(context.Background(), z, useNullifiers, addr, "{}")
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func newTestZeto() (*Zeto, *testDomainCallbacks) {
	testCallbacks := &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}
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
	return z, testCallbacks
}

func TestHandleEventBatch(t *testing.T) {
	z, testCallbacks := newTestZeto()
	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return nil, errors.New("find merkle tree root error")
	}
	ctx := context.Background()
	req := &prototk.HandleEventBatchRequest{
		Events: []*prototk.OnChainEvent{
			{
				DataJson:          "bad data",
				SoliditySignature: "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)",
			},
		},
		ContractInfo: &prototk.ContractInfo{
			ContractConfigJson: `{!!! bad config`,
		},
	}
	_, err := z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210018")

	req.ContractInfo.ContractConfigJson = tktypes.JSONString(map[string]interface{}{
		"circuitId": "anon_nullifier",
		"tokenName": "Zeto_AnonNullifier",
	}).Pretty()
	req.ContractInfo.ContractAddress = "0x1234"
	_, err = z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210017: Failed to decode contract address. bad address - must be 20 bytes (len=2)")

	req.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	_, err = z.HandleEventBatch(ctx, req)
	assert.EqualError(t, err, "PD210019: Failed to create Merkle tree for smt_Zeto_AnonNullifier_0x1234567890123456789012345678901234567890: PD210065: Failed to find available states for the merkle tree. find merkle tree root error")

	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{}, nil
	}
	res1, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res1.TransactionsComplete, 0)

	req.Events[0].SoliditySignature = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	res2, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res2.TransactionsComplete, 0)

	req.Events[0].SoliditySignature = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"
	res3, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res3.TransactionsComplete, 0)

	req.Events[0].SoliditySignature = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210020: Failed to handle events (failures=1). [0]PD210061: Failed to update merkle tree for the UTXOMint event. PD210056: Failed to create new node index from hash. 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"outputs\":[\"7980718117603030807695495350922077879582656644717071592146865497574198464253\"],\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	res4, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res4.TransactionsComplete, 1)
	assert.Len(t, res4.NewStates, 2)

	req.Events[0].SoliditySignature = "event UTXOWithdraw(uint256 amount, uint256[] inputs, uint256 output, address indexed submitter, bytes data)"
	req.Events[0].DataJson = "{\"data\":\"0x0001000030e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000\",\"output\":\"7980718117603030807695495350922077879582656644717071592146865497574198464253\",\"submitter\":\"0x74e71b05854ee819cb9397be01c82570a178d019\"}"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
}

func TestGetVerifier(t *testing.T) {
	testCallbacks := &testDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &prototk.StateSchema{
		Id: "coin",
	}
	snarkProver, err := zetosigner.NewSnarkProver(&zetosignerapi.SnarkProverConfig{})
	assert.NoError(t, err)
	z.snarkProver = snarkProver
	req := &prototk.GetVerifierRequest{
		Algorithm: "bad algo",
	}
	_, err = z.GetVerifier(context.Background(), req)
	assert.ErrorContains(t, err, "Failed to get verifier. PD210088: 'bad algo' does not match supported algorithm")

	bytes, err := hex.DecodeString("7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	assert.NoError(t, err)
	req = &prototk.GetVerifierRequest{
		Algorithm:    z.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
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
	snarkProver := signer.NewTestProver(t)
	z.snarkProver = snarkProver

	req := &prototk.SignRequest{
		PayloadType: "bad payload",
	}
	_, err := z.Sign(context.Background(), req)
	assert.ErrorContains(t, err, "PD210103: Sign payload type 'bad payload' not recognized")

	req = &prototk.SignRequest{
		PayloadType: "domain:zeto:snark",
		Algorithm:   "bad algo",
	}
	_, err = z.Sign(context.Background(), req)
	assert.ErrorContains(t, err, "PD210023: Failed to sign. PD210088: 'bad algo' does not match supported algorithm")

	bytes, err := hex.DecodeString("7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	assert.NoError(t, err)
	alice := signer.NewTestKeypair()
	bob := signer.NewTestKeypair()

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

	// Test with nullifiers
	salt := crypto.NewSalt()
	fakeCoin := types.ZetoCoin{
		Salt:   (*tktypes.HexUint256)(salt),
		Owner:  tktypes.MustParseHexBytes(alicePubKey),
		Amount: tktypes.Int64ToInt256(12345),
	}
	req = &prototk.SignRequest{
		Algorithm:   z.getAlgoZetoSnarkBJJ(),
		PayloadType: "domain:zeto:nullifier",
		PrivateKey:  bytes,
		Payload:     tktypes.JSONString(fakeCoin),
	}
	res, err = z.Sign(context.Background(), req)
	assert.NoError(t, err)
	assert.Len(t, res.Payload, 32)
}

func TestValidateStateHashes(t *testing.T) {
	z, _ := newTestZeto()
	ctx := context.Background()
	req := &prototk.ValidateStateHashesRequest{
		States: []*prototk.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
	}
	_, err := z.ValidateStateHashes(ctx, req)
	assert.ErrorContains(t, err, "PD210087: Failed to unmarshal state data. invalid character 'b' looking for beginning of value")

	req.States[0].StateDataJson = "{\"salt\":\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\"}"
	_, err = z.ValidateStateHashes(ctx, req)
	assert.ErrorContains(t, err, "PD210048: Failed to create Poseidon hash for an output coin. inputs values not inside Finite Field")

	req.States[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\"}"
	res, err := z.ValidateStateHashes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res.StateIds, 1)

	req.States[0].Id = "0x1234"
	_, err = z.ValidateStateHashes(ctx, req)
	assert.ErrorContains(t, err, "PD210086: State hash mismatch (hashed vs. received): 0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f != 0x1234")

	req.States[0].Id = "0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f"
	res, err = z.ValidateStateHashes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res.StateIds, 1)
}

func findCoins(ctx context.Context, z *Zeto, useNullifiers bool, contractAddress *tktypes.EthAddress, query string) ([]*types.ZetoCoin, error) {
	states, err := z.findAvailableStates(ctx, useNullifiers, contractAddress.String(), query)
	if err != nil {
		return nil, err
	}

	coins := make([]*types.ZetoCoin, len(states))
	for i, state := range states {
		if coins[i], err = z.makeCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}

func TestGetHandler(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}
	assert.NotNil(t, z.GetHandler("mint"))
	assert.NotNil(t, z.GetHandler("transfer"))
	assert.NotNil(t, z.GetHandler("lock"))
	assert.NotNil(t, z.GetHandler("deposit"))
	assert.NotNil(t, z.GetHandler("withdraw"))
	assert.Nil(t, z.GetHandler("bad"))
}

func TestUnimplementedMethods(t *testing.T) {
	z := &Zeto{}
	_, err := z.InitCall(context.Background(), nil)
	assert.ErrorContains(t, err, "PD210085: Not implemented")

	_, err = z.ExecCall(context.Background(), nil)
	assert.ErrorContains(t, err, "PD210085: Not implemented")

	_, err = z.BuildReceipt(context.Background(), nil)
	assert.ErrorContains(t, err, "PD210102: Not implemented")
}
