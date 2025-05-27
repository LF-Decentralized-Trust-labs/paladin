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

package fungible

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"google.golang.org/protobuf/proto"
)

type transferLockedHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

var transferLockedABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER_LOCKED,
	Inputs: abi.ParameterArray{
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var transferLockedABINullifiers = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER_LOCKED,
	Inputs: abi.ParameterArray{
		{Name: "nullifiers", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

func NewTransferLockedHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema *pb.StateSchema) *transferLockedHandler {
	return &transferLockedHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema:           coinSchema,
				MerkleTreeRootSchema: merkleTreeRootSchema,
				MerkleTreeNodeSchema: merkleTreeNodeSchema,
				DataSchema:           dataSchema,
			},
		},
		callbacks: callbacks,
	}
}

func (h *transferLockedHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var transferParams types.FungibleTransferLockedParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}

	if err := validateTransferLockedParams(ctx, transferParams); err != nil {
		return nil, err
	}
	if err := validateTransferParams(ctx, transferParams.Transfers); err != nil {
		return nil, err
	}

	return &transferParams, nil
}

func (h *transferLockedHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.(*types.FungibleTransferLockedParams)

	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	// the delegate can be an address, or a resolvable name
	_, err := pldtypes.ParseEthAddress(params.Delegate)
	if err != nil {
		// delegate is not an eth address, so we need to resolve it
		res.RequiredVerifiers = append(res.RequiredVerifiers, &pb.ResolveVerifierRequest{
			Lookup:       params.Delegate,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		})
	}
	for _, transfer := range params.Transfers {
		res.RequiredVerifiers = append(res.RequiredVerifiers, &pb.ResolveVerifierRequest{
			Lookup:       transfer.To,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		})
	}

	return res, nil
}

func (h *transferLockedHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.FungibleTransferLockedParams)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}
	var delegateAddr string
	_, err := pldtypes.ParseEthAddress(params.Delegate)
	if err != nil {
		resolvedDelegate := domain.FindVerifier(params.Delegate, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
		if resolvedDelegate == nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, params.Delegate)
		}
		delegateAddr = resolvedDelegate.Verifier
	} else {
		delegateAddr = params.Delegate
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputStates, revert, err := h.loadCoins(ctx, params.LockedInputs, useNullifiers, req.StateQueryContext)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}

	// verify that the specified inputs have at least the amount to support te transfers
	inputTotal := big.NewInt(0)
	for _, coin := range inputStates.coins {
		inputTotal = inputTotal.Add(inputTotal, coin.Amount.Int())
	}
	transferTotal := big.NewInt(0)
	for _, param := range params.Transfers {
		transferTotal = transferTotal.Add(transferTotal, param.Amount.Int())
	}
	if inputTotal.Cmp(transferTotal) < 0 {
		message := i18n.NewError(ctx, msgs.MsgErrorInsufficientInputAmount, inputTotal.Text(10), transferTotal.Text(10)).Error()
		return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
			RevertReason:   &message,
		}, nil
	}

	outputCoins, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params.Transfers, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	remainder := big.NewInt(0).Sub(inputTotal, transferTotal)
	if remainder.Sign() > 0 {
		// add the remainder as an output to the sender themselves
		remainderHex := pldtypes.HexUint256(*remainder)
		remainderParams := []*types.FungibleTransferParamEntry{
			{
				To:     tx.Transaction.From,
				Amount: &remainderHex,
			},
		}
		returnedCoins, returnedStates, err := prepareOutputsForTransfer(ctx, useNullifiers, remainderParams, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxChange, err)
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	infoStates := make([]*pb.NewState, 0, len(params.Transfers))
	for _, param := range params.Transfers {
		info, err := prepareTransactionInfoStates(ctx, param.Data, []string{tx.Transaction.From, param.To}, h.stateSchemas.DataSchema)
		if err != nil {
			return nil, err
		}
		infoStates = append(infoStates, info...)
	}

	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, inputStates.coins, outputCoins, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER_LOCKED], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress, delegateAddr)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  inputStates.states,
			OutputStates: outputStates,
			InfoStates:   infoStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "sender",
				AttestationType: pb.AttestationType_SIGN,
				Algorithm:       h.getAlgoZetoSnarkBJJ(),
				VerifierType:    zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				PayloadType:     zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK,
				Payload:         payloadBytes,
				Parties:         []string{tx.Transaction.From},
			},
		},
	}, nil
}

func (h *transferLockedHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *transferLockedHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))
	inputs, err := utxosFromInputStates(ctx, req.InputStates, inputSize)
	if err != nil {
		return nil, err
	}
	outputs, err := utxosFromOutputStates(ctx, req.OutputStates, inputSize)
	if err != nil {
		return nil, err
	}

	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	params := map[string]any{
		"inputs":  inputs,
		"outputs": outputs,
		"proof":   common.EncodeProof(proofRes.Proof),
		"data":    data,
	}
	transferFunction := getTransferLockedABI(tx.DomainConfig.TokenName)
	if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
		delete(params, "inputs")
		params["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
		params["root"] = proofRes.PublicInputs["root"]
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(transferFunction)
	if err != nil {
		return nil, err
	}

	signer := &tx.Params.(*types.FungibleTransferLockedParams).Delegate
	if req.Transaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION {
		// if called as prepare, it means the delegate might not exist locally
		// and the signer doesn't matter because the caller is interested in the
		// prepared transaction, not the signature
		signer = &req.Transaction.From
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  signer,
		},
	}, nil
}

func (h *transferLockedHandler) loadCoins(ctx context.Context, ids []*pldtypes.HexUint256, useNullifiers bool, stateQueryContext string) (inputs *preparedInputs, revert bool, err error) {
	inputIDs := make([]any, 0, len(ids))
	for _, input := range ids {
		if !input.NilOrZero() {
			inputIDs = append(inputIDs, input.String())
		}
	}

	queryBuilder := query.NewQueryBuilder().In(".id", inputIDs)
	inputStates, err := findAvailableStates(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, stateQueryContext, queryBuilder.Query().String())
	if err != nil {
		return nil, false, err
	}
	if len(inputStates) != len(inputIDs) {
		return nil, true, i18n.NewError(ctx, msgs.MsgFailedToQueryStatesById, len(inputIDs), len(inputStates))
	}

	inputCoins := make([]*types.ZetoCoin, len(inputStates))
	stateRefs := make([]*pb.StateRef, 0, len(inputStates))
	for i, state := range inputStates {
		err := json.Unmarshal([]byte(state.DataJson), &inputCoins[i])
		if err != nil {
			return nil, true, i18n.NewError(ctx, msgs.MsgErrorUnmarshalStateData, err)
		}
		if inputCoins[i].Locked == false {
			return nil, true, i18n.NewError(ctx, msgs.MsgErrorInputNotLocked, state.Id)
		}
		stateRefs = append(stateRefs, &pb.StateRef{
			SchemaId: state.SchemaId,
			Id:       state.Id,
		})

	}
	return &preparedInputs{
		coins:  inputCoins,
		states: stateRefs,
	}, false, nil
}

func validateTransferLockedParams(ctx context.Context, params types.FungibleTransferLockedParams) error {
	if params.LockedInputs == nil || len(params.LockedInputs) == 0 {
		return i18n.NewError(ctx, msgs.MsgErrorMissingLockInputs)
	}
	if params.Delegate == "" {
		return i18n.NewError(ctx, msgs.MsgErrorMissingLockDelegate)
	}
	return nil
}

func getTransferLockedABI(tokenName string) *abi.Entry {
	transferFunction := transferLockedABI
	if common.IsNullifiersToken(tokenName) {
		transferFunction = transferLockedABINullifiers
	}
	return transferFunction
}
