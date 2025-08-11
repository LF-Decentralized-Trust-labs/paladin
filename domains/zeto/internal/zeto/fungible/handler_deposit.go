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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &depositHandler{}

type depositHandler struct {
	baseHandler
}

func NewDepositHandler(name string, coinSchema *pb.StateSchema) *depositHandler {
	return &depositHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema: coinSchema,
			},
		},
	}
}

var depositABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_DEPOSIT,
	Inputs: abi.ParameterArray{
		{Name: "amount", Type: "uint256"},
		{Name: "outputs", Type: "uint256[]"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

func (h *depositHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var depositParams types.DepositParams
	if err := json.Unmarshal([]byte(params), &depositParams); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeDepositCall, err)
	}

	if err := validateAmountParam(ctx, depositParams.Amount, 0); err != nil {
		return nil, err
	}

	return depositParams.Amount, nil
}

func (h *depositHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
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

func (h *depositHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	amount := tx.Params.(*pldtypes.HexUint256)

	resolvedSender := domain.FindVerifier(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	outputCoins, outputStates, err := h.prepareOutputs(ctx, useNullifiers, amount, resolvedSender)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}

	payloadBytes, err := h.formatProvingRequest(ctx, outputCoins, (*tx.DomainConfig.Circuits)[types.METHOD_DEPOSIT])
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	amountStr := amount.Int().Text(10)
	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			OutputStates: outputStates,
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

func (h *depositHandler) prepareOutputs(ctx context.Context, useNullifiers bool, amount *pldtypes.HexUint256, resolvedSender *pb.ResolvedVerifier) ([]*types.ZetoCoin, []*pb.NewState, error) {
	var coins []*types.ZetoCoin
	// the token implementation allows up to 2 output states, we will use one of them
	// to bear the deposit amount, and set the other to value of 0. we randomize
	// which one to use and which one to set to 0
	var newStates []*pb.NewState
	amounts := make([]*pldtypes.HexUint256, 2)
	size := 2
	randomIdx := randomSlot(size)
	amounts[randomIdx] = amount
	amounts[size-randomIdx-1] = pldtypes.MustParseHexUint256("0x0")
	for _, amt := range amounts {
		resolvedRecipient := resolvedSender
		recipientKey, err := common.LoadBabyJubKey([]byte(resolvedRecipient.Verifier))
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}

		salt := crypto.NewSalt()
		compressedKeyStr := zetosigner.EncodeBabyJubJubPublicKey(recipientKey)
		newCoin := &types.ZetoCoin{
			Salt:   (*pldtypes.HexUint256)(salt),
			Owner:  pldtypes.MustParseHexBytes(compressedKeyStr),
			Amount: amt,
		}

		newState, err := makeNewState(ctx, h.stateSchemas.CoinSchema, useNullifiers, newCoin, h.name, resolvedRecipient.Lookup)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorCreateNewState, err)
		}
		coins = append(coins, newCoin)
		newStates = append(newStates, newState)
	}
	return coins, newStates, nil
}

func (h *depositHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *depositHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proofRes corepb.ProvingResponse
	result := domain.FindAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	outputSize := common.GetInputSize(len(req.OutputStates))
	outputs := make([]string, outputSize)
	for i := 0; i < outputSize; i++ {
		if i < len(req.OutputStates) {
			state := req.OutputStates[i]
			coin, err := makeCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorParseOutputStates, err)
			}
			hash, err := coin.Hash(ctx)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgErrorHashOutputState, err)
			}
			outputs[i] = hash.String()
		} else {
			outputs[i] = "0"
		}
	}

	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	amount := pldtypes.MustParseHexUint256(*req.DomainData)
	params := map[string]any{
		"amount":  amount.Int().Text(10),
		"outputs": outputs,
		"proof":   common.EncodeProof(proofRes.Proof),
		"data":    data,
	}
	depositFunction := depositABI
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalPrepedParams, err)
	}
	functionJSON, err := json.Marshal(depositFunction)
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

func (h *depositHandler) formatProvingRequest(ctx context.Context, outputCoins []*types.ZetoCoin, circuit *zetosignerapi.Circuit) ([]byte, error) {
	outputSize := common.GetInputSize(len(outputCoins))
	outputCommitments := make([]string, outputSize)
	outputValueInts := make([]uint64, outputSize)
	outputSalts := make([]string, outputSize)
	outputOwners := make([]string, outputSize)
	for i := 0; i < outputSize; i++ {
		coin := outputCoins[i]
		hash, err := coin.Hash(ctx)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
		}
		outputCommitments[i] = hash.Int().Text(16)
		outputValueInts[i] = coin.Amount.Int().Uint64()
		outputSalts[i] = coin.Salt.Int().Text(16)
		outputOwners[i] = coin.Owner.String()
	}

	tokenSecrets, err := marshalTokenSecrets([]uint64{}, outputValueInts)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalValuesFungible, err)
	}

	payload := &corepb.ProvingRequest{
		Circuit: circuit.ToProto(),
		Common: &corepb.ProvingRequestCommon{
			OutputCommitments: outputCommitments,
			OutputSalts:       outputSalts,
			OutputOwners:      outputOwners,
			TokenSecrets:      tokenSecrets,
			TokenType:         corepb.TokenType_fungible,
		},
	}
	return proto.Marshal(payload)
}
