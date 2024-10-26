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
package domains

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	SimpleStorageDomainStateNotAvailableError = "SDE0101"
	SimpleStorageDomainTooManyStates          = "SDE0102"
)

// Note, here we're simulating a domain that choose to support versions of a "Transfer" function
// with "string" types (rather than "address") for the from/to address and to ask Paladin to do
// verifier resolution for these. The same domain could also support "address" type inputs/outputs
// in the same ABI.
const simpleStorageSetABI = `{
		"type": "function",
		"name": "set",
		"inputs": [
		  {
		    "name": "key",
			"type": "string"
		  },
		  {
		    "name": "value",
			"type": "string"
		  }
		],
		"outputs": null
	}`

func SimpleStorageSetABI() *abi.ABI {
	return &abi.ABI{mustParseABIEntry(simpleStorageSetABI)}
}

// ABI used by paladin to parse the constructor parameters
// different for each endorsement mode
const simpleStorageConstructorABI = `{
  "type": "constructor",
  "inputs": [
  	{
	  "name": "endorsementSet",
	  "type": "string[]"
	},
    {
      "name": "from",
      "type": "string"
    },
    {
      "name": "name",
      "type": "string"
    },
    {
      "name": "endorsementMode",
      "type": "string"
    }
  ],
  "outputs": null
}`

func SimpleStorageConstructorABI(endorsementMode string) *abi.ABI {
	return &abi.ABI{mustParseABIEntry(simpleStorageConstructorABI)}
}

// Go struct used in test (test + domain) to work with JSON structure passed into the paladin transaction for the constructor
// This is a union of the 3 ABI above
type SimpleStorageConstructorParameters struct {
	EndorsementSet  []string `json:"endorsementSet"`
	From            string   `json:"from"`
	Name            string   `json:"name"`
	EndorsementMode string   `json:"endorsementMode"`
}

// Go struct used in test (test + domain) to work with JSON structure for `params` on the base ledger factory function
// This must match (including ordering of fields) the function signature for newSimpleTokenNotarized defined in the solidity contract
type SimpleStorageFactoryParameters struct {
	TxId                   string   `json:"txId"`
	EndorsementMode        string   `json:"endorsementMode"`
	NotaryLocator          string   `json:"notaryLocator"`
	EndorsementSetLocators []string `json:"endorsementSetLocators"`
}

// JSON structure passed into the paladin transaction for the storage set function
type simpleStorageSetParser struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// JSON structure for the state data
type StorageState struct {
	Salt    tktypes.HexBytes `json:"salt"`
	Records string           `json:"records"` //JSON string that can be parsed as a map of keys to values of StorageRecord
}

const simpleStorageStateSchema = `{
	"type": "tuple",
	"internalType": "struct SimpleStorage",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "records",
			"type": "string"
		}
	]
}`

// contract instance config( i.e. data field of event PaladinRegisterSmartContract_V0)
type simpleStorageConfigParser simpleTokenConfigParser // for now, we are re-sing the .sol from simpleDomain.

// domain config
type SimpleStorageDomainConfig struct {
	SubmitMode      string   `json:"submitMode"`
	EndorsementMode string   `json:"endorsementMode"`
	EndorsementSet  []string `json:"endorsementSet"`
}

func SimpleStorageDomain(t *testing.T, ctx context.Context) plugintk.PluginBase {
	simpleDomainABI := mustParseBuildABI(simpleDomainBuild)
	simpleTokenABI := mustParseBuildABI(simpleTokenBuild)

	transferABI := simpleTokenABI.Events()["UTXOTransfer"]
	require.NotEmpty(t, transferABI)
	transferSignature := transferABI.SolString()

	return plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {

		var simpleStorageSchemaID string
		simpleStorageSelection := func(ctx context.Context, stateQueryContext string) (*StorageState, *prototk.StateRef, error) {
			var lastStateTimestamp int64
			// There is only one state per domain instance
			// (might be more realistic emulation of the Pente domain if we had one state per record / key name?)
			jq := &query.QueryJSON{
				Limit:      confutil.P(10),
				Sort:       []string{".created"},
				Statements: query.Statements{},
			}
			if lastStateTimestamp > 0 {
				jq.GT = []*query.OpSingleVal{
					{Op: query.Op{Field: ".created"}, Value: tktypes.RawJSON(strconv.FormatInt(lastStateTimestamp, 10))},
				}
			}
			res, err := callbacks.FindAvailableStates(ctx, &prototk.FindAvailableStatesRequest{
				StateQueryContext: stateQueryContext,
				SchemaId:          simpleStorageSchemaID,
				QueryJson:         tktypes.JSONString(jq).String(),
			})
			if err != nil {
				return nil, nil, err
			}
			states := res.States
			if len(states) == 0 {
				//assume this is the first transaction
				return nil, nil, nil

				//return nil, nil, fmt.Errorf("%s: state not available", SimpleStorageDomainStateNotAvailableError)
			}
			if len(states) > 1 {
				return nil, nil, fmt.Errorf("%s: state not available", SimpleStorageDomainTooManyStates)
			}
			state := states[0]
			stateData := new(StorageState)
			if err := json.Unmarshal([]byte(state.DataJson), stateData); err != nil {
				return nil, nil, fmt.Errorf("stateData %s is invalid: %s", state.Id, err)
			}
			return stateData,
				&prototk.StateRef{
					Id:       state.Id,
					SchemaId: state.SchemaId,
				}, nil
		}

		validateSimpleStorageSetInput := func(tx *prototk.TransactionSpecification) (*ethtypes.Address0xHex, simpleStorageConfigParser, *simpleStorageSetParser) {
			//assert.JSONEq(t, simpleTokenTransferABI, tx.FunctionAbiJson)
			//assert.Equal(t, "function transfer(string memory from, string memory to, uint256 amount) external { }", tx.FunctionSignature)
			var inputs simpleStorageSetParser
			err := json.Unmarshal([]byte(tx.FunctionParamsJson), &inputs)
			require.NoError(t, err)

			contractAddr, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
			require.NoError(t, err)

			require.NoError(t, err)
			var config simpleStorageConfigParser
			err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &config)
			require.NoError(t, err)

			return contractAddr, config, &inputs
		}

		return &plugintk.DomainAPIBase{Functions: &plugintk.DomainAPIFunctions{

			ConfigureDomain: func(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
				assert.Equal(t, "simpleStorageDomain", req.Name)
				domainConfig := &SimpleStorageDomainConfig{}
				err := json.Unmarshal([]byte(req.ConfigJson), domainConfig)
				require.NoError(t, err)

				var eventsABI abi.ABI
				eventsABI = append(eventsABI, transferABI)
				eventsJSON, err := json.Marshal(eventsABI)
				require.NoError(t, err)

				return &prototk.ConfigureDomainResponse{
					DomainConfig: &prototk.DomainConfig{
						AbiStateSchemasJson: []string{simpleStorageStateSchema},
						AbiEventsJson:       string(eventsJSON),
					},
				}, nil
			},

			InitDomain: func(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
				assert.Len(t, req.AbiStateSchemas, 1)
				simpleStorageSchemaID = req.AbiStateSchemas[0].Id
				assert.Equal(t, "type=SimpleStorage(bytes32 salt,string records),labels=[]", req.AbiStateSchemas[0].Signature)
				return &prototk.InitDomainResponse{}, nil
			},

			InitDeploy: func(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
				constructorParameters := &SimpleStorageConstructorParameters{}
				err := json.Unmarshal([]byte(req.Transaction.ConstructorParamsJson), constructorParameters)
				require.NoError(t, err)

				switch constructorParameters.EndorsementMode {
				case PrivacyGroupEndorsement:
					requiredVerifiers := make([]*prototk.ResolveVerifierRequest, len(constructorParameters.EndorsementSet))
					for i, v := range constructorParameters.EndorsementSet {
						requiredVerifiers[i] = &prototk.ResolveVerifierRequest{
							Lookup:       v,
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
						}
					}
					return &prototk.InitDeployResponse{
						RequiredVerifiers: requiredVerifiers,
					}, nil
				}
				return nil, fmt.Errorf("unknown endorsement mode %s", constructorParameters.EndorsementMode)
			},

			PrepareDeploy: func(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
				/*assert.JSONEq(t, `{
					"notary": "domain1.contract1.notary",
					"name": "FakeToken1",
					"symbol": "FT1",
					"endorsementMode": "NoEndorsement"
				}`, req.Transaction.ConstructorParamsJson)*/
				constructorParameters := &SimpleStorageConstructorParameters{}
				err := json.Unmarshal([]byte(req.Transaction.ConstructorParamsJson), constructorParameters)
				require.NoError(t, err)

				switch constructorParameters.EndorsementMode {
				case PrivacyGroupEndorsement:
					assert.Len(t, req.ResolvedVerifiers, len(constructorParameters.EndorsementSet))
					// We don't know that the order of the ResolvedVerifiers will match the order of the endorsement set,
					// so we just check that they are all there
					for _, v := range constructorParameters.EndorsementSet {
						found := false
						for j := range req.ResolvedVerifiers {
							if req.ResolvedVerifiers[j].Lookup == v {
								assert.Equal(t, algorithms.ECDSA_SECP256K1, req.ResolvedVerifiers[j].Algorithm)
								assert.Equal(t, verifiers.ETH_ADDRESS, req.ResolvedVerifiers[j].VerifierType)
								assert.Equal(t, v, req.ResolvedVerifiers[j].Lookup)
								assert.NotEmpty(t, req.ResolvedVerifiers[j].Verifier)
								found = true
							}
						}
						assert.True(t, found, "endorser %s not found in ResolvedVerifiers", v)
					}
				}
				if constructorParameters.EndorsementSet == nil {
					constructorParameters.EndorsementSet = []string{}
				}
				params := FactoryParameters{
					TxId:                   req.Transaction.TransactionId,
					EndorsementSetLocators: constructorParameters.EndorsementSet,
					EndorsementMode:        constructorParameters.EndorsementMode,
				}
				return &prototk.PrepareDeployResponse{
					Signer: confutil.P(fmt.Sprintf("domain1.transactions.%s", req.Transaction.TransactionId)),
					Transaction: &prototk.PreparedTransaction{
						FunctionAbiJson: toJSONString(t, simpleDomainABI.Functions()["newSimpleTokenNotarized"]),
						ParamsJson:      tktypes.JSONString(params).String(),
					},
				}, nil
			},

			InitContract: func(ctx context.Context, icr *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {

				configValues, err := contractDataABI.DecodeABIData(icr.ContractConfig, 0)
				str := tktypes.HexBytes(icr.ContractConfig).HexString0xPrefix()
				assert.NotEqual(t, "", str)
				require.NoError(t, err)

				configJSON, err := tktypes.StandardABISerializer().SerializeJSON(configValues)
				require.NoError(t, err)
				contractConfig := &prototk.ContractConfig{
					ContractConfigJson: string(configJSON),
				}
				var constructorParameters SimpleStorageConstructorParameters
				err = json.Unmarshal([]byte(configJSON), &constructorParameters)
				require.NoError(t, err)

				if constructorParameters.EndorsementMode == SelfEndorsement {
					contractConfig.CoordinatorSelection = prototk.ContractConfig_COORDINATOR_SENDER
					contractConfig.SubmitterSelection = prototk.ContractConfig_SUBMITTER_SENDER
				} else if constructorParameters.EndorsementMode == PrivacyGroupEndorsement {
					contractConfig.CoordinatorSelection = prototk.ContractConfig_COORDINATOR_ENDORSER
					contractConfig.SubmitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
				} else {
					return nil, fmt.Errorf("unknown endorsement mode %s", constructorParameters.EndorsementMode)
				}

				return &prototk.InitContractResponse{
					Valid:          true,
					ContractConfig: contractConfig,
				}, nil
			},

			InitTransaction: func(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
				_, config, txInputs := validateSimpleStorageSetInput(req.Transaction)

				requiredVerifiers := []*prototk.ResolveVerifierRequest{
					{
						Lookup:       req.Transaction.From,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					},
				}
				// We require ethereum addresses for the "from" and "to" addresses to actually
				// execute the transaction. See notes above about this.
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

				switch config.EndorsementMode {

				case PrivacyGroupEndorsement:
					for _, v := range config.EndorsementSet {
						requiredVerifiers = append(requiredVerifiers, &prototk.ResolveVerifierRequest{
							Lookup:       v,
							Algorithm:    algorithms.ECDSA_SECP256K1,
							VerifierType: verifiers.ETH_ADDRESS,
						})
					}
				default:
					return nil, fmt.Errorf("unknown endorsement mode %s", config.EndorsementMode)
				}

				return &prototk.InitTransactionResponse{
					RequiredVerifiers: requiredVerifiers,
				}, nil
			},

			AssembleTransaction: func(ctx context.Context, req *prototk.AssembleTransactionRequest) (_ *prototk.AssembleTransactionResponse, err error) {
				_, config, txInputs := validateSimpleStorageSetInput(req.Transaction)

				//_, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)

				stateToSpend, stateRefToSpend, err := simpleStorageSelection(ctx, req.StateQueryContext)
				if err != nil {
					return nil, err
				}

				stateRefsToSpend := make([]*prototk.StateRef, 0)
				storage := make(map[string]string)
				if stateToSpend != nil {
					stateRefsToSpend = append(stateRefsToSpend, stateRefToSpend)
					if err := json.Unmarshal([]byte(stateToSpend.Records), &storage); err != nil {
						return nil, fmt.Errorf("invalid state data: %s", err)
					}
				}

				storage[txInputs.Key] = txInputs.Value

				newStateData := &StorageState{
					Records: toJSONString(t, storage),
					Salt:    tktypes.RandBytes(32),
				}
				newState := &prototk.NewState{
					SchemaId:         simpleStorageSchemaID,
					StateDataJson:    toJSONString(t, newStateData),
					DistributionList: config.EndorsementSet,
				}

				//eip712Payload, err := typedDataV4TransferWithSalts(contractAddr, coinsToSpend, newCoins)
				//require.NoError(t, err)
				eip712Payload := tktypes.RandBytes(32)

				switch config.EndorsementMode {

				case PrivacyGroupEndorsement:

					return &prototk.AssembleTransactionResponse{
						AssembledTransaction: &prototk.AssembledTransaction{
							InputStates:  stateRefsToSpend,
							OutputStates: []*prototk.NewState{newState},
						},
						AssemblyResult: prototk.AssembleTransactionResponse_OK,
						AttestationPlan: []*prototk.AttestationRequest{
							{
								Name:            "sender",
								AttestationType: prototk.AttestationType_SIGN,
								Algorithm:       algorithms.ECDSA_SECP256K1,
								VerifierType:    verifiers.ETH_ADDRESS,
								PayloadType:     signpayloads.OPAQUE_TO_RSV,
								Payload:         eip712Payload,
								Parties: []string{
									req.Transaction.From,
								},
							},
							{
								Name:            "privacyGroup",
								AttestationType: prototk.AttestationType_ENDORSE,
								// we expect an endorsement is of the form ENDORSER_SUBMIT - so we need an eth signing key to exist
								Algorithm:    algorithms.ECDSA_SECP256K1,
								VerifierType: verifiers.ETH_ADDRESS,
								PayloadType:  signpayloads.OPAQUE_TO_RSV,
								Parties:      config.EndorsementSet,
							},
						},
					}, nil
				default:
					return nil, fmt.Errorf("unsupported endorsement mode: %s", config.EndorsementMode)
				}
			},

			EndorseTransaction: func(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
				_, config, _ := validateSimpleStorageSetInput(req.Transaction)
				//notaryLocator := config.NotaryLocator
				//senderAddr, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)
				//assert.Equal(t, req.EndorsementVerifier.Lookup, req.EndorsementRequest.Parties[0])
				//assert.Equal(t, req.EndorsementVerifier.Lookup, notaryLocator)
				if len(req.Inputs) > 1 {
					//We allow 0 in the special case of the first transaction
					//TODO is that a faithful emulation of the pente domain?
					return nil, fmt.Errorf("invalid number of inputs [%d]", len(req.Inputs))
				}
				if len(req.Outputs) != 1 {
					return nil, fmt.Errorf("invalid number of outputs [%d]", len(req.Outputs))
				}

				if len(req.Inputs) == 1 {
					inputState := &StorageState{}
					if err := json.Unmarshal([]byte(req.Inputs[0].StateDataJson), &inputState); err != nil {
						return nil, fmt.Errorf("invalid input (%s): %s", req.Inputs[0].Id, err)
					}
					assert.Equal(t, simpleStorageSchemaID, req.Inputs[0].SchemaId)

				}

				outputState := &StorageState{}
				//TODO should validate that the diffs between inputState and outputState match the txInputs

				if err := json.Unmarshal([]byte(req.Outputs[0].StateDataJson), &outputState); err != nil {
					return nil, fmt.Errorf("invalid output (%s): %s", req.Outputs[0].Id, err)
				}
				assert.Equal(t, simpleStorageSchemaID, req.Outputs[0].SchemaId)

				switch config.EndorsementMode {
				case PrivacyGroupEndorsement:
					return &prototk.EndorseTransactionResponse{
						EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
						Payload:           tktypes.RandBytes(32),
					}, nil
				default:
					return nil, fmt.Errorf("unsupported endorsement mode: %s", config.EndorsementMode)

				}

			},

			PrepareTransaction: func(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
				var signerSignature tktypes.HexBytes
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
						FunctionAbiJson: toJSONString(t, simpleTokenABI.Functions()["executeNotarized"]),
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
		}}
	})
}
