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
	"encoding/json"
	"fmt"
	"testing"

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
	pb "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

//go:embed abis/SIMDomain.json
var simDomainBuild []byte // comes from Hardhat build

func toJSONString(t *testing.T, v interface{}) string {
	b, err := json.Marshal(v)
	assert.NoError(t, err)
	return string(b)
}

// Example of how someone might use this testbed externally
func TestDemoNotarizedCoinSelection(t *testing.T) {

	simDomainABI := mustParseBuildABI(simDomainBuild)

	fakeCoinConstructorABI := `{
		"type": "constructor",
		"inputs": [
		  {
		    "name": "notary",
			"type": "string"
		  },
		  {
		    "name": "name",
			"type": "string"
		  },
		  {
		    "name": "symbol",
			"type": "string"
		  }
		],
		"outputs": null
	}`

	fakeCoinStateSchema := `{
		"type": "tuple",
		"internalType": "struct FakeCoin",
		"components": [
			{
				"name": "salt",
				"type": "bytes32"
			},
			{
				"name": "owner",
				"type": "address",
				"indexed": true
			},
			{
				"name": "amount",
				"type": "uint256",
				"indexed": true
			}
		]
	}`

	// Note, here we're simulating a domain that choose to support versions of a "Transfer" function
	// with "string" types (rather than "address") for the from/to address and to ask Paladin to do
	// verifier resolution for these. The same domain could also support "address" type inputs/outputs
	// in the same ABI.
	fakeCoinTransferABI := `{
		"type": "function",
		"inputs": [
		  {
		    "name": "from",
			"type": "address"
		  },
		  {
		    "name": "to",
			"type": "address"
		  },
		  {
		    "name": "value",
			"type": "uint256"
		  }
		],
		"outputs": null
	}`

	fakeDeployPayload := `{
		"notary": "domain1/contract1/notary",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`

	type fakeTransferParser struct {
		From   *string              `json:"from,omitempty"`
		To     *string              `json:"to,omitempty"`
		Amount *ethtypes.HexInteger `json:"amount"`
	}

	// fakeTransferMintPayload := `{
	// 	"from": null,
	// 	"to": "wallets/org1/aaaaaa",
	// 	"amount": "123000000000000000000"
	// }`

	// fakeTransferPayload1 := `{
	// 	"from": "wallets/org1/aaaaaa",
	// 	"to": "wallets/org2/bbbbbb",
	// 	"amount": "23000000000000000000"
	// }`

	var factoryAddr ethtypes.Address0xHex
	_, rpcCall, done := newDomainSimulator(t, map[protoreflect.FullName]domainSimulatorFn{

		CONFIGURE_DOMAIN: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.ConfigureDomainRequest](t, iReq)
			assert.Equal(t, "domain1", req.Name)
			assert.JSONEq(t, `{"some":"config"}`, req.ConfigYaml)
			assert.Equal(t, int64(1337), req.ChainId) // from tools/besu_bootstrap
			return &proto.ConfigureDomainResponse{
				DomainConfig: &proto.DomainConfig{
					ConstructorAbiJson:     fakeCoinConstructorABI,
					FactoryContractAddress: factoryAddr.String(), // note this requires testbed_deployBytecode to have completed
					FactoryContractAbiJson: toJSONString(t, simDomainABI),
					AbiStateSchemasJson:    []string{fakeCoinStateSchema},
				},
			}, nil
		},

		INIT_DOMAIN: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.InitDomainRequest](t, iReq)
			assert.Len(t, req.AbiStateSchemaIds, 1)
			return &proto.InitDomainResponse{}, nil
		},

		INIT_DEPLOY: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.InitDeployTransactionRequest](t, iReq)
			assert.JSONEq(t, fakeCoinConstructorABI, req.Transaction.ConstructorAbi)
			assert.JSONEq(t, fakeDeployPayload, req.Transaction.ConstructorParamsJson)
			return &proto.InitDeployTransactionResponse{
				RequiredVerifiers: []*proto.ResolveVerifierRequest{
					{
						Lookup:    "domain1/contract1/notary",
						Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
					},
				},
			}, nil
		},

		PREPARE_DEPLOY: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.PrepareDeployTransactionRequest](t, iReq)
			assert.JSONEq(t, fakeCoinConstructorABI, req.Transaction.ConstructorAbi)
			assert.JSONEq(t, `{
				"notary": "domain1/contract1/notary",
				"name": "FakeToken1",
				"symbol": "FT1"
			}`, req.Transaction.ConstructorParamsJson)
			assert.Len(t, req.Verifiers, 1)
			assert.Equal(t, signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, req.Verifiers[0].Algorithm)
			assert.Equal(t, "domain1/contract1/notary", req.Verifiers[0].Lookup)
			assert.NotEmpty(t, req.Verifiers[0].Verifier)
			return &proto.PrepareDeployTransactionResponse{
				Transaction: &proto.BaseLedgerTransaction{
					FunctionName: "newSIMTokenNotarized",
					ParamsJson: fmt.Sprintf(`{
						"txId": "%s",
						"notary": "%s"
					}`, req.Transaction.TransactionId, req.Verifiers[0].Verifier),
					SigningAddress: fmt.Sprintf("domain1/contract1/onetimekeys/%s", req.Transaction.TransactionId),
				},
			}, nil
		},

		INIT_TRANSACTION: func(iReq pb.Message) (pb.Message, error) {
			req := simRequestToProto[*proto.InitTransactionRequest](t, iReq)
			assert.JSONEq(t, fakeCoinTransferABI, req.Transaction.FunctionAbiJson)
			assert.Equal(t, "Transfer(string,string,uint256)", req.Transaction.FunctionSignature)
			var inputs fakeTransferParser
			err := json.Unmarshal([]byte(req.Transaction.FunctionParamsJson), &inputs)
			assert.NoError(t, err)
			assert.Greater(t, inputs.Amount.BigInt().Sign(), 0)
			assert.False(t, inputs.From == nil && inputs.To == nil)
			// We require ethereum addresses for the "from" and "to" addresses to actually
			// execute the transaction. See notes above about this.
			requiredVerifiers := []*proto.ResolveVerifierRequest{
				{
					Lookup:    "domain1/contract1/notary",
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				},
			}
			if inputs.From != nil {
				requiredVerifiers = append(requiredVerifiers, &proto.ResolveVerifierRequest{
					Lookup:    *inputs.From,
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				})
			}
			if inputs.To != nil && (inputs.From == nil || *inputs.From != *inputs.To) {
				requiredVerifiers = append(requiredVerifiers, &proto.ResolveVerifierRequest{
					Lookup:    *inputs.To,
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				})
			}
			return &proto.InitTransactionResponse{
				RequiredVerifiers: requiredVerifiers,
			}, nil
		},
	})
	defer done()

	err := rpcCall(&factoryAddr, "testbed_deployBytecode", "domain1_admin",
		mustParseBuildABI(simDomainBuild), mustParseBuildBytecode(simDomainBuild),
		types.RawJSON(`{}`)) // no params on constructor
	assert.NoError(t, err)

	err = rpcCall(types.RawJSON{}, "testbed_configureInit", "domain1", types.RawJSON(`{
		"some": "config"
	}`))
	assert.NoError(t, err)

	err = rpcCall(types.RawJSON{}, "testbed_configureInit", "domain1", types.RawJSON(`{
		"some": "config"
	}`))
	assert.NoError(t, err)

	var contractAddr *ethtypes.Address0xHex
	err = rpcCall(&contractAddr, "testbed_deploy", "domain1", types.RawJSON(`{
		"notary": "domain1/contract1/notary",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`))
	assert.NoError(t, err)

	// err = rpcCall(types.RawJSON{}, "testbed_invoke", &types.PrivateContractInvoke{
	// 	From:   "wallets/org1/aaaaaa",
	// 	To:     deployEvent.Address,
	// 	Inputs: types.RawJSON(fakeTransferMintPayload),
	// })
	// assert.NoError(t, err)

}
