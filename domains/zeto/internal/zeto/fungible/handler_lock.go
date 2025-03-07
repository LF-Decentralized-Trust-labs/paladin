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
	"slices"
	"strings"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &lockHandler{}

type lockHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

type lockedStatesInfo struct {
	Size *tktypes.HexUint64 `json:"size"`
}

var lockABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_LOCK,
	Inputs: abi.ParameterArray{
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "lockedOutputs", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "delegate", Type: "address"},
		{Name: "data", Type: "bytes"},
	},
}

var lockABINullifiers = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_LOCK,
	Inputs: abi.ParameterArray{
		{Name: "nullifiers", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "lockedOutputs", Type: "uint256[]"},
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "delegate", Type: "address"},
		{Name: "data", Type: "bytes"},
	},
}

func NewLockHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, infoSchema *pb.StateSchema) *lockHandler {
	return &lockHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema:           coinSchema,
				MerkleTreeRootSchema: merkleTreeRootSchema,
				MerkleTreeNodeSchema: merkleTreeNodeSchema,
				LockInfoSchema:       infoSchema,
			},
		},
		callbacks: callbacks,
	}
}

func (h *lockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var lockParams types.LockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalLockParams, err)
	}
	if lockParams.Amount == nil {
		return nil, i18n.NewError(ctx, msgs.MsgNoParamAmount, 0)
	}
	if lockParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}
	if lockParams.Amount.Int().Cmp(MAX_TRANSFER_AMOUNT) >= 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParamTotalAmountInRange)
	}
	return &lockParams, nil
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return &prototk.InitTransactionResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}, nil
}

func (h *lockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)
	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputCoins, inputStates, _, remainder, err := buildInputsForExpectedTotal(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, req.StateQueryContext, resolvedSender.Verifier, params.Amount.Int(), false)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}

	var outputCoins []*types.ZetoCoin
	var outputStates []*pb.NewState
	if remainder.Sign() > 0 {
		remainderOutputEntries := []*types.FungibleTransferParamEntry{
			{
				To:     tx.Transaction.From, // remainder outputs are for the sender themselves
				Amount: tktypes.Uint64ToUint256(remainder.Uint64()),
			},
		}
		outputCoins, outputStates, err = prepareOutputsForTransfer(ctx, useNullifiers, remainderOutputEntries, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
		}
	}

	lockedOutputEntries := []*types.FungibleTransferParamEntry{
		{
			To:     tx.Transaction.From, // locked outputs are for the sender themselves
			Amount: params.Amount,
		},
	}
	lockedOutputCoins, lockedOutputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, lockedOutputEntries, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name, true)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}
	outputStates = append(outputStates, lockedOutputStates...)

	// use the info states to record the sizes of the unlocked vs. locked states
	infoStates := h.newInfoStates(lockedOutputStates)

	contractAddress, err := tktypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	allOutputCoins := slices.Concat(outputCoins, lockedOutputCoins)
	circuit := (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER] // use the transfer circuit for locking proofs
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, inputCoins, allOutputCoins, circuit, tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: outputStates,
			InfoStates:   infoStates,
			ReadStates:   inputStates,
		},
		AttestationPlan: []*prototk.AttestationRequest{
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

func (h *lockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *lockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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

	lockedInfo, err := parseInfoStatesJson(req.InfoStates[0].StateDataJson)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseInfoStates, err)
	}
	unlockedOutputStates := req.OutputStates[:len(req.OutputStates)-int(lockedInfo.Size.Uint64())]
	lockedOutputStates := req.OutputStates[len(req.OutputStates)-int(lockedInfo.Size.Uint64()):]

	outputs, err := utxosFromOutputStates(ctx, unlockedOutputStates, inputSize)
	if err != nil {
		return nil, err
	}
	outputs = trimZeroUtxos(outputs)

	lockedOutputs, err := utxosFromOutputStates(ctx, lockedOutputStates, inputSize)
	if err != nil {
		return nil, err
	}
	lockedOutputs = trimZeroUtxos(lockedOutputs)

	data, err := common.EncodeTransactionData(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	params := map[string]any{
		"inputs":        inputs,
		"outputs":       outputs,
		"lockedOutputs": lockedOutputs,
		"proof":         common.EncodeProof(proofRes.Proof),
		"delegate":      tx.Params.(*types.LockParams).Delegate.String(),
		"data":          data,
	}
	transferFunction := getLockABI(tx.DomainConfig.TokenName)
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

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}
func (h *lockHandler) newInfoStates(lockedOutputStates []*pb.NewState) []*pb.NewState {
	size := tktypes.HexUint64(uint64(len(lockedOutputStates)))
	lockedStatesInfoBytes, _ := json.Marshal(&lockedStatesInfo{
		Size: &size,
	})
	randId := (tktypes.RandHex(32))
	return []*pb.NewState{
		{
			SchemaId:      h.stateSchemas.LockInfoSchema.Id,
			Id:            &randId,
			StateDataJson: string(lockedStatesInfoBytes),
		},
	}
}

func getLockABI(tokenName string) *abi.Entry {
	transferFunction := lockABI
	if common.IsNullifiersToken(tokenName) {
		transferFunction = lockABINullifiers
	}
	return transferFunction
}

func parseInfoStatesJson(statesJson string) (*lockedStatesInfo, error) {
	var info lockedStatesInfo
	if err := json.Unmarshal([]byte(statesJson), &info); err != nil {
		return nil, err
	}
	return &info, nil
}
