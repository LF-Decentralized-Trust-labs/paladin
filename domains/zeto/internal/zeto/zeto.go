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
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/node"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signer/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/ZetoFactory.json
var factoryJSONBytes []byte // From "gradle copySolidity"
//go:embed abis/IZetoFungibleInitializable.json
var zetoFungibleInitializableABIBytes []byte // From "gradle copySolidity"
//go:embed abis/IZetoNonFungibleInitializable.json
var zetoNonFungibleInitializableABIBytes []byte // From "gradle copySolidity"

type Zeto struct {
	Callbacks plugintk.DomainCallbacks

	name                     string
	config                   *types.DomainFactoryConfig
	chainID                  int64
	coinSchema               *prototk.StateSchema
	merkleTreeRootSchema     *prototk.StateSchema
	merkleTreeNodeSchema     *prototk.StateSchema
	factoryABI               abi.ABI
	mintSignature            string
	transferSignature        string
	transferWithEncSignature string
	snarkProver              signerapi.InMemorySigner
}

type MintEvent struct {
	Outputs []tktypes.HexUint256 `json:"outputs"`
	Data    tktypes.HexBytes     `json:"data"`
}

type TransferEvent struct {
	Inputs  []tktypes.HexUint256 `json:"inputs"`
	Outputs []tktypes.HexUint256 `json:"outputs"`
	Data    tktypes.HexBytes     `json:"data"`
}

type TransferWithEncryptedValuesEvent struct {
	Inputs          []tktypes.HexUint256 `json:"inputs"`
	Outputs         []tktypes.HexUint256 `json:"outputs"`
	Data            tktypes.HexBytes     `json:"data"`
	EncryptionNonce tktypes.HexUint256   `json:"encryptionNonce"`
	EncryptedValues []tktypes.HexUint256 `json:"encryptedValues"`
}

func New(callbacks plugintk.DomainCallbacks) *Zeto {
	return &Zeto{
		Callbacks: callbacks,
	}
}

func (z *Zeto) getAlgoZetoSnarkBJJ() string {
	return zetosigner.AlgoDomainZetoSnarkBJJ(z.name)
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	var config types.DomainFactoryConfig
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse domain config json. %s", err)
	}

	z.name = req.Name
	z.config = &config
	z.chainID = req.ChainId

	factory := domain.LoadBuildLinked(factoryJSONBytes, config.Libraries)
	z.factoryABI = factory.ABI

	schemas, err := getStateSchemas()
	if err != nil {
		return nil, fmt.Errorf("failed to configure Zeto domain. %s", err)
	}

	events := getAllZetoEventAbis()
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return nil, fmt.Errorf("failed to configure Zeto domain. Failed to marshal Zeto event abis. %s", err)
	}

	z.registerEventSignatures(events)

	var signingAlgos map[string]int32
	if config.SnarkProver.CircuitsDir != "" {
		// Only build the prover and enable the algorithms for signing if circuits configured
		z.snarkProver, err = zetosigner.NewSnarkProver(&config.SnarkProver)
		if err != nil {
			return nil, err
		}
		signingAlgos = map[string]int32{
			z.getAlgoZetoSnarkBJJ(): 32,
		}
	}

	return &prototk.ConfigureDomainResponse{
		DomainConfig: &prototk.DomainConfig{
			AbiStateSchemasJson: schemas,
			BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{
				SubmitMode: prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION,
			},
			AbiEventsJson:     string(eventsJSON),
			SigningAlgorithms: signingAlgos,
		},
	}, nil
}

func (z *Zeto) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	z.coinSchema = req.AbiStateSchemas[0]
	z.merkleTreeRootSchema = req.AbiStateSchemas[1]
	z.merkleTreeNodeSchema = req.AbiStateSchemas[2]
	return &prototk.InitDomainResponse{}, nil
}

func (z *Zeto) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	initParams, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate init deploy parameters. %s", err)
	}
	return &prototk.InitDeployResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       initParams.From,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (z *Zeto) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	initParams, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate prepare deploy parameters. %s", err)
	}
	circuitId, err := z.config.GetCircuitId(initParams.TokenName)
	if err != nil {
		return nil, fmt.Errorf("failed to find circuit ID based on the token name. %s", err)
	}
	config := &types.DomainInstanceConfig{
		CircuitId: circuitId,
		TokenName: initParams.TokenName,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	encoded, err := types.DomainInstanceConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	deployParams := &types.DeployParams{
		TransactionID: req.Transaction.TransactionId,
		Data:          tktypes.HexBytes(encoded),
		TokenName:     initParams.TokenName,
		InitialOwner:  req.ResolvedVerifiers[0].Verifier, // TODO: allow the initial owner to be specified by the deploy request
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(z.factoryABI.Functions()["deploy"])
	if err != nil {
		return nil, err
	}

	return &prototk.PrepareDeployResponse{
		Transaction: &prototk.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &initParams.From,
	}, nil
}

func (z *Zeto) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate init transaction spec. %s", err)
	}
	return handler.Init(ctx, tx, req)
}

func (z *Zeto) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate assemble transaction spec. %s", err)
	}
	return handler.Assemble(ctx, tx, req)
}

func (z *Zeto) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate endorse transaction spec. %s", err)
	}
	return handler.Endorse(ctx, tx, req)
}

func (z *Zeto) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to validate prepare transaction spec. %s", err)
	}
	return handler.Prepare(ctx, tx, req)
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainInstanceConfig, error) {
	configValues, err := types.DomainInstanceConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := tktypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config types.DomainInstanceConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (z *Zeto) validateDeploy(tx *prototk.DeployTransactionSpecification) (*types.InitializerParams, error) {
	var params types.InitializerParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (z *Zeto) validateTransaction(ctx context.Context, tx *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal function abi json. %s", err)
	}

	domainConfig, err := z.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode domain config. %s", err)
	}

	abi := types.ZetoABI.Functions()[functionABI.Name]
	handler := z.GetHandler(functionABI.Name)
	if abi == nil || handler == nil {
		return nil, nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}
	params, err := handler.ValidateParams(ctx, domainConfig, tx.FunctionParamsJson)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate function params. %s", err)
	}

	signature, _, err := abi.SolidityDefCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	if tx.FunctionSignature != signature {
		return nil, nil, fmt.Errorf("unexpected signature for function '%s': expected=%s actual=%s", functionABI.Name, signature, tx.FunctionSignature)
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode contract address. %s", err)
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, handler, nil
}

func (z *Zeto) FindCoins(ctx context.Context, contractAddress ethtypes.Address0xHex, query string) ([]*types.ZetoCoin, error) {
	states, err := z.findAvailableStates(ctx, contractAddress.String(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to find available states. %s", err)
	}

	coins := make([]*types.ZetoCoin, len(states))
	for i, state := range states {
		if coins[i], err = z.makeCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}

func (z *Zeto) registerEventSignatures(eventAbis abi.ABI) {
	for _, event := range eventAbis.Events() {
		switch event.Name {
		case "UTXOMint":
			z.mintSignature = event.SolString()
		case "UTXOTransfer":
			z.transferSignature = event.SolString()
		case "UTXOTransferWithEncryptedValues":
			z.transferWithEncSignature = event.SolString()
		}
	}
}

func (z *Zeto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	var events []*blockindexer.EventWithData
	if err := json.Unmarshal([]byte(req.JsonEvents), &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events. %s", err)
	}
	bytes := tktypes.MustParseHexBytes(req.ConfigBytes)
	cv, err := types.DomainInstanceConfigABI.DecodeABIData(bytes, 0)
	if err != nil {
		return nil, err
	}
	j, err := cv.JSON()
	if err != nil {
		return nil, err
	}
	domainConfig := &types.DomainInstanceConfig{}
	if err := json.Unmarshal(j, domainConfig); err != nil {
		return nil, err
	}

	var res prototk.HandleEventBatchResponse
	for _, ev := range events {
		switch ev.SoliditySignature {
		case z.mintSignature:
			var mint MintEvent
			if err := json.Unmarshal(ev.Data, &mint); err == nil {
				txID := decodeTransactionData(mint.Data)
				res.TransactionsComplete = append(res.TransactionsComplete, txID.String())
				res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, mint.Outputs)...)
				if domainConfig.TokenName == constants.TOKEN_ANON_NULLIFIER {
					newStates, err := z.updateMerkleTree(txID, domainConfig.TokenName, ev.Address, mint.Outputs)
					if err != nil {
						return nil, err
					}
					res.NewStates = append(res.NewStates, newStates...)
				}
			} else {
				log.L(ctx).Errorf("Failed to unmarshal mint event: %s", err)
			}
		case z.transferSignature:
			var transfer TransferEvent
			if err := json.Unmarshal(ev.Data, &transfer); err == nil {
				txID := decodeTransactionData(transfer.Data)
				res.TransactionsComplete = append(res.TransactionsComplete, txID.String())
				res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, transfer.Inputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, transfer.Outputs)...)
				if domainConfig.TokenName == constants.TOKEN_ANON_NULLIFIER {
					newStates, err := z.updateMerkleTree(txID, domainConfig.TokenName, ev.Address, transfer.Outputs)
					if err != nil {
						return nil, err
					}
					res.NewStates = append(res.NewStates, newStates...)
				}
			} else {
				log.L(ctx).Errorf("Failed to unmarshal transfer event: %s", err)
			}
		case z.transferWithEncSignature:
			var transfer TransferWithEncryptedValuesEvent
			if err := json.Unmarshal(ev.Data, &transfer); err == nil {
				txID := decodeTransactionData(transfer.Data)
				res.TransactionsComplete = append(res.TransactionsComplete, txID.String())
				res.SpentStates = append(res.SpentStates, parseStatesFromEvent(txID, transfer.Inputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, parseStatesFromEvent(txID, transfer.Outputs)...)
				if domainConfig.TokenName == constants.TOKEN_ANON_NULLIFIER {
					newStates, err := z.updateMerkleTree(txID, domainConfig.TokenName, ev.Address, transfer.Outputs)
					if err != nil {
						return nil, err
					}
					res.NewStates = append(res.NewStates, newStates...)
				}
			} else {
				log.L(ctx).Errorf("Failed to unmarshal transfer with encrypted values event: %s", err)
			}
		}
	}
	return &res, nil
}

func (z *Zeto) updateMerkleTree(txID tktypes.HexBytes, tokenName string, address tktypes.EthAddress, output []tktypes.HexUint256) ([]*prototk.NewLocalState, error) {
	var newStates []*prototk.NewLocalState
	for _, out := range output {
		states, err := z.addOutputToMerkleTree(txID, tokenName, address, out)
		if err != nil {
			return nil, err
		}
		newStates = append(newStates, states...)
	}
	return newStates, nil
}

func (z *Zeto) addOutputToMerkleTree(txID tktypes.HexBytes, tokenName string, address tktypes.EthAddress, output tktypes.HexUint256) ([]*prototk.NewLocalState, error) {
	smtName := smt.MerkleTreeName(tokenName, address.Address0xHex())
	storage, tree, err := smt.New(z.Callbacks, smtName, address.Address0xHex(), z.merkleTreeRootSchema.Id, z.merkleTreeNodeSchema.Id)
	if err != nil {
		return nil, fmt.Errorf("Failed to create Merkle tree for %s: %s", smtName, err)
	}
	idx, err := node.NewNodeIndexFromBigInt(output.Int())
	if err != nil {
		return nil, fmt.Errorf("Failed to create node index for %s: %s", output.String(), err)
	}
	n := node.NewIndexOnly(idx)
	leaf, err := node.NewLeafNode(n)
	if err != nil {
		return nil, fmt.Errorf("Failed to create leaf node for %s: %s", output.String(), err)
	}
	err = tree.AddLeaf(leaf)
	if err != nil {
		return nil, fmt.Errorf("Failed to add leaf node for %s: %s", output.String(), err)
	}
	newStates := storage.GetNewStates()
	for _, state := range newStates {
		state.TransactionId = txID.String()
	}
	return newStates, nil
}

func encodeTransactionData(ctx context.Context, transaction *prototk.TransactionSpecification) (tktypes.HexBytes, error) {
	txID, err := tktypes.ParseHexBytes(ctx, transaction.TransactionId)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction id. %s", err)
	}
	var data []byte
	data = append(data, types.ZetoTransactionData_V0...)
	data = append(data, txID...)
	return data, nil
}

func decodeTransactionData(data tktypes.HexBytes) (txID tktypes.HexBytes) {
	if len(data) < 4 {
		return nil
	}
	dataPrefix := data[0:4]
	if dataPrefix.String() != types.ZetoTransactionData_V0.String() {
		return nil
	}
	return data[4:]
}

func parseStatesFromEvent(txID tktypes.HexBytes, states []tktypes.HexUint256) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func (z *Zeto) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	verifier, err := z.snarkProver.GetVerifier(ctx, req.Algorithm, req.VerifierType, req.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &prototk.GetVerifierResponse{
		Verifier: verifier,
	}, nil
}

func (z *Zeto) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	proof, err := z.snarkProver.Sign(ctx, req.Algorithm, req.PayloadType, req.PrivateKey, req.Payload)
	if err != nil {
		return nil, err
	}
	return &prototk.SignResponse{
		Payload: proof,
	}, nil
}
