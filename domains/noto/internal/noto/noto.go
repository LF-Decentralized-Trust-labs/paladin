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
	_ "embed"
	"encoding/json"
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte

//go:embed abis/INoto.json
var notoInterfaceJSON []byte

func NewNoto(callbacks plugintk.DomainCallbacks) plugintk.DomainAPI {
	return &Noto{Callbacks: callbacks}
}

type Noto struct {
	Callbacks plugintk.DomainCallbacks

	config            types.DomainConfig
	chainID           int64
	coinSchema        *prototk.StateSchema
	factoryABI        abi.ABI
	contractABI       abi.ABI
	transferSignature string
	approvedSignature string
}

type NotoDeployParams struct {
	Name          string                    `json:"name,omitempty"`
	TransactionID string                    `json:"transactionId"`
	Notary        string                    `json:"notary"`
	Config        ethtypes.HexBytes0xPrefix `json:"config"`
}

type NotoTransfer_Event struct {
	Inputs    []tktypes.Bytes32 `json:"inputs"`
	Outputs   []tktypes.Bytes32 `json:"outputs"`
	Signature tktypes.HexBytes  `json:"signature"`
	Data      tktypes.HexBytes  `json:"data"`
}

type NotoApproved_Event struct {
	Delegate  tktypes.EthAddress `json:"delegate"`
	TXHash    tktypes.Bytes32    `json:"txhash"`
	Signature tktypes.HexBytes   `json:"signature"`
	Data      tktypes.HexBytes   `json:"data"`
}

type gatheredCoins struct {
	inCoins   []*types.NotoCoin
	inStates  []*prototk.StateRef
	inTotal   *big.Int
	outCoins  []*types.NotoCoin
	outStates []*prototk.StateRef
	outTotal  *big.Int
}

func getEventSignature(ctx context.Context, abi abi.ABI, eventName string) (string, error) {
	event := abi.Events()[eventName]
	if event == nil {
		return "", i18n.NewError(ctx, msgs.MsgUnknownEvent, eventName)
	}
	return event.SolString(), nil
}

func (n *Noto) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	err := json.Unmarshal([]byte(req.ConfigJson), &n.config)
	if err != nil {
		return nil, err
	}

	factory := domain.LoadBuild(notoFactoryJSON)
	contract := domain.LoadBuild(notoInterfaceJSON)

	n.chainID = req.ChainId
	n.factoryABI = factory.ABI
	n.contractABI = contract.ABI

	n.transferSignature, err = getEventSignature(ctx, contract.ABI, "NotoTransfer")
	if err != nil {
		return nil, err
	}
	n.approvedSignature, err = getEventSignature(ctx, contract.ABI, "NotoApproved")
	if err != nil {
		return nil, err
	}

	schemaJSON, err := json.Marshal(types.NotoCoinABI)
	if err != nil {
		return nil, err
	}

	var events abi.ABI
	for _, entry := range contract.ABI {
		if entry.Type == abi.Event {
			events = append(events, entry)
		}
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}

	return &prototk.ConfigureDomainResponse{
		DomainConfig: &prototk.DomainConfig{
			AbiStateSchemasJson: []string{string(schemaJSON)},
			BaseLedgerSubmitConfig: &prototk.BaseLedgerSubmitConfig{
				SubmitMode: prototk.BaseLedgerSubmitConfig_ENDORSER_SUBMISSION,
			},
			AbiEventsJson: string(eventsJSON),
		},
	}, nil
}

func (n *Noto) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	n.coinSchema = req.AbiStateSchemas[0]
	return &prototk.InitDomainResponse{}, nil
}

func (n *Noto) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	params, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	return &prototk.InitDeployResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       params.Notary,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (n *Noto) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	params, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	notary := domain.FindVerifier(params.Notary, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if notary == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "notary")
	}
	config := &types.NotoConfigInput_V0{
		NotaryLookup: notary.Lookup,
	}
	configABI, err := n.encodeConfig(config)
	if err != nil {
		return nil, err
	}

	deployParams := &NotoDeployParams{
		Name:          params.Implementation,
		TransactionID: req.Transaction.TransactionId,
		Notary:        notary.Verifier,
		Config:        configABI,
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}
	functionName := "deploy"
	if deployParams.Name != "" {
		functionName = "deployImplementation"
	}
	functionJSON, err := json.Marshal(n.factoryABI.Functions()[functionName])
	if err != nil {
		return nil, err
	}

	return &prototk.PrepareDeployResponse{
		Transaction: &prototk.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &config.NotaryLookup,
	}, nil
}

func (n *Noto) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Init(ctx, tx, req)
}

func (n *Noto) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Assemble(ctx, tx, req)
}

func (n *Noto) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Endorse(ctx, tx, req)
}

func (n *Noto) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Prepare(ctx, tx, req)
}

func (n *Noto) encodeConfig(config *types.NotoConfigInput_V0) ([]byte, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	encodedConfig, err := types.NotoConfigInputABI_V0.EncodeABIDataJSON(configJSON)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(types.NotoConfigID_V0)+len(encodedConfig))
	result = append(result, types.NotoConfigID_V0...)
	result = append(result, encodedConfig...)
	return result, nil
}

func (n *Noto) decodeConfig(ctx context.Context, domainConfig []byte) (*types.NotoConfigOutput_V0, error) {
	var configSelector ethtypes.HexBytes0xPrefix
	if len(domainConfig) >= 4 {
		configSelector = ethtypes.HexBytes0xPrefix(domainConfig[0:4])
	}
	if configSelector.String() != types.NotoConfigID_V0.String() {
		return nil, i18n.NewError(ctx, msgs.MsgUnexpectedConfigType, configSelector)
	}
	configValues, err := types.NotoConfigOutputABI_V0.DecodeABIDataCtx(ctx, domainConfig[4:], 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := tktypes.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config types.NotoConfigOutput_V0
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (n *Noto) validateDeploy(tx *prototk.DeployTransactionSpecification) (*types.ConstructorParams, error) {
	var params types.ConstructorParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (n *Noto) validateTransaction(ctx context.Context, tx *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, nil, err
	}

	domainConfig, err := n.decodeConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, nil, err
	}

	abi := types.NotoABI.Functions()[functionABI.Name]
	handler := n.GetHandler(functionABI.Name)
	if abi == nil || handler == nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgUnknownFunction, functionABI.Name)
	}
	params, err := handler.ValidateParams(ctx, domainConfig, tx.FunctionParamsJson)
	if err != nil {
		return nil, nil, err
	}

	signature, err := abi.SolidityStringCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	if tx.FunctionSignature != signature {
		return nil, nil, i18n.NewError(ctx, msgs.MsgUnexpectedFunctionSignature, functionABI.Name, signature, tx.FunctionSignature)
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, nil, err
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, handler, nil
}

func (n *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}

func (n *Noto) parseCoinList(ctx context.Context, label string, states []*prototk.EndorsableState) ([]*types.NotoCoin, []*prototk.StateRef, *big.Int, error) {
	var err error
	statesUsed := make(map[string]bool)
	coins := make([]*types.NotoCoin, len(states))
	refs := make([]*prototk.StateRef, len(states))
	total := big.NewInt(0)
	for i, state := range states {
		if state.SchemaId != n.coinSchema.Id {
			return nil, nil, nil, i18n.NewError(ctx, msgs.MsgUnknownSchema, state.SchemaId)
		}
		if statesUsed[state.Id] {
			return nil, nil, nil, i18n.NewError(ctx, msgs.MsgDuplicateStateInList, label, i, state.Id)
		}
		statesUsed[state.Id] = true
		if coins[i], err = n.unmarshalCoin(state.StateDataJson); err != nil {
			return nil, nil, nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
		}
		refs[i] = &prototk.StateRef{
			SchemaId: state.SchemaId,
			Id:       state.Id,
		}
		total = total.Add(total, coins[i].Amount.BigInt())
	}
	return coins, refs, total, nil
}

func (n *Noto) gatherCoins(ctx context.Context, inputs, outputs []*prototk.EndorsableState) (*gatheredCoins, error) {
	inCoins, inStates, inTotal, err := n.parseCoinList(ctx, "input", inputs)
	if err != nil {
		return nil, err
	}
	outCoins, outStates, outTotal, err := n.parseCoinList(ctx, "output", outputs)
	if err != nil {
		return nil, err
	}
	return &gatheredCoins{
		inCoins:   inCoins,
		inStates:  inStates,
		inTotal:   inTotal,
		outCoins:  outCoins,
		outStates: outStates,
		outTotal:  outTotal,
	}, nil
}

func (n *Noto) FindCoins(ctx context.Context, contractAddress ethtypes.Address0xHex, query string) ([]*types.NotoCoin, error) {
	states, err := n.findAvailableStates(ctx, contractAddress.String(), query)
	if err != nil {
		return nil, err
	}

	coins := make([]*types.NotoCoin, len(states))
	for i, state := range states {
		if coins[i], err = n.unmarshalCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}

func (n *Noto) encodeTransactionData(ctx context.Context, transaction *prototk.TransactionSpecification) (tktypes.HexBytes, error) {
	txID, err := tktypes.ParseHexBytes(ctx, transaction.TransactionId)
	if err != nil {
		return nil, err
	}

	var data []byte
	data = append(data, types.NotoTransactionData_V0...)
	data = append(data, txID...)
	return data, nil
}

func (n *Noto) decodeTransactionData(data tktypes.HexBytes) (txID tktypes.HexBytes) {
	if len(data) < 4 {
		return nil
	}
	dataPrefix := data[0:4]
	if dataPrefix.String() != types.NotoTransactionData_V0.String() {
		return nil
	}
	return data[4:]
}

func (n *Noto) parseStatesFromEvent(txID tktypes.HexBytes, states []tktypes.Bytes32) []*prototk.StateUpdate {
	refs := make([]*prototk.StateUpdate, len(states))
	for i, state := range states {
		refs[i] = &prototk.StateUpdate{
			Id:            state.String(),
			TransactionId: txID.String(),
		}
	}
	return refs
}

func (n *Noto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	var events []*blockindexer.EventWithData
	if err := json.Unmarshal([]byte(req.JsonEvents), &events); err != nil {
		return nil, err
	}

	var res prototk.HandleEventBatchResponse
	for _, ev := range events {
		switch ev.SoliditySignature {
		case n.transferSignature:
			var transfer NotoTransfer_Event
			if err := json.Unmarshal(ev.Data, &transfer); err == nil {
				txID := n.decodeTransactionData(transfer.Data)
				res.TransactionsComplete = append(res.TransactionsComplete, txID.String())
				res.SpentStates = append(res.SpentStates, n.parseStatesFromEvent(txID, transfer.Inputs)...)
				res.ConfirmedStates = append(res.ConfirmedStates, n.parseStatesFromEvent(txID, transfer.Outputs)...)
			}

		case n.approvedSignature:
			var approved NotoApproved_Event
			if err := json.Unmarshal(ev.Data, &approved); err == nil {
				txID := n.decodeTransactionData(approved.Data)
				res.TransactionsComplete = append(res.TransactionsComplete, txID.String())
			}
		}
	}
	return &res, nil
}

func (n *Noto) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}
