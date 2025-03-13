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

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/smt"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/tktypes"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &withdrawHandler{}

type withdrawHandler struct {
	baseHandler
	callbacks            plugintk.DomainCallbacks
	coinSchema           *pb.StateSchema
	merkleTreeRootSchema *prototk.StateSchema
	merkleTreeNodeSchema *prototk.StateSchema
}

var withdrawABI = &abi.Entry{
	Type: abi.Function,
	Name: "withdraw",
	Inputs: abi.ParameterArray{
		{Name: "amount", Type: "uint256"},
		{Name: "inputs", Type: "uint256[]"},
		{Name: "output", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

var withdrawABI_nullifiers = &abi.Entry{
	Type: abi.Function,
	Name: "withdraw",
	Inputs: abi.ParameterArray{
		{Name: "amount", Type: "uint256"},
		{Name: "nullifiers", Type: "uint256[]"},
		{Name: "output", Type: "uint256"},
		{Name: "root", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

func NewWithdrawHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema, merkleTreeRootSchema, merkleTreeNodeSchema *pb.StateSchema) *withdrawHandler {
	return &withdrawHandler{
		baseHandler: baseHandler{
			name: name,
		},
		callbacks:            callbacks,
		coinSchema:           coinSchema,
		merkleTreeRootSchema: merkleTreeRootSchema,
		merkleTreeNodeSchema: merkleTreeNodeSchema,
	}
}

func (h *withdrawHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var withdrawParams types.WithdrawParams
	if err := json.Unmarshal([]byte(params), &withdrawParams); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeWithdrawCall, err)
	}

	if err := validateAmountParam(ctx, withdrawParams.Amount, 0); err != nil {
		return nil, err
	}

	return withdrawParams.Amount, nil
}
func (h *withdrawHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}
	return res, nil
}

func (h *withdrawHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	amount := tx.Params.(*tktypes.HexUint256)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputCoins, inputStates, _, remainder, err := h.prepareInputs(ctx, useNullifiers, req.StateQueryContext, resolvedSender.Verifier, amount)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}

	outputCoin, outputState, err := h.prepareOutput(ctx, tktypes.MustParseHexUint256(remainder.Text(10)), resolvedSender)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}

	contractAddress, err := tktypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	payloadBytes, err := h.formatProvingRequest(ctx, inputCoins, outputCoin, tx.DomainConfig.CircuitId, tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	amountStr := amount.Int().Text(10)
	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: []*pb.NewState{outputState},
			DomainData:   &amountStr,
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

func (h *withdrawHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *withdrawHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	inputSize := common.GetInputSize(len(req.InputStates))
	inputs := make([]string, inputSize)
	for i := 0; i < inputSize; i++ {
		if i < len(req.InputStates) {
			state := req.InputStates[i]
			coin, err := makeCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorParseInputStates, err)
			}
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
			}
			inputs[i] = hash.String()
		} else {
			inputs[i] = "0"
		}
	}

	outputCoin, err := makeCoin(req.OutputStates[0].StateDataJson)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseOutputStates, err)
	}
	hash, err := outputCoin.Hash(ctx)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
	}
	output := hash.String()

	data, err := common.EncodeTransactionData(ctx, req.Transaction, types.ZetoTransactionData_V0)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	amount := tktypes.MustParseHexUint256(*req.DomainData)
	params := map[string]any{
		"amount": amount.Int().Text(10),
		"inputs": inputs,
		"output": output,
		"proof":  common.EncodeProof(proofRes.Proof),
		"data":   data,
	}
	if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
		delete(params, "inputs")
		params["nullifiers"] = strings.Split(proofRes.PublicInputs["nullifiers"], ",")
		params["root"] = proofRes.PublicInputs["root"]
	}
	withdrawFunction := getWithdrawABI(tx.DomainConfig.TokenName)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(withdrawFunction)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  &req.Transaction.From, // must be signed by the original sender
		},
	}, nil
}

func (h *withdrawHandler) prepareInputs(ctx context.Context, useNullifiers bool, stateQueryContext, senderKey string, amount *tktypes.HexUint256) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, *big.Int, error) {
	expectedTotal := amount.Int()
	return buildInputsForExpectedTotal(ctx, h.callbacks, h.coinSchema, useNullifiers, stateQueryContext, senderKey, expectedTotal)
}

func (h *withdrawHandler) prepareOutput(ctx context.Context, amount *tktypes.HexUint256, resolvedRecipient *pb.ResolvedVerifier) (*types.ZetoCoin, *pb.NewState, error) {
	recipientKey, err := common.LoadBabyJubKey([]byte(resolvedRecipient.Verifier))
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
	}

	salt := crypto.NewSalt()
	compressedKeyStr := zetosigner.EncodeBabyJubJubPublicKey(recipientKey)
	newCoin := &types.ZetoCoin{
		Salt:   (*tktypes.HexUint256)(salt),
		Owner:  tktypes.MustParseHexBytes(compressedKeyStr),
		Amount: amount,
	}

	newState, err := makeNewState(ctx, h.coinSchema, false, newCoin, h.name, resolvedRecipient.Lookup)
	if err != nil {
		return nil, nil, i18n.NewError(ctx, msgs.MsgErrorCreateNewState, err)
	}
	return newCoin, newState, nil
}

func (h *withdrawHandler) formatProvingRequest(ctx context.Context, inputCoins []*types.ZetoCoin, outputCoin *types.ZetoCoin, circuitId, tokenName, stateQueryContext string, contractAddress *tktypes.EthAddress) ([]byte, error) {
	inputSize := common.GetInputSize(len(inputCoins))
	inputCommitments := make([]string, inputSize)
	inputValueInts := make([]uint64, inputSize)
	inputSalts := make([]string, inputSize)
	inputOwner := inputCoins[0].Owner.String()
	for i := 0; i < inputSize; i++ {
		if i < len(inputCoins) {
			coin := inputCoins[i]
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
			}
			inputCommitments[i] = hash.Int().Text(16)
			inputValueInts[i] = coin.Amount.Int().Uint64()
			inputSalts[i] = coin.Salt.Int().Text(16)
		} else {
			inputCommitments[i] = "0"
			inputSalts[i] = "0"
		}
	}

	hash, err := outputCoin.Hash(ctx)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
	}
	outputCommitment := hash.Int().Text(16)
	outputValueInt := outputCoin.Amount.Int().Uint64()
	outputSalt := outputCoin.Salt.Int().Text(16)
	outputOwner := outputCoin.Owner.String()

	var extras []byte
	if common.IsFungibleNullifiersCircuit(circuitId) {
		proofs, extrasObj, err := generateMerkleProofs(ctx, h.callbacks, h.merkleTreeRootSchema, h.merkleTreeNodeSchema, tokenName, stateQueryContext, contractAddress, inputCoins)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateMTP, err)
		}
		for i := len(proofs); i < inputSize; i++ {
			extrasObj.MerkleProofs = append(extrasObj.MerkleProofs, &smt.Empty_Proof)
			extrasObj.Enabled = append(extrasObj.Enabled, false)
		}
		protoExtras, err := proto.Marshal(extrasObj)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalExtraObj, err)
		}
		extras = protoExtras
	}

	tokenSecrets, err := marshalTokenSecrets(inputValueInts, []uint64{outputValueInt})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalValuesFungible, err)
	}

	// marshalValues
	payload := &corepb.ProvingRequest{
		CircuitId: getCircuitId(tokenName),
		Common: &corepb.ProvingRequestCommon{
			InputCommitments:  inputCommitments,
			InputSalts:        inputSalts,
			InputOwner:        inputOwner,
			OutputCommitments: []string{outputCommitment},
			OutputSalts:       []string{outputSalt},
			OutputOwners:      []string{outputOwner},
			TokenSecrets:      tokenSecrets,
			TokenType:         corepb.TokenType_fungible,
		},
	}

	if extras != nil {
		payload.Extras = extras
	}

	return proto.Marshal(payload)
}

func getCircuitId(tokenName string) string {
	isNullifier := common.IsNullifiersToken(tokenName)

	if isNullifier {
		return constants.CIRCUIT_WITHDRAW_NULLIFIER
	} else {
		return constants.CIRCUIT_WITHDRAW
	}
}

func getWithdrawABI(tokenName string) *abi.Entry {
	withdrawFunction := withdrawABI
	if common.IsNullifiersToken(tokenName) {
		withdrawFunction = withdrawABI_nullifiers
	}
	return withdrawFunction
}
