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
	"fmt"
	"math/big"
	"testing"

	zetocommon "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer"
	signercommon "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	protoz "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type testDomainCallbacks struct {
	returnFunc func() (*pb.FindAvailableStatesResponse, error)
}

func (dc *testDomainCallbacks) FindAvailableStates(ctx context.Context, req *pb.FindAvailableStatesRequest) (*pb.FindAvailableStatesResponse, error) {
	return dc.returnFunc()
}

func (dc *testDomainCallbacks) EncodeData(ctx context.Context, req *pb.EncodeDataRequest) (*pb.EncodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) RecoverSigner(ctx context.Context, req *pb.RecoverSignerRequest) (*pb.RecoverSignerResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) DecodeData(context.Context, *pb.DecodeDataRequest) (*pb.DecodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) GetStatesByID(ctx context.Context, req *pb.GetStatesByIDRequest) (*pb.GetStatesByIDResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) LocalNodeName(context.Context, *pb.LocalNodeNameRequest) (*pb.LocalNodeNameResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) SendTransaction(ctx context.Context, tx *pb.SendTransactionRequest) (*pb.SendTransactionResponse, error) {
	return nil, nil
}

func TestNew(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
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
	req := &pb.ConfigureDomainRequest{
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
		Circuits: &zetosignerapi.Circuits{
			"deposit":        &zetosignerapi.Circuit{Name: "circuit-deposit"},
			"withdraw":       &zetosignerapi.Circuit{Name: "circuit-withdraw"},
			"transfer":       &zetosignerapi.Circuit{Name: "circuit-transfer"},
			"transferLocked": &zetosignerapi.Circuit{Name: "circuit-transfer-locked"},
		},
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
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	req := &pb.InitDomainRequest{
		AbiStateSchemas: []*pb.StateSchema{
			{
				Id: "schema1",
			},
			{
				Id: "schema2",
			},
			{
				Id: "schema3",
			},
			{
				Id: "schema4",
			},
			{
				Id: "schema5",
			},
		},
	}
	res, err := z.InitDomain(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "schema1", z.coinSchema.Id)
	assert.Equal(t, "schema2", z.nftSchema.Id)
	assert.Equal(t, "schema3", z.merkleTreeRootSchema.Id)
	assert.Equal(t, "schema4", z.merkleTreeNodeSchema.Id)
}

func TestInitDeploy(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	req := &pb.InitDeployRequest{
		Transaction: &pb.DeployTransactionSpecification{
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
	testCases := []struct {
		name                  string
		constructorParamsJson string
		errorMsg              string
		tokenName             string
		circuits              *zetosignerapi.Circuits
		isNonFungible         bool
	}{
		{
			name:                  "Invalid JSON in ConstructorParamsJson",
			constructorParamsJson: "bad json",
			errorMsg:              "PD210006: Failed to validate prepare deploy parameters. invalid character 'b' looking for beginning of value",
		},
		{
			name:                  "Circuit ID Not Found",
			constructorParamsJson: "{}",
			circuits: &zetosignerapi.Circuits{
				"deposit": &zetosignerapi.Circuit{Name: "circuit1"},
			},
			tokenName: "testToken1",
			errorMsg:  "PD210007: Failed to find circuit ID based on the token name. PD210000: Contract '' not found",
		},
		{
			name:      "Valid fungible token",
			tokenName: constants.TOKEN_ANON,
			circuits: &zetosignerapi.Circuits{
				"deposit":        &zetosignerapi.Circuit{Name: "circuit-deposit"},
				"withdraw":       &zetosignerapi.Circuit{Name: "circuit-withdraw"},
				"transfer":       &zetosignerapi.Circuit{Name: "circuit-transfer"},
				"transferLocked": &zetosignerapi.Circuit{Name: "circuit-transfer-locked"},
			},
			constructorParamsJson: fmt.Sprintf("{\"tokenName\":\"%s\"}", constants.TOKEN_ANON),
			isNonFungible:         false,
		},
		{
			name:      "Non-fungible token",
			tokenName: constants.TOKEN_NF_ANON,
			circuits: &zetosignerapi.Circuits{
				"deposit":        &zetosignerapi.Circuit{Name: "circuit-deposit"},
				"withdraw":       &zetosignerapi.Circuit{Name: "circuit-withdraw"},
				"transfer":       &zetosignerapi.Circuit{Name: "circuit-transfer"},
				"transferLocked": &zetosignerapi.Circuit{Name: "circuit-transfer-locked"},
			},
			constructorParamsJson: fmt.Sprintf("{\"tokenName\":\"%s\"}", constants.TOKEN_NF_ANON),
			isNonFungible:         true,
		},
		{
			name:      "Non-fungible token with nullifier",
			tokenName: constants.TOKEN_NF_ANON_NULLIFIER,
			circuits: &zetosignerapi.Circuits{
				"deposit":        &zetosignerapi.Circuit{Name: "circuit-deposit"},
				"withdraw":       &zetosignerapi.Circuit{Name: "circuit-withdraw"},
				"transfer":       &zetosignerapi.Circuit{Name: "circuit-transfer"},
				"transferLocked": &zetosignerapi.Circuit{Name: "circuit-transfer-locked"},
			},
			constructorParamsJson: fmt.Sprintf("{\"tokenName\":\"%s\"}", constants.TOKEN_NF_ANON_NULLIFIER),
			isNonFungible:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testCallbacks := &domain.MockDomainCallbacks{}
			z := New(testCallbacks)
			z.config = &types.DomainFactoryConfig{
				DomainContracts: types.DomainConfigContracts{
					Implementations: []*types.DomainContract{
						{
							Name:     tc.tokenName,
							Circuits: tc.circuits,
						},
					},
				},
			}

			req := &pb.PrepareDeployRequest{
				Transaction: &pb.DeployTransactionSpecification{
					TransactionId:         "0x1234",
					ConstructorParamsJson: tc.constructorParamsJson,
				},
				ResolvedVerifiers: []*pb.ResolvedVerifier{
					{
						Verifier: "Alice",
					},
				},
			}

			res, err := z.PrepareDeploy(context.Background(), req)

			if tc.errorMsg == "" {
				require.NoError(t, err)
				assert.Contains(t, res.Transaction.ParamsJson, fmt.Sprintf("\"tokenName\":\"%s\"", tc.tokenName))
				assert.Contains(t, res.Transaction.ParamsJson, fmt.Sprintf("\"isNonFungible\":%t", tc.isNonFungible))
			} else {
				assert.EqualError(t, err, tc.errorMsg)
			}
		})
	}
}

func TestInitContract(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	req := &pb.InitContractRequest{
		ContractConfig: []byte("bad config"),
	}
	res, err := z.InitContract(context.Background(), req)
	assert.NoError(t, err) // so we don't block the indexing
	require.False(t, res.Valid)

	conf := types.DomainInstanceConfig{
		Circuits: &zetosignerapi.Circuits{
			"deposit":        &zetosignerapi.Circuit{Name: "circuit-deposit"},
			"withdraw":       &zetosignerapi.Circuit{Name: "circuit-withdraw"},
			"transfer":       &zetosignerapi.Circuit{Name: "circuit-transfer"},
			"transferLocked": &zetosignerapi.Circuit{Name: "circuit-transfer-locked"},
		},
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
		"circuits": {
			"deposit": { "name": "circuit-deposit", "type": "", "usesEncryption": false, "usesKyc":false, "usesNullifiers": false },
			"withdraw": { "name": "circuit-withdraw", "type": "", "usesEncryption": false, "usesKyc":false, "usesNullifiers": false },
			"transfer": { "name": "circuit-transfer", "type": "", "usesEncryption": false, "usesKyc":false, "usesNullifiers": false },
			"transferLocked": { "name": "circuit-transfer-locked", "type": "", "usesEncryption": false, "usesKyc":false, "usesNullifiers": false }
		},
		"tokenName": "testToken1"
	}`, res.ContractConfig.ContractConfigJson)
}

func TestInitTransaction(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	req := &pb.InitTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionAbiJson:    "bad json",
			FunctionParamsJson: "bad json",
			ContractInfo: &pb.ContractInfo{
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
		Circuits: &zetosignerapi.Circuits{
			"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
		},
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
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210016: Unexpected signature for function 'mint': expected='function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; bytes data; }', actual=''")

	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; bytes data; }"
	_, err = z.InitTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210008: Failed to validate init transaction spec. PD210017: Failed to decode contract address. bad address - must be 20 bytes (len=0)")

	req.Transaction.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	res, err := z.InitTransaction(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "Alice", res.RequiredVerifiers[0].Lookup)
}

func TestAssembleTransaction(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &pb.StateSchema{
		Id: "coin",
	}
	z.dataSchema = &pb.StateSchema{
		Id: "data",
	}

	assert.Equal(t, "z1", z.Name())
	assert.Equal(t, "coin", z.CoinSchemaID())

	req := &pb.AssembleTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionAbiJson:    "{\"type\":\"function\",\"name\":\"mint\"}",
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &pb.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		ResolvedVerifiers: []*pb.ResolvedVerifier{
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

	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; bytes data; }"
	conf := types.DomainInstanceConfig{
		Circuits: &zetosignerapi.Circuits{
			"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
		},
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = pldtypes.JSONString(conf).Pretty()
	_, err = z.AssembleTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestEndorseTransaction(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	req := &pb.EndorseTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			TransactionId:      "0x1234",
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &pb.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
	}
	_, err := z.EndorseTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210010: Failed to validate endorse transaction spec. PD210012: Failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; bytes data; }"
	conf := types.DomainInstanceConfig{
		Circuits: &zetosignerapi.Circuits{
			"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
		},
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = pldtypes.JSONString(conf).Pretty()
	_, err = z.EndorseTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func TestPrepareTransaction(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{
				{
					Name: "testToken1",
					Circuits: &zetosignerapi.Circuits{
						"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
					},
				},
			},
		},
	}
	req := &pb.PrepareTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			TransactionId:      pldtypes.RandBytes32().String(),
			FunctionParamsJson: "{\"mints\":[{\"to\":\"Alice\",\"amount\":\"10\"}]}",
			ContractInfo: &pb.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		OutputStates: []*pb.EndorsableState{
			{
				StateDataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}",
			},
		},
	}
	_, err := z.PrepareTransaction(context.Background(), req)
	assert.EqualError(t, err, "PD210011: Failed to validate prepare transaction spec. PD210012: Failed to unmarshal function abi json. unexpected end of JSON input")

	req.Transaction.FunctionAbiJson = "{\"type\":\"function\",\"name\":\"mint\"}"
	req.Transaction.FunctionSignature = "function mint(TransferParam[] memory mints) external { }; struct TransferParam { string to; uint256 amount; bytes data; }"
	conf := types.DomainInstanceConfig{
		Circuits: &zetosignerapi.Circuits{
			"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
		},
		TokenName: "testToken1",
	}
	req.Transaction.ContractInfo.ContractConfigJson = pldtypes.JSONString(conf).Pretty()
	_, err = z.PrepareTransaction(context.Background(), req)
	assert.NoError(t, err)
}

func newTestZeto() (*Zeto, *domain.MockDomainCallbacks) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*pb.FindAvailableStatesResponse, error) {
			return &pb.FindAvailableStatesResponse{}, nil
		},
	}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &pb.StateSchema{
		Id: "coin",
	}
	z.merkleTreeRootSchema = &pb.StateSchema{
		Id: "merkle_tree_root",
	}
	z.merkleTreeNodeSchema = &pb.StateSchema{
		Id: "merkle_tree_node",
	}
	z.dataSchema = &pb.StateSchema{
		Id: "data",
	}
	z.events.mint = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	z.events.burn = "event UTXOBurn(uint256[] inputs, uint256 output, address indexed submitter, bytes data)"
	z.events.transfer = "event UTXOTransfer(uint256[] inputs, uint256[] outputs, address indexed submitter, bytes data)"
	z.events.transferWithEnc = "event UTXOTransferWithEncryptedValues(uint256[] inputs, uint256[] outputs, uint256 encryptionNonce, uint256[2] ecdhPublicKey, uint256[] encryptedValues, address indexed submitter, bytes data)"
	z.events.withdraw = "event UTXOWithdraw(uint256 amount, uint256[] inputs, uint256 output, address indexed submitter, bytes data)"
	z.events.lock = "event UTXOsLocked(uint256[] inputs, uint256[] outputs, uint256[] lockedOutputs, address indexed delegate, address indexed submitter, bytes data)"
	z.events.identityRegistered = "event IdentityRegistered(uint256[] publicKey, bytes data)"
	return z, testCallbacks
}

func TestHandleEventBatch(t *testing.T) {
	z, testCallbacks := newTestZeto()
	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return nil, errors.New("find merkle tree root error")
	}
	ctx := context.Background()
	req := &pb.HandleEventBatchRequest{
		Events: []*pb.OnChainEvent{
			{
				DataJson:          "bad data",
				SoliditySignature: "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)",
			},
		},
		ContractInfo: &pb.ContractInfo{
			ContractConfigJson: `{!!! bad config`,
		},
	}
	_, err := z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210018")

	req.ContractInfo.ContractConfigJson = pldtypes.JSONString(map[string]interface{}{
		"circuitId": "anon_nullifier",
		"tokenName": "Zeto_AnonNullifier",
	}).Pretty()
	req.ContractInfo.ContractAddress = "0x1234"
	_, err = z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210017: Failed to decode contract address. bad address - must be 20 bytes (len=2)")

	req.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	_, err = z.HandleEventBatch(ctx, req)
	assert.EqualError(t, err, "PD210019: Failed to create Merkle tree for smt_Zeto_AnonNullifier_0x1234567890123456789012345678901234567890: PD210065: Failed to find available states for the merkle tree. find merkle tree root error")

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{}, nil
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

	encodedData, err := zetocommon.EncodeTransactionData(ctx, &prototk.TransactionSpecification{
		TransactionId: "0x30e43028afbb41d6887444f4c2b4ed6d00000000000000000000000000000000",
	}, nil)
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"data":      encodedData,
		"outputs":   []string{"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		"submitter": "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	req.Events[0].DataJson = string(data)
	req.Events[0].SoliditySignature = "event UTXOMint(uint256[] outputs, address indexed submitter, bytes data)"
	_, err = z.HandleEventBatch(ctx, req)
	assert.ErrorContains(t, err, "PD210020: Failed to handle events (failures=1). [0]PD210061: Failed to update merkle tree for the UTXOMint event. PD210056: Failed to create new node index from hash. 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	data, _ = json.Marshal(map[string]any{
		"data":      encodedData,
		"outputs":   []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"submitter": "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	req.Events[0].DataJson = string(data)
	res4, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res4.TransactionsComplete, 1)
	assert.Len(t, res4.NewStates, 2)

	data, _ = json.Marshal(map[string]any{
		"data":      encodedData,
		"outputs":   []string{"7980718117603030807695495350922077879582656644717071592146865497574198464253"},
		"submitter": "0x74e71b05854ee819cb9397be01c82570a178d019",
	})
	req.Events[0].DataJson = string(data)
	req.Events[0].SoliditySignature = "event UTXOWithdraw(uint256 amount, uint256[] inputs, uint256 output, address indexed submitter, bytes data)"
	_, err = z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)

	req.Events[0].SoliditySignature = "event UTXOsLocked(uint256[] inputs, uint256[] outputs, uint256[] lockedOutputs, address indexed delegate, address indexed submitter, bytes data)"
	res5, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res5.TransactionsComplete, 1)

	req.ContractInfo.ContractConfigJson = pldtypes.JSONString(map[string]interface{}{
		"circuitId": "anon_nullifier_kyc_transfer",
		"tokenName": "Zeto_AnonNullifierKYC",
	}).Pretty()
	req.Events[0].SoliditySignature = "event IdentityRegistered(uint256[] publicKey, bytes data)"
	res6, err := z.HandleEventBatch(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res6.TransactionsComplete, 0)
}

func TestGetVerifier(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &pb.StateSchema{
		Id: "coin",
	}
	snarkProver, err := zetosigner.NewSnarkProver(&zetosignerapi.SnarkProverConfig{})
	assert.NoError(t, err)
	z.snarkProver = snarkProver
	req := &pb.GetVerifierRequest{
		Algorithm: "bad algo",
	}
	_, err = z.GetVerifier(context.Background(), req)
	assert.ErrorContains(t, err, "Failed to get verifier. PD210088: 'bad algo' does not match supported algorithm")

	bytes, err := hex.DecodeString("7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	assert.NoError(t, err)
	req = &pb.GetVerifierRequest{
		Algorithm:    z.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		PrivateKey:   bytes,
	}
	res, err := z.GetVerifier(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "0x7045538ed4083e2cffffca1e98783aa97103226d1c44f3a54e11455822469e20", res.Verifier)
}

func TestSign(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{}
	z := New(testCallbacks)
	z.name = "z1"
	z.coinSchema = &pb.StateSchema{
		Id: "coin",
	}
	snarkProver := signer.NewTestProver(t)
	z.snarkProver = snarkProver

	req := &pb.SignRequest{
		PayloadType: "bad payload",
	}
	_, err := z.Sign(context.Background(), req)
	assert.ErrorContains(t, err, "PD210103: Sign payload type 'bad payload' not recognized")

	req = &pb.SignRequest{
		PayloadType: "domain:zeto:snark",
		Algorithm:   "bad algo",
	}
	_, err = z.Sign(context.Background(), req)
	assert.ErrorContains(t, err, "PD210023: Failed to sign. PD210088: 'bad algo' does not match supported algorithm")

	alice := signercommon.NewTestKeypair()
	bob := signercommon.NewTestKeypair()

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
	s := protoz.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	}
	bytes, err := json.Marshal(&s)
	require.NoError(t, err)

	provingReq := protoz.ProvingRequest{
		Circuit: &protoz.Circuit{
			Name:           "anon",
			Type:           "transfer",
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &protoz.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenType:        protoz.TokenType_fungible,
			TokenSecrets:     bytes,
		},
	}
	payload, err := proto.Marshal(&provingReq)
	require.NoError(t, err)
	req = &pb.SignRequest{
		Algorithm:   z.getAlgoZetoSnarkBJJ(),
		PayloadType: "domain:zeto:snark",
		PrivateKey:  bytes,
		Payload:     payload,
	}
	res, err := z.Sign(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, res.Payload, 36)

	// Test with nullifiers
	salt := crypto.NewSalt()
	fakeCoin := types.ZetoCoin{
		Salt:   (*pldtypes.HexUint256)(salt),
		Owner:  pldtypes.MustParseHexBytes(alicePubKey),
		Amount: pldtypes.Int64ToInt256(12345),
	}
	req = &pb.SignRequest{
		Algorithm:   z.getAlgoZetoSnarkBJJ(),
		PayloadType: "domain:zeto:nullifier",
		PrivateKey:  bytes,
		Payload:     pldtypes.JSONString(fakeCoin),
	}
	res, err = z.Sign(context.Background(), req)
	assert.NoError(t, err)
	assert.Len(t, res.Payload, 32)
}

func TestValidateStateHashes(t *testing.T) {
	z, _ := newTestZeto()
	ctx := context.Background()
	req := &pb.ValidateStateHashesRequest{
		States: []*pb.EndorsableState{
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

func TestValidateStateHashesDataState(t *testing.T) {
	z, _ := newTestZeto()
	ctx := context.Background()

	req := &pb.ValidateStateHashesRequest{
		States: []*pb.EndorsableState{
			{
				SchemaId:      z.DataSchemaID(),
				StateDataJson: "bad json",
			},
		},
	}
	_, err := z.ValidateStateHashes(ctx, req)
	assert.ErrorContains(t, err, "PD210087: Failed to unmarshal state data. invalid character 'b' looking for beginning of value")

	// Test case: Valid data state with no ID
	req.States[0].StateDataJson = `{
		"salt": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"data": "0xabcdef"
	}`
	res, err := z.ValidateStateHashes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res.StateIds, 1)

	// Test case: State hash mismatch
	req.States[0].Id = "0x1234"
	_, err = z.ValidateStateHashes(ctx, req)
	assert.ErrorContains(t, err, "PD210086: State hash mismatch (hashed vs. received)")

	// Test case: Matching state hash
	req.States[0].Id = "0x90a5df696783c409e262a20766584d6c90faf92c2851eed119d8b56704b90335"
	res, err = z.ValidateStateHashes(ctx, req)
	assert.NoError(t, err)
	assert.Len(t, res.StateIds, 1)
}

func TestGetHandler(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}

	tests := []struct {
		name        string
		action      string
		tokenName   string
		expectedNil bool
	}{
		// Tests for TOKEN_ANON
		{"Valid mint handler for TOKEN_ANON", "mint", constants.TOKEN_ANON, false},
		{"Valid transfer handler for TOKEN_ANON", "transfer", constants.TOKEN_ANON, false},
		{"Valid transferLocked handler for TOKEN_ANON", "transferLocked", constants.TOKEN_ANON, false},
		{"Valid lock handler for TOKEN_ANON", "lock", constants.TOKEN_ANON, false},
		{"Valid deposit handler for TOKEN_ANON", "deposit", constants.TOKEN_ANON, false},
		{"Valid withdraw handler for TOKEN_ANON", "withdraw", constants.TOKEN_ANON, false},
		{"Invalid handler for TOKEN_ANON", "bad", constants.TOKEN_ANON, true},

		// Tests for TOKEN_NF_ANON
		{"Valid mint handler for TOKEN_NF_ANON", "mint", constants.TOKEN_NF_ANON, false},
		{"Valid transfer handler for TOKEN_NF_ANON", "transfer", constants.TOKEN_NF_ANON, false},
		{"Invalid handler for TOKEN_NF_ANON", "bad", constants.TOKEN_NF_ANON, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := z.GetHandler(tt.action, tt.tokenName)
			if tt.expectedNil {
				assert.Nil(t, handler)
			} else {
				assert.NotNil(t, handler)
			}
		})
	}
}

func TestGetCallHandler(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}

	tests := []struct {
		name        string
		action      string
		tokenName   string
		expectedNil bool
	}{
		{"Valid call handler for TOKEN_ANON", "balanceOf", constants.TOKEN_ANON, false},
		{"Invalid call handler for TOKEN_ANON", "bad", constants.TOKEN_ANON, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := z.GetCallHandler(tt.action, tt.tokenName)
			if tt.expectedNil {
				assert.Nil(t, handler)
			} else {
				assert.NotNil(t, handler)
			}
		})
	}
}

func TestUnimplementedMethods(t *testing.T) {
	z := &Zeto{}
	_, err := z.BuildReceipt(context.Background(), nil)
	assert.ErrorContains(t, err, "PD210102: Not implemented")
}

func TestGetStateSchemas(t *testing.T) {
	schemas, err := types.GetStateSchemas()
	assert.NoError(t, err)
	assert.Len(t, schemas, 5)
}
