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
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/SimpleDomain.json
var simpleDomainBuild []byte // comes from Hardhat build

//go:embed abis/SimpleToken.json
var simpleTokenBuild []byte // comes from Hardhat build

const (
	SimpleDomainInsufficientFundsError = "SDE0001"
)

const (
	ONE_TIME_USE_KEYS   = "ONE_TIME_USE_KEYS"
	ENDORSER_SUBMISSION = "ENDORSER_SUBMISSION"
)

const (
	// SelfEndorsement is kinda like zeto
	//  There is a single endorser which is the same as the sender.
	//Unlike zeto, this does *not* imply a domain provided signer algo.
	// TODO maybe add signerMode flexibiltiy to the simple domain at some point
	SelfEndorsement = "SelfEndorsement"

	//NotaryEndorsement is kinda like noto.
	// There is a single endorser which is a notary and endorser must submit.
	NotaryEndorsement = "NotaryEndorsement"

	//PrivacyGroupEndorsement is kinda like pente.
	//But pente is not a token based domain so we tend to use simpleStorageDomain to emulate pente like behavior
	//An endorsement set is provided in the constructor and every member of that group must endorse every transaction.
	PrivacyGroupEndorsement = "PrivacyGroupEndorsement"
)

func toJSONString(t *testing.T, v interface{}) string {
	b, err := json.Marshal(v)
	assert.NoError(t, err)
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

func mustParseBuildABI(buildJSON []byte) abi.ABI {
	var buildParsed map[string]pldtypes.RawJSON
	var buildABI abi.ABI
	err := json.Unmarshal(buildJSON, &buildParsed)
	if err == nil {
		err = json.Unmarshal(buildParsed["abi"], &buildABI)
	}
	if err != nil {
		panic(err)
	}
	return buildABI
}

func mustParseBuildBytecode(buildJSON []byte) pldtypes.HexBytes {
	var buildParsed map[string]pldtypes.RawJSON
	var byteCode pldtypes.HexBytes
	err := json.Unmarshal(buildJSON, &buildParsed)
	if err == nil {
		err = json.Unmarshal(buildParsed["bytecode"], &byteCode)
	}
	if err != nil {
		panic(err)
	}
	return byteCode
}

func DeploySmartContract(t *testing.T, p persistence.Persistence, txm components.TXManager, km components.KeyManager) *pldtypes.EthAddress {
	ctx := context.Background()

	simpleDomainABI := mustParseBuildABI(simpleDomainBuild)
	simpleDomainBytecode := mustParseBuildBytecode(simpleDomainBuild)

	// In this test we deploy the factory in-line
	var txIDs []uuid.UUID
	err := p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		txIDs, err = txm.SendTransactions(ctx, dbTX, &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type: pldapi.TransactionTypePublic.Enum(),
				From: "domain1_admin",
			},
			ABI:      simpleDomainABI,
			Bytecode: simpleDomainBytecode,
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

// Note, here we're simulating a domain that choose to support versions of a "Transfer" function
// with "string" types (rather than "address") for the from/to address and to ask Paladin to do
// verifier resolution for these. The same domain could also support "address" type inputs/outputs
// in the same ABI.
const simpleTokenTransferABI = `{
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

func SimpleTokenTransferABI() *abi.ABI {
	return &abi.ABI{mustParseABIEntry(simpleTokenTransferABI)}
}

// ABI used by paladin to parse the constructor parameters
// different for each endorsement mode
const simpleTokenSelfEndorsementConstructorABI = `{
  "type": "constructor",
  "inputs": [
    {
      "name": "from",
      "type": "string"
    },
    {
      "name": "name",
      "type": "string"
    },
    {
      "name": "symbol",
      "type": "string"
    },
    {
      "name": "endorsementMode",
      "type": "string"
    }
  ],
  "outputs": null
}`

const simpleTokenNotaryEndorsementConstructorABI = `{
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
	  },
	  {
		"name": "endorsementMode",
		"type": "string"
	  }
	],
	"outputs": null
}`

const simpleTokenPrivacyGroupEndorsementConstructorABI = `{
	"type": "constructor",
	"inputs": [
	  {
		"name": "endorsementSet",
		"type": "string[]"
	  },
	  {
		"name": "name",
		"type": "string"
	  },
	  {
		"name": "symbol",
		"type": "string"
	  },
	  {
		"name": "endorsementMode",
		"type": "string"
	  }
	],
	"outputs": null
}`

func SimpleTokenConstructorABI(endorsementMode string) *abi.ABI {
	switch endorsementMode {
	case SelfEndorsement:

		return &abi.ABI{mustParseABIEntry(simpleTokenSelfEndorsementConstructorABI)}
	case NotaryEndorsement:
		return &abi.ABI{mustParseABIEntry(simpleTokenNotaryEndorsementConstructorABI)}
	case PrivacyGroupEndorsement:
		return &abi.ABI{mustParseABIEntry(simpleTokenPrivacyGroupEndorsementConstructorABI)}
	default:
		panic("unknown endorsement mode")
	}

}

// Go struct used in test (test + domain) to work with JSON structure passed into the paladin transaction for the constructor
// This is a union of the 3 ABI above
type ConstructorParameters struct {
	Notary          string   `json:"notary"`         // empty string if  endorsementMode is PrivacyGroupEndorsement
	EndorsementSet  []string `json:"endorsementSet"` // empty array if endorsementMode is not PrivacyGroupEndorsement
	From            string   `json:"from"`           // empty string if endorsementMode is not SelfEndorsement
	Name            string   `json:"name"`
	Symbol          string   `json:"symbol"`
	EndorsementMode string   `json:"endorsementMode"`
}

// Go struct used in test (test + domain) to work with JSON structure for `params` on the base ledger factory function
// This must match (including ordering of fields) the function signature for newSimpleTokenNotarized defined in the solidity contract
type FactoryParameters struct {
	TxId                   string   `json:"txId"`
	EndorsementMode        string   `json:"endorsementMode"`
	NotaryLocator          string   `json:"notaryLocator"`
	EndorsementSetLocators []string `json:"endorsementSetLocators"`
}

// JSON structure passed into the paladin transaction for the transfer
type fakeTransferParser struct {
	From   string               `json:"from,omitempty"`
	To     string               `json:"to,omitempty"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

// JSON structure for the state data
type simpleTokenParser struct {
	Salt   pldtypes.HexBytes     `json:"salt"`
	Owner  ethtypes.Address0xHex `json:"owner"`
	Amount *ethtypes.HexInteger  `json:"amount"`
}

type SimpleDomainConfig struct {
	SubmitMode string `json:"submitMode"`
}

// ABI for the config field in the PaladinRegisterSmartContract_V0 event
// this must match the type and order of arguments passed to the abi.encode function call in the solidity contract
var contractDataABI = &abi.ParameterArray{
	{Name: "endorsementMode", Type: "string"},
	{Name: "notaryLocator", Type: "string"},
	{Name: "endorsementSetLocators", Type: "string[]"},
}

// golang struct to parse and serialize the data received from the block indexer when the base ledger factor contract
// emits a PaladinRegisterSmartContract_V0 event
// this must match the ABI above
type simpleTokenConfigParser struct {
	EndorsementMode string   `json:"endorsementMode"`
	NotaryLocator   string   `json:"notaryLocator"`
	EndorsementSet  []string `json:"endorsementSetLocators"`
}

func SimpleTokenDomain(t *testing.T, ctx context.Context) plugintk.PluginBase {
	simpleDomainABI := mustParseBuildABI(simpleDomainBuild)
	simpleTokenABI := mustParseBuildABI(simpleTokenBuild)

	transferABI := simpleTokenABI.Events()["UTXOTransfer"]
	require.NotEmpty(t, transferABI)
	transferSignature := transferABI.SolString()

	simpleTokenStateSchema := `{
		"type": "tuple",
		"internalType": "struct SimpleToken",
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

	return plugintk.NewDomain(func(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {

		var simpleTokenSchemaID string
		var chainID int64
		simpleTokenSelection := func(ctx context.Context, stateQueryContext string, fromAddr *ethtypes.Address0xHex, amount *big.Int) ([]*simpleTokenParser, []*prototk.StateRef, *big.Int, string, error) {
			var lastStateTimestamp int64
			total := big.NewInt(0)
			coins := []*simpleTokenParser{}
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
					SchemaId:          simpleTokenSchemaID,
					QueryJson:         pldtypes.JSONString(jq).String(),
				})
				if err != nil {
					return nil, nil, nil, "", err
				}
				states := res.States
				if len(states) == 0 {
					return nil, nil, nil, fmt.Sprintf("%s: insufficient funds (available=%s)", SimpleDomainInsufficientFundsError, total.Text(10)), nil
				}
				for _, state := range states {
					lastStateTimestamp = state.CreatedAt
					// Note: More sophisticated coin selection might prefer states that aren't locked to a sequence
					var coin simpleTokenParser
					if err := json.Unmarshal([]byte(state.DataJson), &coin); err != nil {
						return nil, nil, nil, "", fmt.Errorf("coin %s is invalid: %s", state.Id, err)
					}
					total = total.Add(total, coin.Amount.BigInt())
					stateRefs = append(stateRefs, &prototk.StateRef{
						Id:       state.Id,
						SchemaId: state.SchemaId,
					})
					coins = append(coins, &coin)
					if total.Cmp(amount) >= 0 {
						// We've got what we need - return how much over we are
						return coins, stateRefs, new(big.Int).Sub(total, amount), "", nil
					}
				}
			}
		}

		validateTransferTransactionInput := func(tx *prototk.TransactionSpecification) (*ethtypes.Address0xHex, simpleTokenConfigParser, *fakeTransferParser) {
			assert.JSONEq(t, simpleTokenTransferABI, tx.FunctionAbiJson)
			assert.Equal(t, "function transfer(string memory from, string memory to, uint256 amount) external { }", tx.FunctionSignature)
			var inputs fakeTransferParser
			err := json.Unmarshal([]byte(tx.FunctionParamsJson), &inputs)
			require.NoError(t, err)
			assert.Greater(t, inputs.Amount.BigInt().Sign(), 0)
			contractAddr, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
			require.NoError(t, err)
			var config simpleTokenConfigParser
			err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &config)
			require.NoError(t, err)
			//assert.NotEmpty(t, config.NotaryLocator)

			return contractAddr, config, &inputs
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

		typedDataV4TransferWithSalts := func(contract *ethtypes.Address0xHex, inputs, outputs []*simpleTokenParser) (pldtypes.HexBytes, error) {
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
				domainConfig := &SimpleDomainConfig{}
				err := json.Unmarshal([]byte(req.ConfigJson), domainConfig)
				require.NoError(t, err)

				assert.Equal(t, int64(1337), req.ChainId) // from tools/besu_bootstrap
				chainID = req.ChainId

				var eventsABI abi.ABI
				eventsABI = append(eventsABI, transferABI)
				eventsJSON, err := json.Marshal(eventsABI)
				require.NoError(t, err)

				return &prototk.ConfigureDomainResponse{
					DomainConfig: &prototk.DomainConfig{
						AbiStateSchemasJson: []string{simpleTokenStateSchema},
						AbiEventsJson:       string(eventsJSON),
					},
				}, nil
			},

			InitDomain: func(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
				assert.Len(t, req.AbiStateSchemas, 1)
				simpleTokenSchemaID = req.AbiStateSchemas[0].Id
				assert.Equal(t, "type=SimpleToken(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", req.AbiStateSchemas[0].Signature)
				return &prototk.InitDomainResponse{}, nil
			},

			InitDeploy: func(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
				constructorParameters := &ConstructorParameters{}
				err := json.Unmarshal([]byte(req.Transaction.ConstructorParamsJson), constructorParameters)
				require.NoError(t, err)

				switch constructorParameters.EndorsementMode {
				case SelfEndorsement:
					return &prototk.InitDeployResponse{
						RequiredVerifiers: []*prototk.ResolveVerifierRequest{
							{
								Lookup:       constructorParameters.From,
								Algorithm:    algorithms.ECDSA_SECP256K1,
								VerifierType: verifiers.ETH_ADDRESS,
							},
						},
					}, nil

				case NotaryEndorsement:
					return &prototk.InitDeployResponse{
						RequiredVerifiers: []*prototk.ResolveVerifierRequest{
							{
								Lookup:       constructorParameters.Notary,
								Algorithm:    algorithms.ECDSA_SECP256K1,
								VerifierType: verifiers.ETH_ADDRESS,
							},
						},
					}, nil
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
				constructorParameters := &ConstructorParameters{}
				err := json.Unmarshal([]byte(req.Transaction.ConstructorParamsJson), constructorParameters)
				require.NoError(t, err)

				switch constructorParameters.EndorsementMode {
				case SelfEndorsement:

					assert.Len(t, req.ResolvedVerifiers, 1)
					assert.Equal(t, algorithms.ECDSA_SECP256K1, req.ResolvedVerifiers[0].Algorithm)
					assert.Equal(t, verifiers.ETH_ADDRESS, req.ResolvedVerifiers[0].VerifierType)
					assert.Equal(t, constructorParameters.From, req.ResolvedVerifiers[0].Lookup)
					require.NotEmpty(t, req.ResolvedVerifiers[0].Verifier)
				case NotaryEndorsement:
					assert.Len(t, req.ResolvedVerifiers, 1)
					assert.Equal(t, algorithms.ECDSA_SECP256K1, req.ResolvedVerifiers[0].Algorithm)
					assert.Equal(t, verifiers.ETH_ADDRESS, req.ResolvedVerifiers[0].VerifierType)
					assert.Equal(t, constructorParameters.Notary, req.ResolvedVerifiers[0].Lookup)
					require.NotEmpty(t, req.ResolvedVerifiers[0].Verifier)
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
					NotaryLocator:          constructorParameters.Notary,
				}
				return &prototk.PrepareDeployResponse{
					Signer: confutil.P(fmt.Sprintf("domain1.transactions.%s", req.Transaction.TransactionId)),
					Transaction: &prototk.PreparedTransaction{
						FunctionAbiJson: toJSONString(t, simpleDomainABI.Functions()["newSimpleTokenNotarized"]),
						ParamsJson:      pldtypes.JSONString(params).String(),
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
					ContractConfigJson: string(configJSON),
				}
				var constructorParameters simpleTokenConfigParser
				err = json.Unmarshal([]byte(configJSON), &constructorParameters)
				require.NoError(t, err)

				switch constructorParameters.EndorsementMode {
				case SelfEndorsement:
					contractConfig.CoordinatorSelection = prototk.ContractConfig_COORDINATOR_SENDER
					contractConfig.SubmitterSelection = prototk.ContractConfig_SUBMITTER_SENDER
				case NotaryEndorsement:
					contractConfig.CoordinatorSelection = prototk.ContractConfig_COORDINATOR_STATIC
					contractConfig.StaticCoordinator = &constructorParameters.NotaryLocator
					contractConfig.SubmitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
				case PrivacyGroupEndorsement:
					//This combination is less common on a token based domain but may use it in some tests
					contractConfig.CoordinatorSelection = prototk.ContractConfig_COORDINATOR_ENDORSER
					contractConfig.SubmitterSelection = prototk.ContractConfig_SUBMITTER_COORDINATOR
				default:
					return nil, fmt.Errorf("unknown endorsement mode %s", constructorParameters.EndorsementMode)
				}

				return &prototk.InitContractResponse{
					Valid:          true,
					ContractConfig: contractConfig,
				}, nil
			},

			InitTransaction: func(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
				_, config, txInputs := validateTransferTransactionInput(req.Transaction)

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
				case SelfEndorsement:

					//Only need the from and to addresses which have already been added above

				case NotaryEndorsement:
					requiredVerifiers = append(requiredVerifiers, &prototk.ResolveVerifierRequest{
						Lookup:       config.NotaryLocator,
						Algorithm:    algorithms.ECDSA_SECP256K1,
						VerifierType: verifiers.ETH_ADDRESS,
					})
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
				contractAddr, config, txInputs := validateTransferTransactionInput(req.Transaction)

				_, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)
				amount := txInputs.Amount.BigInt()
				toKeep := new(big.Int)
				coinsToSpend := []*simpleTokenParser{}
				stateRefsToSpend := []*prototk.StateRef{}
				revertMessage := ""
				if txInputs.From != "" {
					coinsToSpend, stateRefsToSpend, toKeep, revertMessage, err = simpleTokenSelection(ctx, req.StateQueryContext, fromAddr, amount)
					if err != nil {
						return nil, err
					}
					for _, state := range stateRefsToSpend {
						log.L(ctx).Infof("Spend coin %s", state.Id)
					}
				}
				if revertMessage != "" {
					return &prototk.AssembleTransactionResponse{
						AssembledTransaction: &prototk.AssembledTransaction{},
						AssemblyResult:       prototk.AssembleTransactionResponse_REVERT,
						RevertReason:         &revertMessage,
					}, nil
				}

				newStates := []*prototk.NewState{}
				newCoins := []*simpleTokenParser{}
				if fromAddr != nil && toKeep.Sign() > 0 {
					// Generate a state to keep for ourselves
					coin := simpleTokenParser{
						Salt:   pldtypes.RandBytes(32),
						Owner:  *fromAddr,
						Amount: (*ethtypes.HexInteger)(toKeep),
					}
					newCoins = append(newCoins, &coin)

					distroList := []string{req.Transaction.From}
					if config.NotaryLocator != "" {
						distroList = append(distroList, config.NotaryLocator)
					}
					newStates = append(newStates, &prototk.NewState{
						SchemaId:         simpleTokenSchemaID,
						StateDataJson:    toJSONString(t, &coin),
						DistributionList: distroList,
					})
				}
				if toAddr != nil && amount.Sign() > 0 {
					// Generate the coin to transfer
					coin := simpleTokenParser{
						Salt:   pldtypes.RandBytes(32),
						Owner:  *toAddr,
						Amount: (*ethtypes.HexInteger)(amount),
					}
					newCoins = append(newCoins, &coin)
					distroList := []string{req.Transaction.From, txInputs.To}
					if config.NotaryLocator != "" {
						distroList = append(distroList, config.NotaryLocator)
					}
					newStates = append(newStates, &prototk.NewState{
						SchemaId:         simpleTokenSchemaID,
						StateDataJson:    toJSONString(t, &coin),
						DistributionList: distroList,
					})
				}
				eip712Payload, err := typedDataV4TransferWithSalts(contractAddr, coinsToSpend, newCoins)
				require.NoError(t, err)

				switch config.EndorsementMode {
				case SelfEndorsement:
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
								PayloadType:     signpayloads.OPAQUE_TO_RSV,
								Payload:         eip712Payload,
								Parties: []string{
									req.Transaction.From,
								},
							},
							{
								Name:            "submitter",
								AttestationType: prototk.AttestationType_ENDORSE,
								Algorithm:       algorithms.ECDSA_SECP256K1,
								VerifierType:    verifiers.ETH_ADDRESS,
								PayloadType:     signpayloads.OPAQUE_TO_RSV,
								Payload:         eip712Payload,
								Parties: []string{
									req.Transaction.From,
								},
							},
						},
					}, nil
				case NotaryEndorsement:
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
								PayloadType:     signpayloads.OPAQUE_TO_RSV,
								Payload:         eip712Payload,
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
								PayloadType:  signpayloads.OPAQUE_TO_RSV,
								Payload:      eip712Payload,
								Parties: []string{
									config.NotaryLocator,
								},
							},
						},
					}, nil
				case PrivacyGroupEndorsement:

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
				contractAddr, config, txInputs := validateTransferTransactionInput(req.Transaction)
				//notaryLocator := config.NotaryLocator
				senderAddr, fromAddr, toAddr := extractTransferVerifiers(req.Transaction, txInputs, req.ResolvedVerifiers)
				//assert.Equal(t, req.EndorsementVerifier.Lookup, req.EndorsementRequest.Parties[0])
				//assert.Equal(t, req.EndorsementVerifier.Lookup, notaryLocator)

				inCoins := make([]*simpleTokenParser, len(req.Inputs))
				for i, input := range req.Inputs {
					assert.Equal(t, simpleTokenSchemaID, input.SchemaId)
					if err := json.Unmarshal([]byte(input.StateDataJson), &inCoins[i]); err != nil {
						return nil, fmt.Errorf("invalid input[%d] (%s): %s", i, input.Id, err)
					}
				}
				outCoins := make([]*simpleTokenParser, len(req.Outputs))
				for i, output := range req.Outputs {
					assert.Equal(t, simpleTokenSchemaID, output.SchemaId)
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
				assert.Equal(t, signerAddr.String(), senderAddr.String(), "signer and sender should match")

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

				switch config.EndorsementMode {
				case SelfEndorsement:
					return &prototk.EndorseTransactionResponse{
						EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
						Payload:           []byte("i-hereby-endorse-this-transaction"),
					}, nil
				case NotaryEndorsement:
					return &prototk.EndorseTransactionResponse{
						EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
					}, nil
				case PrivacyGroupEndorsement:
					return &prototk.EndorseTransactionResponse{
						EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
						Payload:           signaturePayload,
					}, nil
				default:
					return nil, fmt.Errorf("unsupported endorsement mode: %s", config.EndorsementMode)

				}

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

func mustParseABIEntry(abiEntryJSON string) *abi.Entry {
	var abiEntry abi.Entry
	err := json.Unmarshal([]byte(abiEntryJSON), &abiEntry)
	if err != nil {
		panic(err)
	}
	return &abiEntry
}
