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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &transferHandler{}

type transferHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

var transferABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER,
	Inputs: abi.ParameterArray{
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var transferABINullifiers = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER,
	Inputs: abi.ParameterArray{
		{Name: "nullifiers", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var transferABI_withEncryption = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER,
	Inputs: abi.ParameterArray{
		{Name: "inputs", Type: "uint256[]"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "encryptionNonce", Type: "uint256"},
		{Name: "ecdhPublicKey", Type: "uint256[2]"},
		{Name: "encryptedValues", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

func NewTransferHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema, dataSchema *pb.StateSchema) *transferHandler {
	return &transferHandler{
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

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var transferParams types.FungibleTransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}

	if err := validateTransferParams(ctx, transferParams.Transfers); err != nil {
		return nil, err
	}

	return transferParams.Transfers, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.([]*types.FungibleTransferParamEntry)

	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	for _, param := range params {
		res.RequiredVerifiers = append(res.RequiredVerifiers, &pb.ResolveVerifierRequest{
			Lookup:       param.To,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		})
	}

	return res, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.([]*types.FungibleTransferParamEntry)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputStates, expectedTotal, revert, err := prepareInputsForTransfer(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, req.StateQueryContext, resolvedSender.Verifier, params)
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
	outputCoins, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}

	remainder := big.NewInt(0).Sub(inputStates.total, expectedTotal)
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

	infoStates := make([]*pb.NewState, 0, len(params))
	for _, param := range params {
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
	payloadBytes, err := formatTransferProvingRequest(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, inputStates.coins, outputCoins, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress)
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

func (h *transferHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
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
		"outputs": outputs,
		"proof":   common.EncodeProof(proofRes.Proof),
		"data":    data,
	}
	transferFunction := getTransferABI(tx.DomainConfig.TokenName)
	if common.IsEncryptionToken(tx.DomainConfig.TokenName) {
		params["ecdhPublicKey"] = strings.Split(proofRes.PublicInputs["ecdhPublicKey"], ",")
		params["encryptionNonce"] = proofRes.PublicInputs["encryptionNonce"]
		params["encryptedValues"] = strings.Split(proofRes.PublicInputs["encryptedValues"], ",")
	}
	if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
		params["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
		params["root"] = proofRes.PublicInputs["root"]
	} else {
		params["inputs"] = inputs
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(transferFunction)
	if err != nil {
		return nil, err
	}

	var signer *string
	if req.Transaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION {
		// All "prepare" transactions must have an explicit "from" signer
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

func getTransferABI(tokenName string) *abi.Entry {
	transferFunction := transferABI
	if common.IsEncryptionToken(tokenName) {
		transferFunction = transferABI_withEncryption
	} else if common.IsNullifiersToken(tokenName) {
		transferFunction = transferABINullifiers
	}
	return transferFunction
}
