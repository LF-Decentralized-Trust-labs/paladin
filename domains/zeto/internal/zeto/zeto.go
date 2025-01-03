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
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/signer"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type Zeto struct {
	Callbacks plugintk.DomainCallbacks

	name                     string
	config                   *types.DomainFactoryConfig
	chainID                  int64
	coinSchema               *prototk.StateSchema
	merkleTreeRootSchema     *prototk.StateSchema
	merkleTreeNodeSchema     *prototk.StateSchema
	mintSignature            string
	transferSignature        string
	transferWithEncSignature string
	withdrawSignature        string
	lockSignature            string
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

type WithdrawEvent struct {
	Amount tktypes.HexUint256   `json:"amount"`
	Inputs []tktypes.HexUint256 `json:"inputs"`
	Output tktypes.HexUint256   `json:"output"`
	Data   tktypes.HexBytes     `json:"data"`
}

type LockedEvent struct {
	UTXOs     []tktypes.HexUint256 `json:"utxos"`
	Delegate  tktypes.EthAddress   `json:"delegate"`
	Submitter tktypes.EthAddress   `json:"submitter"`
	Data      tktypes.HexBytes     `json:"data"`
}

var factoryDeployABI = &abi.Entry{
	Type: abi.Function,
	Name: "deploy",
	Inputs: abi.ParameterArray{
		{Name: "transactionId", Type: "bytes32"},
		{Name: "tokenName", Type: "string"},
		{Name: "initialOwner", Type: "address"},
		{Name: "data", Type: "bytes"},
	},
}

func New(callbacks plugintk.DomainCallbacks) *Zeto {
	return &Zeto{
		Callbacks: callbacks,
	}
}

func (z *Zeto) Name() string {
	return z.name
}

func (z *Zeto) CoinSchemaID() string {
	return z.coinSchema.Id
}

func (z *Zeto) getAlgoZetoSnarkBJJ() string {
	return zetosignerapi.AlgoDomainZetoSnarkBJJ(z.name)
}

func (z *Zeto) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	var config types.DomainFactoryConfig
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseDomainConfig, err)
	}

	z.name = req.Name
	z.config = &config
	z.chainID = req.ChainId

	schemas, err := getStateSchemas(ctx)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorConfigZetoDomain, err)
	}

	events := getAllZetoEventAbis()
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalZetoEventAbis, err)
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
			CustomHashFunction:  true,
			AbiStateSchemasJson: schemas,
			AbiEventsJson:       string(eventsJSON),
			SigningAlgorithms:   signingAlgos,
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
	_, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitDeployParams, err)
	}
	return &prototk.InitDeployResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       req.Transaction.From,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (z *Zeto) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	initParams, err := z.validateDeploy(req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidatePrepDeployParams, err)
	}
	circuitId, err := z.config.GetCircuitId(ctx, initParams.TokenName)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindCircuitId, err)
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
	functionJSON, err := json.Marshal(factoryDeployABI)
	if err != nil {
		return nil, err
	}

	from := req.Transaction.From
	return &prototk.PrepareDeployResponse{
		Transaction: &prototk.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &from,
	}, nil
}

func (z *Zeto) InitContract(ctx context.Context, req *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
	var zetoContractConfigJSON []byte
	domainConfig, err := z.decodeDomainConfig(ctx, req.ContractConfig)
	if err == nil {
		zetoContractConfigJSON, err = json.Marshal(domainConfig)
	}
	if err != nil {
		// This on-chain contract has invalid configuration - not an error in our process
		return &prototk.InitContractResponse{Valid: false}, nil
	}

	return &prototk.InitContractResponse{
		Valid: true,
		ContractConfig: &prototk.ContractConfig{
			ContractConfigJson:   string(zetoContractConfigJSON),
			CoordinatorSelection: prototk.ContractConfig_COORDINATOR_SENDER,
			SubmitterSelection:   prototk.ContractConfig_SUBMITTER_SENDER,
		},
	}, nil
}

func (z *Zeto) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitTxSpec, err)
	}
	return handler.Init(ctx, tx, req)
}

func (z *Zeto) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateAssembleTxSpec, err)
	}
	return handler.Assemble(ctx, tx, req)
}

func (z *Zeto) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateEndorseTxParams, err)
	}
	return handler.Endorse(ctx, tx, req)
}

func (z *Zeto) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	tx, handler, err := z.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidatePrepTxSpec, err)
	}
	return handler.Prepare(ctx, tx, req)
}

func (z *Zeto) GetHandler(method string) types.DomainHandler {
	switch method {
	case "mint":
		return &mintHandler{zeto: z}
	case "transfer":
		return &transferHandler{zeto: z}
	case "lock":
		return &lockHandler{zeto: z}
	case "deposit":
		return &depositHandler{zeto: z}
	case "withdraw":
		return &withdrawHandler{zeto: z}
	default:
		return nil
	}
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*types.DomainInstanceConfig, error) {
	configValues, err := types.DomainInstanceConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorAbiDecodeDomainInstanceConfig, err)
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
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalFuncAbi, err)
	}

	var domainConfig *types.DomainInstanceConfig
	err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &domainConfig)
	if err != nil {
		return nil, nil, err
	}

	abi := types.ZetoABI.Functions()[functionABI.Name]
	handler := z.GetHandler(functionABI.Name)
	if abi == nil || handler == nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgUnknownFunction, functionABI.Name)
	}
	params, err := handler.ValidateParams(ctx, domainConfig, tx.FunctionParamsJson)
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorValidateFuncParams, err)
	}

	signature := abi.SolString()
	if tx.FunctionSignature != signature {
		return nil, nil, i18n.NewError(ctx, msgs.MsgUnexpectedFuncSignature, functionABI.Name, signature, tx.FunctionSignature)
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    domainConfig,
		Params:          params,
	}, handler, nil
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
		case "UTXOWithdraw":
			z.withdrawSignature = event.SolString()
		case "UTXOsLocked":
			z.lockSignature = event.SolString()
		}
	}
}

func (z *Zeto) HandleEventBatch(ctx context.Context, req *prototk.HandleEventBatchRequest) (*prototk.HandleEventBatchResponse, error) {
	var domainConfig *types.DomainInstanceConfig
	err := json.Unmarshal([]byte(req.ContractInfo.ContractConfigJson), &domainConfig)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorAbiDecodeDomainInstanceConfig, err)
	}

	contractAddress, err := tktypes.ParseEthAddress(req.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}

	var res prototk.HandleEventBatchResponse
	var errors []string
	var smtName string
	var storage smt.StatesStorage
	var tree core.SparseMerkleTree
	if common.IsNullifiersToken(domainConfig.TokenName) {
		smtName = smt.MerkleTreeName(domainConfig.TokenName, contractAddress)
		storage = smt.NewStatesStorage(z.Callbacks, smtName, req.StateQueryContext, z.merkleTreeRootSchema.Id, z.merkleTreeNodeSchema.Id)
		tree, err = smt.NewSmt(storage)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorNewSmt, smtName, err)
		}
	}
	for _, ev := range req.Events {
		var err error
		switch ev.SoliditySignature {
		case z.mintSignature:
			err = z.handleMintEvent(ctx, tree, storage, ev, domainConfig.TokenName, &res)
		case z.transferSignature:
			err = z.handleTransferEvent(ctx, tree, storage, ev, domainConfig.TokenName, &res)
		case z.transferWithEncSignature:
			err = z.handleTransferWithEncryptionEvent(ctx, tree, storage, ev, domainConfig.TokenName, &res)
		case z.withdrawSignature:
			err = z.handleWithdrawEvent(ctx, tree, storage, ev, domainConfig.TokenName, &res)
		case z.lockSignature:
			err = z.handleLockedEvent(ctx, ev, &res)
		}
		if err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return &res, i18n.NewError(ctx, msgs.MsgErrorHandleEvents, formatErrors(errors))
	}
	if common.IsNullifiersToken(domainConfig.TokenName) {
		newStatesForSMT, err := storage.GetNewStates()
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGetNewSmtStates, smtName, err)
		}
		if len(newStatesForSMT) > 0 {
			res.NewStates = append(res.NewStates, newStatesForSMT...)
		}
	}
	return &res, nil
}

func (z *Zeto) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	verifier, err := z.snarkProver.GetVerifier(ctx, req.Algorithm, req.VerifierType, req.PrivateKey)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGetVerifier, err)
	}
	return &prototk.GetVerifierResponse{
		Verifier: verifier,
	}, nil
}

func intTo32ByteSlice(bigInt *big.Int) (res []byte) {
	return bigInt.FillBytes(make([]byte, 32))
}

func (z *Zeto) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	switch req.PayloadType {
	case zetosignerapi.PAYLOAD_DOMAIN_ZETO_NULLIFIER:
		var coin *types.ZetoCoin
		var hashInt *big.Int
		keyPair, err := signer.NewBabyJubJubPrivateKey(req.PrivateKey)
		if err == nil {
			err = json.Unmarshal(req.Payload, &coin)
		}
		if err == nil {
			hashInt, err = signer.CalculateNullifier(coin.Amount.Int(), coin.Salt.Int(), babyjub.SkToBigInt(keyPair))
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgNullifierGenerationFailed)
		}
		return &prototk.SignResponse{
			Payload: intTo32ByteSlice(hashInt),
		}, nil
	case zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK:
		proof, err := z.snarkProver.Sign(ctx, req.Algorithm, req.PayloadType, req.PrivateKey, req.Payload)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorSign, err)
		}
		return &prototk.SignResponse{
			Payload: proof,
		}, nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownSignPayload, req.PayloadType)
	}
}

func (z *Zeto) ValidateStateHashes(ctx context.Context, req *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
	var res prototk.ValidateStateHashesResponse
	for _, state := range req.States {
		log.L(ctx).Debugf("validating state hashes: %+v\n", state)
		var coin types.ZetoCoin
		err := json.Unmarshal([]byte(state.StateDataJson), &coin)
		if err != nil {
			log.L(ctx).Errorf("Error unmarshalling state data: %s", err)
			return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
		}
		hash, err := coin.Hash(ctx)
		if err != nil {
			log.L(ctx).Errorf("Error hashing state data: %s", err)
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
		}
		if state.Id == "" {
			// if the requested state ID is empty, we simply set it
			res.StateIds = append(res.StateIds, hash.String())
		} else {
			// if the requested state ID is set, we compare it with the calculated hash
			if hash.String() != state.Id {
				log.L(ctx).Errorf("State hash mismatch (hashed vs. received): %s != %s", hash.String(), state.Id)
				return nil, i18n.NewError(ctx, msgs.MsgErrorStateHashMismatch, hash.String(), state.Id)
			}
			res.StateIds = append(res.StateIds, state.Id)
		}
	}
	return &res, nil
}

func (z *Zeto) InitCall(ctx context.Context, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (z *Zeto) ExecCall(ctx context.Context, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (z *Zeto) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (*prototk.BuildReceiptResponse, error) {
	// TODO: Event logs for transfers would be great for Noto
	return nil, i18n.NewError(ctx, msgs.MsgNoDomainReceipt)
}
