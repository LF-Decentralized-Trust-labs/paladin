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
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"testing"
	"time"

	_ "embed"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/SIMDomain.json
var simDomainBuild []byte // comes from Hardhat build

//go:embed abis/SIMToken.json
var simTokenBuild []byte // comes from Hardhat build

func toJSONString(t *testing.T, v interface{}) string {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

type UTXOTransfer_Event struct {
	TX        pldtypes.Bytes32   `json:"txId"`
	Inputs    []pldtypes.Bytes32 `json:"inputs"`
	Outputs   []pldtypes.Bytes32 `json:"outputs"`
	Signature pldtypes.HexBytes  `json:"signature"`
}

func parseStatesFromEvent(txID pldtypes.Bytes32, states []pldtypes.Bytes32) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

// Example of how someone might use this testbed externally
func TestDemoNotarizedCoinSelection(t *testing.T) {

	ctx := context.Background()
	simDomainABI := mustParseBuildABI(simDomainBuild)
	simTokenABI := mustParseBuildABI(simTokenBuild)

	transferABI := simTokenABI.Events()["UTXOTransfer"]
	require.NotEmpty(t, transferABI)
	transferSignature := transferABI.SolString()

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

	fakeCoinGetBalanceABI := `{
		"type": "function",
		"name": "getBalance",
		"inputs": [
		  {
		    "name": "account",
			"type": "string"
		  }
		],
		"outputs": [
		  {
		    "name": "amount",
			"type": "uint256"
		  }
		]
	}`

	fakeCoinABI := abi.ABI{
		mustParseABIEntry(fakeCoinTransferABI),
		mustParseABIEntry(fakeCoinGetBalanceABI),
	}

	fakeDeployPayload := `{
		"notary": "domain1.contract1.notary",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`

	type fakeTransferParser struct {
		From   string               `json:"from,omitempty"`
		To     string               `json:"to,omitempty"`
		Amount *ethtypes.HexInteger `json:"amount"`
	}

	type fakeCoinParser struct {
		Salt   pldtypes.HexBytes     `json:"salt"`
		Owner  ethtypes.Address0xHex `json:"owner"`
		Amount *ethtypes.HexInteger  `json:"amount"`
	}

	type getBalanceParser struct {
		Account string `json:"account"`
	}

	type getBalanceResult struct {
		Amount *pldtypes.HexUint256 `json:"amount"`
	}

	contractDataABI := &abi.ParameterArray{
		{Name: "notaryLocator", Type: "string"},
	}

	type fakeCoinConfigParser struct {
		NotaryLocator string `json:"notaryLocator"`
	}

	fakeCoinDomain := plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {

		var fakeCoinSchemaID string
		var chainID int64

		fakeCoinSelection := func(ctx context.Context, stateQueryContext string, fromAddr *ethtypes.Address0xHex, amount *big.Int) ([]*fakeCoinParser, []*prototk.StateRef, *big.Int, error) {
			var lastStateTimestamp int64
			total := big.NewInt(0)
			coins := []*fakeCoinParser{}
			stateRefs := []*prototk.StateRef{}
			for {
				// Simple oldest coin first algo
				jq := &query.QueryJSON{
					Limit: confutil.P(10),
					Sort:  []string{".created"},
					Statements: query.Statements{
						Ops: query.Ops{
							Eq: []*query.OpSingleVal{
								{Op: query.Op{Field: "owner"}, Value: pldtypes.JSONString(fromAddr.String())},
							},
						},
					},
				}
				if lastStateTimestamp > 0 {
					jq.GT = []*query.OpSingleVal{
						{Op: query.Op{Field: ".created"}, Value: pldtypes.RawJSON(strconv.FormatInt(lastStateTimestamp, 10))},
					}
				}
				res, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
					StateQueryContext: stateQueryContext,
					SchemaId:          fakeCoinSchemaID,
					QueryJson:         pldtypes.JSONString(jq).String(),
				})
				if err != nil {
					return nil, nil, nil, err
				}
				states := res.States
				if len(states) == 0 {
					return nil, nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
				}
				for _, state := range states {
					lastStateTimestamp = state.CreatedAt
					// Note: More sophisticated coin selection might prefer states that aren't locked to a sequence
					var coin fakeCoinParser
					if err := json.Unmarshal([]byte(state.DataJson), &coin); err != nil {
						return nil, nil, nil, fmt.Errorf("coin %s is invalid: %s", state.Id, err)
					}
					total = total.Add(total, coin.Amount.BigInt())
					stateRefs = append(stateRefs, &prototk.StateRef{
						Id:       state.Id,
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

		validateTransferTransactionInput := func(tx *prototk.TransactionSpecification) (*ethtypes.Address0xHex, string, *fakeTransferParser) {
			assert.JSONEq(t, fakeCoinTransferABI, tx.FunctionAbiJson)
			assert.Equal(t, "function transfer(string memory from, string memory to, uint256 amount) external { }", tx.FunctionSignature)
			var inputs fakeTransferParser
			err := json.Unmarshal([]byte(tx.FunctionParamsJson), &inputs)
			require.NoError(t, err)
			assert.Greater(t, inputs.Amount.BigInt().Sign(), 0)
			contractAddr, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
			require.NoError(t, err)
			var config fakeCoinConfigParser
			err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &config)
			require.NoError(t, err)
			assert.NotEmpty(t, config.NotaryLocator)
			return contractAddr, config.NotaryLocator, &inputs
		}

		extractTransferVerifiers := func(txSpec *prototk.TransactionSpecification, txInputs *fakeTransferParser, verifiers []*prototk.ResolvedVerifier) (senderAddr, fromAddr, toAddr *ethtypes.Address0xHex) {
			for _, v := range verifiers {
				if txSpec.From != "" && v.Lookup == txSpec.From {
					senderAddr = ethtypes.MustNewAddress(v.Verifier)
				}
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

		typedDataV4TransferWithSalts := func(contract *ethtypes.Address0xHex, inputs, outputs []*fakeCoinParser) (pldtypes.HexBytes, error) {
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
			tdv4, err := eip712.EncodeTypedDataV4(context.Background(), &eip712.TypedData{
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
			return pldtypes.HexBytes(tdv4), err
		}

		return &plugintk.DomainAPIBase{Functions: &plugintk.DomainAPIFunctions{

			ConfigureDomain: func(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
				assert.Equal(t, "domain1", req.Name)
				assert.JSONEq(t, `{"some":"config"}`, req.ConfigJson)
				assert.Equal(t, int64(1337), req.ChainId) // from tools/besu_bootstrap
				chainID = req.ChainId

				var eventsABI abi.ABI
				eventsABI = append(eventsABI, transferABI)
				eventsJSON, err := json.Marshal(eventsABI)
				require.NoError(t, err)

				return &prototk.ConfigureDomainResponse{
					DomainConfig: &prototk.DomainConfig{
						AbiStateSchemasJson: []string{fakeCoinStateSchema},
						AbiEventsJson:       string(eventsJSON),
					},
				}, nil
			},

			InitDomain: func(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
				assert.Len(t, req.AbiStateSchemas, 1)
				fakeCoinSchemaID = req.AbiStateSchemas[0].Id
				assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", req.AbiStateSchemas[0].Signature)
				return &prototk.InitDomainResponse{}, nil
			},

			InitDeploy: func(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
				assert.JSONEq(t, fakeDeployPayload, req.Transaction.ConstructorParamsJson)
				return &prototk.InitDeployResponse{
					RequiredVerifiers: []*prototk.ResolveVerifierRequest{
						{
							Lookup:       "domain1.contract1.notary",
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
						},
					},
				}, nil
			},

			PrepareDeploy: func(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
				assert.JSONEq(t, `{
					"notary": "domain1.contract1.notary",
					"name": "FakeToken1",
					"symbol": "FT1"
				}`, req.Transaction.ConstructorParamsJson)
				assert.Len(t, req.ResolvedVerifiers, 1)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, req.ResolvedVerifiers[0].Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, req.ResolvedVerifiers[0].VerifierType)
				assert.Equal(t, "domain1.contract1.notary", req.ResolvedVerifiers[0].Lookup)
				assert.NotEmpty(t, req.ResolvedVerifiers[0].Verifier)
				return &prototk.PrepareDeployResponse{
					Signer: confutil.P(fmt.Sprintf("domain1.transactions.%s", req.Transaction.TransactionId)),
					Transaction: &prototk.PreparedTransaction{
						FunctionAbiJson: toJSONString(t, simDomainABI.Functions()["newSIMTokenNotarized"]),
						ParamsJson: fmt.Sprintf(`{
							"txId": "%s",
							"notary": "%s",
							"notaryLocator": "domain1.contract1.notary"
						}`, req.Transaction.TransactionId, req.ResolvedVerifiers[0].Verifier),
					},
				}, nil
			},

			InitContract: func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {

				configValues, err := contractDataABI.DecodeABIData(icr.ContractConfig, 0)
				str := pldtypes.HexBytes(icr.ContractConfig).HexString0xPrefix()
				assert.NotEqual(t, "", str)
				require.NoError(t, err)

				configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
				require.NoError(t, err)
				contractConfig := &prototk.ContractConfig{
					ContractConfigJson:   string(configJSON),
					CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
					SubmitterSelection:   prototk.ContractConfig_SUBMITTER_COORDINATOR,
				}
				var constructorParameters fakeCoinConfigParser
				err = json.Unmarshal([]byte(configJSON), &constructorParameters)
				require.NoError(t, err)
				return &prototk.InitContractResponse{
					Valid:          true,
					ContractConfig: contractConfig,
				}, nil
			},

			InitTransaction: func(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
				_, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)

				// We require ethereum addresses for the "from" and "to" addresses to actually
				// execute the transaction. See notes above about this.
				requiredVerifiers := []*prototk.ResolveVerifierRequest{
					{
						Lookup:       req.Transaction.From,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					},
					{
						Lookup:       notaryLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					},
				}
				if txInputs.From != "" {
					requiredVerifiers = append(requiredVerifiers, &prototk.ResolveVerifierRequest{
						Lookup:       txInputs.From,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					})
				}
				if txInputs.To != "" && (txInputs.From == "" || txInputs.From != txInputs.To) {
					requiredVerifiers = append(requiredVerifiers, &prototk.ResolveVerifierRequest{
						Lookup:       txInputs.To,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					})
				}
				return &prototk.InitTransactionResponse{
					RequiredVerifiers: requiredVerifiers,
				}, nil
			},

			AssembleTransaction: func(ctx context.Context, req *prototk.AssembleTransactionRequest) (_ *prototk.AssembleTransactionResponse, err error) {
				contractAddr, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)
				_, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)
				amount := txInputs.Amount.BigInt()
				toKeep := new(big.Int)
				coinsToSpend := []*fakeCoinParser{}
				stateRefsToSpend := []*prototk.StateRef{}
				if txInputs.From != "" {
					coinsToSpend, stateRefsToSpend, toKeep, err = fakeCoinSelection(ctx, req.StateQueryContext, fromAddr, amount)
					if err != nil {
						return nil, err
					}
				}
				newStates := []*prototk.NewState{}
				newCoins := []*fakeCoinParser{}
				if fromAddr != nil && toKeep.Sign() > 0 {
					// Generate a state to keep for ourselves
					coin := fakeCoinParser{
						Salt:   pldtypes.RandBytes(32),
						Owner:  *fromAddr,
						Amount: (*ethtypes.HexInteger)(toKeep),
					}
					newCoins = append(newCoins, &coin)
					newStates = append(newStates, &prototk.NewState{
						SchemaId:      fakeCoinSchemaID,
						StateDataJson: toJSONString(t, &coin),
					})
				}
				if toAddr != nil && amount.Sign() > 0 {
					// Generate the coin to transfer
					coin := fakeCoinParser{
						Salt:   pldtypes.RandBytes(32),
						Owner:  *toAddr,
						Amount: (*ethtypes.HexInteger)(amount),
					}
					newCoins = append(newCoins, &coin)
					newStates = append(newStates, &prototk.NewState{
						SchemaId:      fakeCoinSchemaID,
						StateDataJson: toJSONString(t, &coin),
					})
				}
				eip712Payload, err := typedDataV4TransferWithSalts(contractAddr, coinsToSpend, newCoins)
				require.NoError(t, err)
				return &prototk.AssembleTransactionResponse{
					AssembledTransaction: &prototk.AssembledTransaction{
						InputStates:  stateRefsToSpend,
						OutputStates: newStates,
					},
					AssemblyResult: prototk.AssembleTransactionResponse_OK,
					AttestationPlan: []*prototk.AttestationRequest{
						{
							Name:            "sender",
							AttestationType: prototk.AttestationType_SIGN,
							Algorithm:       algorithms.ECDSA_SECP256K1,
							VerifierType:    verifiers.ETH_ADDRESS,
							Payload:         eip712Payload,
							PayloadType:     signpayloads.OPAQUE_TO_RSV,
							Parties: []string{
								req.Transaction.From,
							},
						},
						{
							Name:            "notary",
							AttestationType: prototk.AttestationType_ENDORSE,
							// we expect an endorsement is of the form ENDORSER_SUBMIT - so we need an eth signing key to exist
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
							Parties: []string{
								notaryLocator,
							},
						},
					},
				}, nil
			},

			EndorseTransaction: func(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
				contractAddr, notaryLocator, txInputs := validateTransferTransactionInput(req.Transaction)
				senderAddr, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)
				assert.Equal(t, req.EndorsementVerifier.Lookup, req.EndorsementRequest.Parties[0])
				assert.Equal(t, req.EndorsementVerifier.Lookup, notaryLocator+"@node1" /* all identities get fully qualified on the journey */)

				inCoins := make([]*fakeCoinParser, len(req.Inputs))
				for i, input := range req.Inputs {
					assert.Equal(t, fakeCoinSchemaID, input.SchemaId)
					if err := json.Unmarshal([]byte(input.StateDataJson), &inCoins[i]); err != nil {
						return nil, fmt.Errorf("invalid input[%d] (%s): %s", i, input.Id, err)
					}
				}
				outCoins := make([]*fakeCoinParser, len(req.Outputs))
				for i, output := range req.Outputs {
					assert.Equal(t, fakeCoinSchemaID, output.SchemaId)
					if err := json.Unmarshal([]byte(output.StateDataJson), &outCoins[i]); err != nil {
						return nil, fmt.Errorf("invalid output[%d] (%s): %s", i, output.Id, err)
					}
				}

				// Recover the signature
				signaturePayload, err := typedDataV4TransferWithSalts(contractAddr, inCoins, outCoins)
				require.NoError(t, err)
				var signerVerification *prototk.AttestationResult
				for _, ar := range req.Signatures {
					if ar.AttestationType == prototk.AttestationType_SIGN &&
						ar.Name == "sender" &&
						ar.Verifier.Algorithm == algorithms.ECDSA_SECP256K1 &&
						ar.Verifier.VerifierType == verifiers.ETH_ADDRESS {
						signerVerification = ar
						break
					}
				}
				assert.NotNil(t, signerVerification)
				sig, err := secp256k1.DecodeCompactRSV(context.Background(), signerVerification.Payload)
				require.NoError(t, err)
				signerAddr, err := sig.RecoverDirect(signaturePayload, chainID)
				require.NoError(t, err)

				// There would need to be minting/spending rules here - we just check the signature
				assert.Equal(t, signerAddr.String(), signerVerification.Verifier.Verifier)

				// Check the math
				if fromAddr != nil && toAddr != nil {
					assert.Equal(t, senderAddr, fromAddr)
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
					// NOTE: No minting controls in this demo example
					if fromAddr == nil {
						assert.Len(t, inCoins, 0)
					}
					if toAddr == nil {
						assert.Len(t, outCoins, 0)
					}
				}

				return &prototk.EndorseTransactionResponse{
					EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
				}, nil
			},

			PrepareTransaction: func(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
				var signerSignature pldtypes.HexBytes
				for _, att := range req.AttestationResult {
					if att.AttestationType == prototk.AttestationType_SIGN && att.Name == "sender" {
						signerSignature = att.Payload
					}
				}
				spentStateIds := make([]string, len(req.InputStates))
				for i, s := range req.InputStates {
					spentStateIds[i] = s.Id
				}
				newStateIds := make([]string, len(req.OutputStates))
				for i, s := range req.OutputStates {
					newStateIds[i] = s.Id
				}
				return &prototk.PrepareTransactionResponse{
					Transaction: &prototk.PreparedTransaction{
						FunctionAbiJson: toJSONString(t, simTokenABI.Functions()["executeNotarized"]),
						ParamsJson: toJSONString(t, map[string]interface{}{
							"txId":      req.Transaction.TransactionId,
							"inputs":    spentStateIds,
							"outputs":   newStateIds,
							"signature": signerSignature,
						}),
					},
				}, nil
			},

			HandleEventBatch: func(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
				var res prototk.HandleEventBatchResponse
				for _, ev := range req.Events {
					switch ev.SoliditySignature {
					case transferSignature:
						var transfer UTXOTransfer_Event
						if err := json.Unmarshal([]byte(ev.DataJson), &transfer); err == nil {
							res.TransactionsComplete = append(res.TransactionsComplete, &prototk.CompletedTransaction{
								TransactionId: transfer.TX.String(),
								Location:      ev.Location,
							})
							res.SpentStates = append(res.SpentStates, parseStatesFromEvent(transfer.TX, transfer.Inputs)...)
							res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(transfer.TX, transfer.Outputs)...)
						}
					}
				}
				return &res, nil
			},

			InitCall: func(ctx context.Context, icr *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
				tx := icr.Transaction
				assert.JSONEq(t, fakeCoinGetBalanceABI, tx.FunctionAbiJson)
				assert.Equal(t, "function getBalance(string memory account) external returns (uint256 amount) { }", tx.FunctionSignature)
				var inputs *getBalanceParser
				err := json.Unmarshal([]byte(icr.Transaction.FunctionParamsJson), &inputs)
				require.NoError(t, err)
				return &prototk.InitCallResponse{
					RequiredVerifiers: []*prototk.ResolveVerifierRequest{
						{
							Lookup:       inputs.Account,
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
						},
					},
				}, nil
			},

			ExecCall: func(ctx context.Context, ecr *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
				tx := ecr.Transaction
				assert.JSONEq(t, fakeCoinGetBalanceABI, tx.FunctionAbiJson)
				assert.Equal(t, "function getBalance(string memory account) external returns (uint256 amount) { }", tx.FunctionSignature)
				balance := new(big.Int)
				var limit = 10
				var lastState *prototk.StoredState
				for {
					jq := query.NewQueryBuilder().
						Sort("-.created", "-.id").
						Limit(limit).
						Equal("owner", ecr.ResolvedVerifiers[0].Verifier)
					if lastState != nil {
						jq = jq.Or(
							query.NewQueryBuilder().LessThan(".created", lastState.CreatedAt),
							query.NewQueryBuilder().Equal(".created", lastState.CreatedAt).LessThan(".id", lastState.Id),
						)
					}
					res, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
						StateQueryContext: ecr.StateQueryContext,
						SchemaId:          fakeCoinSchemaID,
						QueryJson:         jq.Query().String(),
					})
					require.NoError(t, err)

					for _, state := range res.States {
						var coin fakeCoinParser
						err := json.Unmarshal([]byte(state.DataJson), &coin)
						require.NoError(t, err)
						balance = balance.Add(balance, coin.Amount.BigInt())
						lastState = state
					}

					if len(res.States) < limit {
						break
					}

				}
				return &prototk.ExecCallResponse{
					ResultJson: pldtypes.JSONString(&getBalanceResult{
						Amount: (*pldtypes.HexUint256)(balance),
					}).Pretty(),
				}, nil
			},
		}}
	})

	confFile := writeTestConfig(t)
	factoryContractAddress := deploySmartContract(t, confFile)
	tb := NewTestBed()
	url, _, done, err := tb.StartForTest(confFile, map[string]*TestbedDomain{
		"domain1": {
			Plugin:          fakeCoinDomain,
			Config:          map[string]any{"some": "config"},
			RegistryAddress: factoryContractAddress,
		},
	})
	require.NoError(t, err)
	defer done()

	tbRPC := rpcclient.WrapRestyClient(resty.New().SetBaseURL(url))

	var contractAddr pldtypes.EthAddress
	rpcErr := tbRPC.CallRPC(ctx, &contractAddr, "testbed_deploy", "domain1", "me", pldtypes.RawJSON(`{
		"notary": "domain1.contract1.notary",
		"name": "FakeToken1",
		"symbol": "FT1"
	}`))
	assert.NoError(t, rpcErr)

	rpcErr = tbRPC.CallRPC(ctx, pldtypes.RawJSON{}, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "wallets.org1.aaaaaa",
			To:       &contractAddr,
			Function: "transfer",
			Data: pldtypes.RawJSON(`{
				"from": "",
				"to": "wallets.org1.aaaaaa",
				"amount": "123000000000000000000"
			}`),
		},
		ABI: fakeCoinABI,
	}, true)
	assert.NoError(t, rpcErr)

	rpcErr = tbRPC.CallRPC(ctx, pldtypes.RawJSON{}, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "wallets.org1.aaaaaa",
			To:       &contractAddr,
			Function: "transfer",
			Data: pldtypes.RawJSON(`{
				"from": "wallets.org1.aaaaaa",
				"to": "wallets.org2.bbbbbb",
				"amount": "23000000000000000000"
			}`),
		},
		ABI: fakeCoinABI,
	}, true)
	assert.NoError(t, rpcErr)

	var balance *getBalanceResult
	rpcErr = tbRPC.CallRPC(ctx, &balance, "testbed_call", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:       &contractAddr,
			Function: "getBalance",
			Data: pldtypes.RawJSON(`{
				"account": "wallets.org1.aaaaaa"
			}`),
		},
		ABI: fakeCoinABI,
	}, pldtypes.DefaultJSONFormatOptions)
	assert.NoError(t, rpcErr)
	assert.Equal(t, "100000000000000000000", balance.Amount.Int().String())

	// Check we can also use the utility function externally to resolve verifiers
	var address pldtypes.EthAddress
	rpcErr = tbRPC.CallRPC(ctx, &address, "testbed_resolveVerifier", "wallets.org2.bbbbbb", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.NoError(t, rpcErr)
	assert.False(t, address.IsZero())

}

// We have create a testbed with no domains from our config, to be able to deploy the factory.
// Then we return the factory
func deploySmartContract(t *testing.T, confFile string) *pldtypes.EthAddress {
	ctx := context.Background()
	tb := NewTestBed()
	_, _, done, err := tb.StartForTest(confFile, nil)
	require.NoError(t, err)
	defer done()

	simDomainABI := mustParseBuildABI(simDomainBuild)
	simDomainBytecode := mustParseBuildBytecode(simDomainBuild)
	txm := tb.Components().TxManager()

	// In this test we deploy the factory in-line
	var txIDs []uuid.UUID
	err = tb.Components().Persistence().Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		txIDs, err = tb.Components().TxManager().SendTransactions(ctx, dbTX, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type: pldapi.TransactionTypePublic.Enum(),
				From: "domain1_admin",
			},
			ABI:      simDomainABI,
			Bytecode: simDomainBytecode,
		})
		return err
	})
	require.NoError(t, err)
	txID := txIDs[0]

	var receipt *pldapi.TransactionReceipt
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		<-ticker.C
		require.False(t, t.Failed())
		receipt, err = txm.GetTransactionReceiptByID(ctx, txID)
		require.NoError(t, err)
		if receipt != nil {
			break
		}
	}

	require.True(t, receipt.Success)
	require.NotNil(t, receipt.ContractAddress)
	return receipt.ContractAddress
}
