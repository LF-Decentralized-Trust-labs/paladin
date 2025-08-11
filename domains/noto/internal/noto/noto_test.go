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

package noto

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var encodedConfig = func(data *types.NotoConfigData_V0) []byte {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	encoded, err := types.NotoConfigABI_V0.EncodeABIDataJSON([]byte(fmt.Sprintf(`{
		"notaryAddress": "0x138baffcdcc3543aad1afd81c71d2182cdf9c8cd",
		"variant": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"data": "%s"
	}`, pldtypes.HexBytes(dataJSON).String())))
	if err != nil {
		panic(err)
	}
	var result []byte
	result = append(result, types.NotoConfigID_V0...)
	result = append(result, encoded...)
	return result
}

var mockCallbacks = &domain.MockDomainCallbacks{
	MockLocalNodeName: func() (*prototk.LocalNodeNameResponse, error) {
		return &prototk.LocalNodeNameResponse{
			Name: "node1",
		}, nil
	},
}

func TestABIParseFailure(t *testing.T) {
	assert.Panics(t, func() {
		mustLoadEventSignatures(abi.ABI{}, []string{"will-not-exist"})
	})
	assert.Panics(t, func() {
		mustParseJSON(map[any]any{true: false})
	})
}

func TestNotoDomainInit(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	configureRes, err := n.ConfigureDomain(ctx, &prototk.ConfigureDomainRequest{
		Name:       "noto",
		ConfigJson: "{}",
	})
	require.NoError(t, err)
	assert.Len(t, configureRes.DomainConfig.AbiStateSchemasJson, 4)

	initRes, err := n.InitDomain(ctx, &prototk.InitDomainRequest{
		AbiStateSchemas: []*prototk.StateSchema{
			{Id: "schema1"},
			{Id: "schema2"},
			{Id: "schema3"},
			{Id: "schema4"},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, initRes)

	assert.Equal(t, "noto", n.Name())
	assert.Equal(t, "schema1", n.CoinSchemaID())
	assert.Equal(t, "schema2", n.LockInfoSchemaID())
	assert.Equal(t, "schema3", n.LockedCoinSchemaID())
	assert.Equal(t, "schema4", n.DataSchemaID())
}

func TestNotoDomainDeployDefaults(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	deployTransaction := &prototk.DeployTransactionSpecification{
		TransactionId: "tx1",
		ConstructorParamsJson: `{
			"notary": "notary@node1",
			"notaryMode": "basic"
		}`,
	}

	initDeployRes, err := n.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: deployTransaction,
	})
	require.NoError(t, err)
	assert.Len(t, initDeployRes.RequiredVerifiers, 1)
	assert.Equal(t, "notary@node1", initDeployRes.RequiredVerifiers[0].Lookup)

	prepareDeployRes, err := n.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: deployTransaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, prepareDeployRes.Transaction.FunctionAbiJson)
	var deployParams map[string]any
	err = json.Unmarshal([]byte(prepareDeployRes.Transaction.ParamsJson), &deployParams)
	require.NoError(t, err)
	assert.Equal(t, "tx1", deployParams["transactionId"])
	assert.Equal(t, "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71", deployParams["notaryAddress"])
	deployData := pldtypes.MustParseHexBytes(deployParams["data"].(string))
	assert.JSONEq(t, `{
		"notaryLookup": "notary@node1",
		"notaryMode": "0x0",
		"privateAddress": null,
		"privateGroup": null,
		"restrictMint": true,
		"allowBurn": true,
		"allowLock": true
	}`, string(deployData))

	initContractRes, err := n.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
		ContractConfig:  encodedConfig(&types.NotoConfigData_V0{NotaryLookup: "notary@node1"}),
	})
	require.NoError(t, err)
	assert.True(t, initContractRes.Valid)
}

func TestNotoDomainDeployBasicConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	deployTransaction := &prototk.DeployTransactionSpecification{
		TransactionId: "tx1",
		ConstructorParamsJson: `{
			"notary": "notary@node1",
			"notaryMode": "basic",
			"options": {
				"basic": {
					"restrictMint": false,
					"allowBurn": false,
					"allowLock": false
				}
			}
		}`,
	}

	initDeployRes, err := n.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: deployTransaction,
	})
	require.NoError(t, err)
	assert.Len(t, initDeployRes.RequiredVerifiers, 1)
	assert.Equal(t, "notary@node1", initDeployRes.RequiredVerifiers[0].Lookup)

	prepareDeployRes, err := n.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: deployTransaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, prepareDeployRes.Transaction.FunctionAbiJson)
	var deployParams map[string]any
	err = json.Unmarshal([]byte(prepareDeployRes.Transaction.ParamsJson), &deployParams)
	require.NoError(t, err)
	assert.Equal(t, "tx1", deployParams["transactionId"])
	assert.Equal(t, "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71", deployParams["notaryAddress"])
	deployData := pldtypes.MustParseHexBytes(deployParams["data"].(string))
	assert.JSONEq(t, `{
		"notaryLookup": "notary@node1",
		"notaryMode": "0x0",
		"privateAddress": null,
		"privateGroup": null,
		"restrictMint": false,
		"allowBurn": false,
		"allowLock": false
	}`, string(deployData))

	initContractRes, err := n.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
		ContractConfig:  encodedConfig(&types.NotoConfigData_V0{NotaryLookup: "notary@node1"}),
	})
	require.NoError(t, err)
	assert.True(t, initContractRes.Valid)
}

func TestNotoDomainDeployHooksConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	ctx := context.Background()

	groupSalt := pldtypes.RandBytes32()
	deployTransaction := &prototk.DeployTransactionSpecification{
		TransactionId: "tx1",
		ConstructorParamsJson: fmt.Sprintf(`{
			"notary": "notary@node1",
			"notaryMode": "hooks",
			"options": {
				"hooks": {
					"publicAddress": "0x0a8cb8c4cf5aea4ea2ed3b3777ccddd3e0eb9bc5",
					"privateAddress": "0x37427fe250dcf58ea934cf91fb6248ac6eba1fd0",
					"privateGroup": {
						"salt": "%s",
						"members": ["notary@node1"]
					}
				}
			}
		}`, groupSalt.String()),
	}

	initDeployRes, err := n.InitDeploy(ctx, &prototk.InitDeployRequest{
		Transaction: deployTransaction,
	})
	require.NoError(t, err)
	assert.Len(t, initDeployRes.RequiredVerifiers, 1)
	assert.Equal(t, "notary@node1", initDeployRes.RequiredVerifiers[0].Lookup)

	prepareDeployRes, err := n.PrepareDeploy(ctx, &prototk.PrepareDeployRequest{
		Transaction: deployTransaction,
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, prepareDeployRes.Transaction.FunctionAbiJson)
	var deployParams map[string]any
	err = json.Unmarshal([]byte(prepareDeployRes.Transaction.ParamsJson), &deployParams)
	require.NoError(t, err)
	assert.Equal(t, "tx1", deployParams["transactionId"])
	assert.Equal(t, "0x0a8cb8c4cf5aea4ea2ed3b3777ccddd3e0eb9bc5", deployParams["notaryAddress"])
	deployData := pldtypes.MustParseHexBytes(deployParams["data"].(string))
	assert.JSONEq(t, fmt.Sprintf(`{
		"notaryLookup": "notary@node1",
		"notaryMode": "0x1",
		"privateAddress": "0x37427fe250dcf58ea934cf91fb6248ac6eba1fd0",
		"privateGroup": {
			"salt": "%s",
			"members": ["notary@node1"]
		},
		"restrictMint": false,
		"allowBurn": false,
		"allowLock": false
	}`, groupSalt), string(deployData))

	initContractRes, err := n.InitContract(ctx, &prototk.InitContractRequest{
		ContractAddress: "0xf6a75f065db3cef95de7aa786eee1d0cb1aeafc3",
		ContractConfig: encodedConfig(&types.NotoConfigData_V0{
			NotaryLookup: "notary@node1",
			NotaryMode:   types.NotaryModeIntHooks,
		}),
	})
	require.NoError(t, err)
	assert.True(t, initContractRes.Valid)
}

func TestConfigureDomainBadConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.ConfigureDomain(context.Background(), &prototk.ConfigureDomainRequest{
		ConfigJson: "!!wrong",
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitDeployBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitDeployBadMode(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "bad"
			}`,
		},
	})
	assert.ErrorContains(t, err, "PD200007")
}

func TestInitDeployMissingNotary(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{}`,
		},
	})
	assert.ErrorContains(t, err, "PD200007")
}

func TestInitDeployMissingHooksOptions(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}

	_, err := n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "hooks"
			}`,
		},
	})
	assert.Regexp(t, "PD200007.*options.hooks", err)

	_, err = n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "hooks",
				"options": {
					"hooks": {}
				}
			}`,
		},
	})
	assert.Regexp(t, "PD200007.*options.hooks.publicAddress", err)

	_, err = n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "hooks",
				"options": {
					"hooks": {
						"publicAddress": "0x0a8cb8c4cf5aea4ea2ed3b3777ccddd3e0eb9bc5"
					}
				}
			}`,
		},
	})
	assert.Regexp(t, "PD200007.*options.hooks.privateAddress", err)

	_, err = n.InitDeploy(context.Background(), &prototk.InitDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "hooks",
				"options": {
					"hooks": {
						"publicAddress": "0x0a8cb8c4cf5aea4ea2ed3b3777ccddd3e0eb9bc5",
						"privateAddress": "0x37427fe250dcf58ea934cf91fb6248ac6eba1fd0"
					}
				}
			}`,
		},
	})
	assert.Regexp(t, "PD200007.*options.hooks.privateGroup", err)
}

func TestPrepareDeployBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareDeployMissingVerifier(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "basic"
			}`,
		},
	})
	assert.ErrorContains(t, err, "PD200011")
}

func TestPrepareDeployBadNotary(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@bad@notgood",
				"notaryMode": "basic"
			}`,
		},
	})
	assert.ErrorContains(t, err, "PD020006")
}

func TestPrepareDeployUnqualifiedNotary(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	res, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary",
				"notaryMode": "basic"
			}`,
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	assert.NoError(t, err)

	var deployParams map[string]any
	err = json.Unmarshal([]byte(res.Transaction.ParamsJson), &deployParams)
	require.NoError(t, err)
	assert.Equal(t, "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71", deployParams["notaryAddress"])
	var deployData map[string]any
	err = json.Unmarshal(pldtypes.MustParseHexBytes(deployParams["data"].(string)), &deployData)
	require.NoError(t, err)
	assert.Equal(t, "notary@node1", deployData["notaryLookup"])
}

func TestPrepareDeployCheckFunction(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	res, err := n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "basic"
			}`,
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	assert.NoError(t, err)

	var fn abi.Entry
	err = json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &fn)
	require.NoError(t, err)
	assert.Equal(t, "deploy", fn.Name)

	res, err = n.PrepareDeploy(context.Background(), &prototk.PrepareDeployRequest{
		Transaction: &prototk.DeployTransactionSpecification{
			ConstructorParamsJson: `{
				"notary": "notary@node1",
				"notaryMode": "basic",
				"implementation": "alt-noto"
			}`,
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "notary@node1",
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
				Verifier:     "0x6e2430d15301a7ee28ceaaee0dff9781f8f82f71",
			},
		},
	})
	assert.NoError(t, err)

	err = json.Unmarshal([]byte(res.Transaction.FunctionAbiJson), &fn)
	require.NoError(t, err)
	assert.Equal(t, "deployImplementation", fn.Name)
}

func TestInitContractBadConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	res, err := n.InitContract(context.Background(), &prototk.InitContractRequest{
		ContractConfig: []byte("!!wrong"),
	})
	require.NoError(t, err)
	assert.False(t, res.Valid)
}

func TestInitContractBadNotary(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitContract(context.Background(), &prototk.InitContractRequest{
		ContractAddress: pldtypes.RandAddress().String(),
		ContractConfig:  encodedConfig(&types.NotoConfigData_V0{NotaryLookup: "notary@bad@notgood"}),
	})
	require.ErrorContains(t, err, "PD020006")
}

func TestInitTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionBadConfig(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `!!wrong`,
			},
			FunctionAbiJson: `{}`,
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionBadFunction(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
			},
			FunctionAbiJson: `{"name": "does-not-exist"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200001")
}

// TODO: rework this test because Function signature correctness is checked before contractAddress
// func TestInitTransactionBadAddress(t *testing.T) {
// 	n := &Noto{Callbacks: mockCallbacks}
// 	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
// 		Transaction: &prototk.TransactionSpecification{
// 			ContractInfo: &prototk.ContractInfo{
// 				ContractConfigJson: `{"notaryLookup":"notary"}`,
// 				ContractAddress:    "!!wrong",
// 			},
// 			FunctionAbiJson: `{"name": "transfer"}`,
// 		},
// 	})
// 	assert.ErrorContains(t, err, "bad address")
// }

func TestInitTransactionBadParams(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
				ContractAddress:    pldtypes.RandAddress().String(),
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestInitTransactionMissingTo(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
				ContractAddress:    pldtypes.RandAddress().String(),
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: "{}",
		},
	})
	assert.ErrorContains(t, err, "PD200007")
}

func TestInitTransactionMissingAmount(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
				ContractAddress:    pldtypes.RandAddress().String(),
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient"}`,
		},
	})
	assert.ErrorContains(t, err, "PD200008")
}

func TestInitTransactionBadSignature(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.InitTransaction(context.Background(), &prototk.InitTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractConfigJson: `{"notaryLookup":"notary"}`,
				ContractAddress:    pldtypes.RandAddress().String(),
			},
			FunctionAbiJson:    `{"name": "transfer"}`,
			FunctionParamsJson: `{"to": "recipient", "amount": 1}`,
		},
	})
	assert.ErrorContains(t, err, "PD200002")
}

func TestAssembleTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.AssembleTransaction(context.Background(), &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestEndorseTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.EndorseTransaction(context.Background(), &prototk.EndorseTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestPrepareTransactionBadAbi(t *testing.T) {
	n := &Noto{Callbacks: mockCallbacks}
	_, err := n.PrepareTransaction(context.Background(), &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			FunctionAbiJson: "!!wrong",
		},
	})
	assert.ErrorContains(t, err, "invalid character")
}

func TestUnimplementedMethods(t *testing.T) {
	n := &Noto{}
	ctx := context.Background()

	_, err := n.Sign(ctx, nil)
	assert.ErrorContains(t, err, "PD200022")

	_, err = n.GetVerifier(ctx, nil)
	assert.ErrorContains(t, err, "PD200022")

	_, err = n.ValidateStateHashes(ctx, nil)
	assert.ErrorContains(t, err, "PD200022")
}

func TestDecodeConfigInvalid(t *testing.T) {
	n := &Noto{}
	ctx := context.Background()

	_, _, err := n.decodeConfig(ctx, types.NotoConfigID_V0)
	assert.ErrorContains(t, err, "FF22047")
}

func TestRecoverSignatureInvalid(t *testing.T) {
	n := &Noto{}
	ctx := context.Background()

	_, err := n.recoverSignature(ctx, nil, nil)
	assert.ErrorContains(t, err, "FF22087")
}
