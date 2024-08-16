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
	"strconv"
	"testing"

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
	pb "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

//go:embed abis/SIMDomain.json
var simDomainBuild []byte // comes from Hardhat build

//go:embed abis/SIMToken.json
var simTokenBuild []byte // comes from Hardhat build

func toJSONString(t *testing.T, v interface{}) string {
	b, err := json.Marshal(v)
	assert.NoError(t, err)
	return string(b)
}

// Example of how someone might use this testbed externally
func TestDemoNotarizedCoinSelection(t *testing.T) {

	simDomainABI := mustParseBuildABI(simDomainBuild)
	simTokenABI := mustParseBuildABI(simTokenBuild)

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
		"name": "transfer",
		"inputs": [
		  {
		    "name": "from",
			"type": "string"
		  },
		  {
		    "name": "to",
			"type": "string"
		  },
		  {
		    "name": "amount",
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
		From   string               `json:"from,omitempty"`
		To     string               `json:"to,omitempty"`
		Amount *ethtypes.HexInteger `json:"amount"`
	}

	type fakeCoinParser struct {
		Salt   ethtypes.HexBytes0xPrefix `json:"salt"`
		Owner  ethtypes.Address0xHex     `json:"owner"`
		Amount *ethtypes.HexInteger      `json:"amount"`
	}

	fakeTransferMintPayload := `{
		"from": "",
		"to": "wallets/org1/aaaaaa",
		"amount": "123000000000000000000"
	}`

	fakeTransferPayload1 := `{
		"from": "wallets/org1/aaaaaa",
		"to": "wallets/org2/bbbbbb",
		"amount": "23000000000000000000"
	}`

	contractDataABI := &abi.ParameterArray{
		{Name: "notaryLocator", Type: "string"},
	}

	type fakeCoinConfigParser struct {
		NotaryLocator string `json:"notaryLocator"`
	}

	var factoryAddr ethtypes.Address0xHex
	var fakeCoinSchemaID string
	var domainUUID string
	var chainID int64

	fakeCoinSelection := func(sc simCallbacks, fromAddr *ethtypes.Address0xHex, amount *big.Int) ([]*fakeCoinParser, []*proto.StateRef, *big.Int, error) {
		var lastStateTimestamp int64
		total := big.NewInt(0)
		coins := []*fakeCoinParser{}
		stateRefs := []*proto.StateRef{}
		for {
			// Simple oldest coin first algo
			query := &filters.QueryJSON{
				Limit: confutil.P(10),
				Sort:  []string{".created"},
				Statements: filters.Statements{
					Ops: filters.Ops{
						Eq: []*filters.OpSingleVal{
							{Op: filters.Op{Field: "owner"}, Value: types.JSONString(fromAddr.String())},
						},
					},
				},
			}
			if lastStateTimestamp > 0 {
				query.GT = []*filters.OpSingleVal{
					{Op: filters.Op{Field: ".created"}, Value: types.RawJSON(strconv.FormatInt(lastStateTimestamp, 10))},
				}
			}
			states, err := sc.FindAvailableStates(domainUUID, fakeCoinSchemaID, query)
			if err != nil {
				return nil, nil, nil, err
			}
			if len(states) == 0 {
				return nil, nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
			}
			for _, state := range states {
				lastStateTimestamp = state.StoredAt
				// Note: More sophisticated coin selection might prefer states that aren't locked to a sequence
				var coin fakeCoinParser
				if err := json.Unmarshal([]byte(state.DataJson), &coin); err != nil {
					return nil, nil, nil, fmt.Errorf("coin %s is invalid: %s", state.HashId, err)
				}
				total = total.Add(total, coin.Amount.BigInt())
				stateRefs = append(stateRefs, &proto.StateRef{
					HashId:   state.HashId,
					SchemaId: state.SchemaId,
				})
				coins = append(coins, &coin)
				if total.Cmp(amount) >= 0 {
					// We've got what we need - return how much over we are
					return coins, stateRefs, new(big.Int).Sub(total, amount), nil
				}
			}
		}
	}

	validateTransferTransactionInput := func(tx *proto.TransactionSpecification) (*ethtypes.Address0xHex, string, *fakeTransferParser) {
		assert.JSONEq(t, fakeCoinTransferABI, tx.FunctionAbiJson)
		assert.Equal(t, "transfer(string,string,uint256)", tx.FunctionSignature)
		var inputs fakeTransferParser
		err := json.Unmarshal([]byte(tx.FunctionParamsJson), &inputs)
		assert.NoError(t, err)
		assert.Greater(t, inputs.Amount.BigInt().Sign(), 0)
		contractAddr, err := ethtypes.NewAddress(tx.ContractAddress)
		assert.NoError(t, err)
		configValues, err := contractDataABI.DecodeABIData(tx.ContractConfig, 0)
		assert.NoError(t, err)
		configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
		assert.NoError(t, err)
		var config fakeCoinConfigParser
		err = json.Unmarshal(configJSON, &config)
		assert.NoError(t, err)
		assert.NotEmpty(t, config.NotaryLocator)
		return contractAddr, config.NotaryLocator, &inputs
	}

	extractTransferVerifiers := func(txInputs *fakeTransferParser, verifiers []*proto.ResolvedVerifier) (fromAddr, toAddr *ethtypes.Address0xHex) {
		for _, v := range verifiers {
			if txInputs.From != "" && v.Lookup == txInputs.From {
				fromAddr = ethtypes.MustNewAddress(v.Verifier)
			}
			if txInputs.To != "" && v.Lookup == txInputs.To {
				toAddr = ethtypes.MustNewAddress(v.Verifier)
			}
		}
		assert.True(t, txInputs.From == "" || (fromAddr != nil && *fromAddr != ethtypes.Address0xHex{}))
		assert.True(t, txInputs.To == "" || (toAddr != nil && *toAddr != ethtypes.Address0xHex{}))
		return
	}

	typedDataV4TransferWithSalts := func(contract *ethtypes.Address0xHex, inputs, outputs []*fakeCoinParser) (ethtypes.HexBytes0xPrefix, error) {
		typeSet := eip712.TypeSet{
			"FakeTransfer": {
				{Name: "inputs", Type: "Coin[]"},
				{Name: "outputs", Type: "Coin[]"},
			},
			"Coin": {
				{Name: "salt", Type: "bytes32"},
				{Name: "owner", Type: "address"},
				{Name: "amount", Type: "uint256"},
			},
			eip712.EIP712Domain: {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
		}
		messageInputs := make([]interface{}, len(inputs))
		for i, input := range inputs {
			messageInputs[i] = map[string]interface{}{
				"salt":   input.Salt.String(),
				"owner":  input.Owner.String(),
				"amount": input.Amount.String(),
			}
		}
		messageOutputs := make([]interface{}, len(outputs))
		for i, output := range outputs {
			messageOutputs[i] = map[string]interface{}{
				"salt":   output.Salt.String(),
				"owner":  output.Owner.String(),
				"amount": output.Amount.String(),
			}
		}
		return eip712.EncodeTypedDataV4(context.Background(), &eip712.TypedData{
			Types:       typeSet,
			PrimaryType: "FakeTransfer",
			Domain: map[string]interface{}{
				"name":              "FakeTransfer",
				"version":           "0.0.1",
				"chainId":           chainID,
				"verifyingContract": contract,
			},
			Message: map[string]interface{}{
				"inputs":  messageInputs,
				"outputs": messageOutputs,
			},
		})
	}

	_, rpcCall, done := newDomainSimulator(t, map[protoreflect.FullName]domainSimulatorFn{

		CONFIGURE_DOMAIN: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.ConfigureDomainRequest)
			assert.Equal(t, "domain1", req.Name)
			assert.JSONEq(t, `{"some":"config"}`, req.ConfigYaml)
			assert.Equal(t, int64(1337), req.ChainId) // from tools/besu_bootstrap
			chainID = req.ChainId
			return &proto.ConfigureDomainResponse{
				DomainConfig: &proto.DomainConfig{
					ConstructorAbiJson:     fakeCoinConstructorABI,
					FactoryContractAddress: factoryAddr.String(), // note this requires testbed_deployBytecode to have completed
					FactoryContractAbiJson: toJSONString(t, simDomainABI),
					PrivateContractAbiJson: toJSONString(t, simTokenABI),
					AbiStateSchemasJson:    []string{fakeCoinStateSchema},
				},
			}, nil
		},

		INIT_DOMAIN: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.InitDomainRequest)
			assert.Len(t, req.AbiStateSchemas, 1)
			fakeCoinSchemaID = req.AbiStateSchemas[0].Id
			assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", req.AbiStateSchemas[0].Signature)
			domainUUID = req.DomainUuid
			return &proto.InitDomainResponse{}, nil
		},

		INIT_DEPLOY: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.InitDeployRequest)
			assert.JSONEq(t, fakeCoinConstructorABI, req.Transaction.ConstructorAbi)
			assert.JSONEq(t, fakeDeployPayload, req.Transaction.ConstructorParamsJson)
			return &proto.InitDeployResponse{
				RequiredVerifiers: []*proto.ResolveVerifierRequest{
					{
						Lookup:    "domain1/contract1/notary",
						Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
					},
				},
			}, nil
		},

		PREPARE_DEPLOY: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.PrepareDeployRequest)
			assert.JSONEq(t, fakeCoinConstructorABI, req.Transaction.ConstructorAbi)
			assert.JSONEq(t, `{
				"notary": "domain1/contract1/notary",
				"name": "FakeToken1",
				"symbol": "FT1"
			}`, req.Transaction.ConstructorParamsJson)
			assert.Len(t, req.ResolvedVerifiers, 1)
			assert.Equal(t, signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES, req.ResolvedVerifiers[0].Algorithm)
			assert.Equal(t, "domain1/contract1/notary", req.ResolvedVerifiers[0].Lookup)
			assert.NotEmpty(t, req.ResolvedVerifiers[0].Verifier)
			return &proto.PrepareDeployResponse{
				SigningAddress: fmt.Sprintf("domain1/transactions/%s", req.Transaction.TransactionId),
				Transaction: &proto.BaseLedgerTransaction{
					FunctionName: "newSIMTokenNotarized",
					ParamsJson: fmt.Sprintf(`{
						"txId": "%s",
						"notary": "%s",
						"notaryLocator": "domain1/contract1/notary"
					}`, req.Transaction.TransactionId, req.ResolvedVerifiers[0].Verifier),
				},
			}, nil
		},

		INIT_TRANSACTION: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.InitTransactionRequest)
			_, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)

			// We require ethereum addresses for the "from" and "to" addresses to actually
			// execute the transaction. See notes above about this.
			requiredVerifiers := []*proto.ResolveVerifierRequest{
				{
					Lookup:    notaryLocator,
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				},
			}
			if txInputs.From != "" {
				requiredVerifiers = append(requiredVerifiers, &proto.ResolveVerifierRequest{
					Lookup:    txInputs.From,
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				})
			}
			if txInputs.To != "" && (txInputs.From == "" || txInputs.From != txInputs.To) {
				requiredVerifiers = append(requiredVerifiers, &proto.ResolveVerifierRequest{
					Lookup:    txInputs.To,
					Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				})
			}
			return &proto.InitTransactionResponse{
				RequiredVerifiers: requiredVerifiers,
			}, nil
		},

		ASSEMBLE_TRANSACTION: func(sc simCallbacks, iReq pb.Message) (_ pb.Message, err error) {
			req := iReq.(*proto.AssembleTransactionRequest)
			contractAddr, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)
			fromAddr, toAddr := extractTransferVerifiers(txInputs, req.ResolvedVerifiers)
			amount := txInputs.Amount.BigInt()
			toKeep := new(big.Int)
			coinsToSpend := []*fakeCoinParser{}
			stateRefsToSpend := []*proto.StateRef{}
			if txInputs.From != "" {
				coinsToSpend, stateRefsToSpend, toKeep, err = fakeCoinSelection(sc, fromAddr, amount)
				if err != nil {
					return nil, err
				}
			}
			newStates := []*proto.NewState{}
			newCoins := []*fakeCoinParser{}
			if fromAddr != nil && toKeep.Sign() > 0 {
				// Generate a state to keep for ourselves
				coin := fakeCoinParser{
					Salt:   types.RandBytes(32),
					Owner:  *fromAddr,
					Amount: (*ethtypes.HexInteger)(toKeep),
				}
				newCoins = append(newCoins, &coin)
				newStates = append(newStates, &proto.NewState{
					SchemaId:      fakeCoinSchemaID,
					StateDataJson: toJSONString(t, &coin),
				})
			}
			if toAddr != nil && amount.Sign() > 0 {
				// Generate the coin to transfer
				coin := fakeCoinParser{
					Salt:   types.RandBytes(32),
					Owner:  *toAddr,
					Amount: (*ethtypes.HexInteger)(amount),
				}
				newCoins = append(newCoins, &coin)
				newStates = append(newStates, &proto.NewState{
					SchemaId:      fakeCoinSchemaID,
					StateDataJson: toJSONString(t, &coin),
				})
			}
			eip712Payload, err := typedDataV4TransferWithSalts(contractAddr, coinsToSpend, newCoins)
			assert.NoError(t, err)
			return &proto.AssembleTransactionResponse{
				AssembledTransaction: &proto.AssembledTransaction{
					SpentStates: stateRefsToSpend,
					NewStates:   newStates,
				},
				AssemblyResult: proto.AssembleTransactionResponse_OK,
				AttestationPlan: []*proto.AttestationRequest{
					{
						Name:            "sender",
						AttestationType: proto.AttestationType_SIGN,
						Algorithm:       signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
						Payload:         eip712Payload,
						Parties: []string{
							req.Transaction.From,
						},
					},
					{
						Name:            "notary",
						AttestationType: proto.AttestationType_ENDORSE,
						// we expect an endorsement is of the form ENDORSER_SUBMIT - so we need an eth signing key to exist
						Algorithm: signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
						Parties: []string{
							notaryLocator,
						},
					},
				},
			}, nil
		},

		ENDORSE_TRANSACTION: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.EndorseTransactionRequest)
			contractAddr, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)
			fromAddr, toAddr := extractTransferVerifiers(txInputs, req.ResolvedVerifiers)
			assert.Equal(t, req.EndorsementVerifier.Lookup, notaryLocator)

			inCoins := make([]*fakeCoinParser, len(req.Inputs))
			for i, input := range req.Inputs {
				assert.Equal(t, fakeCoinSchemaID, input.SchemaId)
				if err := json.Unmarshal([]byte(input.StateDataJson), &inCoins[i]); err != nil {
					return nil, fmt.Errorf("invalid input[%d] (%s): %s", i, input.HashId, err)
				}
			}
			outCoins := make([]*fakeCoinParser, len(req.Outputs))
			for i, output := range req.Outputs {
				assert.Equal(t, fakeCoinSchemaID, output.SchemaId)
				if err := json.Unmarshal([]byte(output.StateDataJson), &outCoins[i]); err != nil {
					return nil, fmt.Errorf("invalid output[%d] (%s): %s", i, output.HashId, err)
				}
			}

			// Recover the signature
			signaturePayload, err := typedDataV4TransferWithSalts(contractAddr, inCoins, outCoins)
			assert.NoError(t, err)
			var signerVerification *proto.AttestationResult
			for _, ar := range req.Signatures {
				if ar.AttestationType == proto.AttestationType_SIGN &&
					ar.Name == "sender" &&
					ar.Verifier.Algorithm == signer.Algorithm_ECDSA_SECP256K1_PLAINBYTES {
					signerVerification = ar
					break
				}
			}
			assert.NotNil(t, signerVerification)
			sig, err := secp256k1.DecodeCompactRSV(context.Background(), signerVerification.Payload)
			assert.NoError(t, err)
			signerAddr, err := sig.RecoverDirect(signaturePayload, chainID)
			assert.NoError(t, err)

			// There would need to be minting/spending rules here - we just check the signature
			assert.Equal(t, signerAddr.String(), signerVerification.Verifier.Verifier)

			// Check the math
			if fromAddr != nil && toAddr != nil {
				inTotal := big.NewInt(0)
				for _, c := range inCoins {
					inTotal = inTotal.Add(inTotal, c.Amount.BigInt())
				}
				outTotal := big.NewInt(0)
				for _, c := range outCoins {
					outTotal = outTotal.Add(outTotal, c.Amount.BigInt())
				}
				assert.True(t, inTotal.Cmp(outTotal) == 0)
			} else {
				if fromAddr == nil {
					assert.Len(t, inCoins, 0)
				}
				if toAddr == nil {
					assert.Len(t, outCoins, 0)
				}
			}

			return &proto.EndorseTransactionResponse{
				EndorsementResult: proto.EndorseTransactionResponse_ENDORSER_SUBMIT,
			}, nil
		},

		PREPARE_TRANSACTION: func(_ simCallbacks, iReq pb.Message) (pb.Message, error) {
			req := iReq.(*proto.PrepareTransactionRequest)
			var signerSignature ethtypes.HexBytes0xPrefix
			for _, att := range req.AttestationResult {
				if att.AttestationType == proto.AttestationType_SIGN && att.Name == "sender" {
					signerSignature = att.Payload
				}
			}
			spentStateIds := make([]string, len(req.Transaction.SpentStates))
			for i, s := range req.Transaction.SpentStates {
				spentStateIds[i] = s.HashId
			}
			newStateIds := make([]string, len(req.Transaction.NewStates))
			for i, s := range req.Transaction.NewStates {
				newStateIds[i] = s.HashId
			}
			return &proto.PrepareTransactionResponse{
				Transaction: &proto.BaseLedgerTransaction{
					FunctionName: "executeNotarized",
					ParamsJson: toJSONString(t, map[string]interface{}{
						"txId":      req.Transaction.TransactionId,
						"inputs":    spentStateIds,
						"outputs":   newStateIds,
						"signature": signerSignature,
					}),
				},
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

	var contractAddr ethtypes.Address0xHex
	err = rpcCall(&contractAddr, "testbed_deploy", "domain1", types.RawJSON(`{
		"notary": "domain1/contract1/notary",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`))
	assert.NoError(t, err)

	err = rpcCall(types.RawJSON{}, "testbed_invoke", &types.PrivateContractInvoke{
		From:     "wallets/org1/aaaaaa",
		To:       types.EthAddress(contractAddr),
		Function: *mustParseABIEntry(fakeCoinTransferABI),
		Inputs:   types.RawJSON(fakeTransferMintPayload),
	})
	assert.NoError(t, err)

	err = rpcCall(types.RawJSON{}, "testbed_invoke", &types.PrivateContractInvoke{
		From:     "wallets/org1/aaaaaa",
		To:       types.EthAddress(contractAddr),
		Function: *mustParseABIEntry(fakeCoinTransferABI),
		Inputs:   types.RawJSON(fakeTransferPayload1),
	})
	assert.NoError(t, err)

}
