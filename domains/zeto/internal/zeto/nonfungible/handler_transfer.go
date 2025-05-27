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

package nonfungible

import (
	"context"
	"encoding/json"
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
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/protobuf/proto"
)

var _ types.DomainHandler = &transferHandler{}

type transferHandler struct {
	baseHandler
	callbacks            plugintk.DomainCallbacks
	nftSchema            *pb.StateSchema
	merkleTreeRootSchema *prototk.StateSchema
	merkleTreeNodeSchema *prototk.StateSchema
}

var transferABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_TRANSFER,
	Inputs: abi.ParameterArray{
		{Name: "input", Type: "uint256"},
		{Name: "output", Type: "uint256"},
		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
		{Name: "data", Type: "bytes"},
	},
}

// var transferABI_nullifiers = &abi.Entry{
// 	Type: abi.Function,
// 	Name: "transfer",
// 	Inputs: abi.ParameterArray{
// 		{Name: "nullifier", Type: "uint256"},
// 		{Name: "output", Type: "uint256"},
// 		{Name: "root", Type: "uint256"},
// 		{Name: "proof", Type: "tuple", InternalType: "struct Commonlib.Proof", Components: common.ProofComponents},
// 		{Name: "data", Type: "bytes"},
// 	},
// }

func NewTransferHandler(name string, callbacks plugintk.DomainCallbacks, nftSchema, merkleTreeRootSchema, merkleTreeNodeSchema *pb.StateSchema) *transferHandler {
	return &transferHandler{
		baseHandler: baseHandler{
			name: name,
		},
		callbacks:            callbacks,
		nftSchema:            nftSchema,
		merkleTreeRootSchema: merkleTreeRootSchema,
		merkleTreeNodeSchema: merkleTreeNodeSchema,
	}
}

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var transferParams types.NonFungibleTransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}

	if err := validateTransferParams(ctx, transferParams.Transfers); err != nil {
		return nil, err
	}

	return transferParams.Transfers, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.([]*types.NonFungibleTransferParamEntry)

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
	params := tx.Params.([]*types.NonFungibleTransferParamEntry)

	resolvedSender := findVerifierFunc(tx.Transaction.From, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, tx.Transaction.From)
	}

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	inputTokens, inputStates, err := prepareInputsForTransfer(ctx, h.callbacks, h.nftSchema, useNullifiers, req.StateQueryContext, resolvedSender.Verifier, params)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxInputs, err)
	}
	outputTokens, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params, req.ResolvedVerifiers, h.nftSchema, h.name)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPrepTxOutputs, err)
	}

	contractAddress, err := pldtypes.ParseEthAddress(req.Transaction.ContractInfo.ContractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeContractAddress, err)
	}
	payloadBytes, err := h.formatProvingRequest(ctx, inputTokens, outputTokens, (*tx.DomainConfig.Circuits)[types.METHOD_TRANSFER], tx.DomainConfig.TokenName, req.StateQueryContext, contractAddress)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFormatProvingReq, err)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: outputStates,
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
	result := findAttestationFunc("sender", req.AttestationResult)
	if result == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorFindSenderAttestation)
	}
	if err := proto.Unmarshal(result.Payload, &proofRes); err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorUnmarshalProvingRes, err)
	}

	if len(req.InputStates) != 1 {
	}

	// set input
	input, err := prepareState(ctx, req.InputStates[0])
	if err != nil {
		return nil, err
	}

	// set output
	output, err := prepareState(ctx, req.OutputStates[0])
	if err != nil {
		return nil, err
	}

	data, err := encodeTransactionDataFunc(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	params := map[string]any{
		"input":  input,
		"output": output,
		"proof":  encodeProofFunc(proofRes.Proof),
		"data":   data,
	}
	transferFunction := getTransferABI(tx.DomainConfig.TokenName)

	if common.IsNullifiersToken(tx.DomainConfig.TokenName) {
		delete(params, "input")
		params["nullifier"] = strings.Split(proofRes.PublicInputs["nullifier"], ",")
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

func prepareState(ctx context.Context, state *pb.EndorsableState) (string, error) {
	token, err := makeNFToken(state.StateDataJson)
	if err != nil {
		return "", i18n.NewError(ctx, msgs.MsgErrorParseInputStates, err)
	}
	hash, err := token.Hash(ctx)
	if err != nil {
		return "", i18n.NewError(ctx, msgs.MsgErrorHashInputState, err)
	}
	return hash.String(), nil

}

func (h *transferHandler) formatProvingRequest(ctx context.Context, input, output []*types.ZetoNFToken, circuit *zetosignerapi.Circuit, tokenName, stateQueryContext string, contractAddress *pldtypes.EthAddress) ([]byte, error) {

	inputCommitments, inputSalts, tokenURIs, tokenIDs, inputOwners, err := processTokens(ctx, input)
	if err != nil {
		return nil, err
	}

	outputCommitments, outputSalts, _, _, outputOwners, err := processTokens(ctx, output)
	if err != nil {
		return nil, err
	}

	/* TODO: Add support for nullifiers
	if common.IsNullifiersCircuit(circuitId) {
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
	*/

	tokenSecrets, err := json.Marshal(corepb.TokenSecrets_NonFungible{TokenIds: tokenIDs, TokenUris: tokenURIs})
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalValuesNonFungible, err)
	}

	payload := &corepb.ProvingRequest{
		Circuit: circuit.ToProto(),
		Common: &corepb.ProvingRequestCommon{
			InputCommitments:  inputCommitments,
			InputSalts:        inputSalts,
			InputOwner:        inputOwners[0], // there is only one owner for all inputs
			OutputSalts:       outputSalts,
			OutputOwners:      outputOwners,
			OutputCommitments: outputCommitments,
			TokenType:         corepb.TokenType_nunFungible,
			TokenSecrets:      tokenSecrets,
		},
	}

	return proto.Marshal(payload)
}

func getTransferABI(tokenName string) *abi.Entry {
	return transferABI
}

func validateTransferParams(ctx context.Context, params []*types.NonFungibleTransferParamEntry) error {
	if len(params) == 0 {
		return i18n.NewError(ctx, msgs.MsgNoTransferParams)
	}
	for i, param := range params {
		if param.To == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamTo, i)
		}
		if param.TokenID.NilOrZero() {
			return i18n.NewError(ctx, msgs.MsgNoParamTokenID, i)
		}
	}
	return nil
}

func prepareInputsForTransfer(
	ctx context.Context,
	callbacks plugintk.DomainCallbacks,
	stateSchema *pb.StateSchema,
	useNullifiers bool,
	stateQueryContext, senderKey string,
	params []*types.NonFungibleTransferParamEntry,
) ([]*types.ZetoNFToken, []*pb.StateRef, error) {

	var tokens []*types.ZetoNFToken
	var stateRefs []*pb.StateRef

	// Process each transfer parameter
	for _, transferParam := range params {
		// Build a query to fetch the single token matching this ID.
		qb := query.NewQueryBuilder().
			Limit(1).
			Equal("owner", senderKey).
			Equal("tokenID", transferParam.TokenID.String())
		queryStr := qb.Query().String()

		// Retrieve available states for the given query.
		states, err := findAvailableStates(ctx, callbacks, stateSchema, useNullifiers, stateQueryContext, queryStr)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorQueryAvailCoins, err)
		}
		if len(states) != 1 {
			return nil, nil, i18n.NewError(ctx, msgs.MsgInsufficientFunds)
		}
		foundState := states[0]

		// Unmarshal the state data into a non-fungible token.
		token, err := makeNFToken(foundState.DataJson)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgInvalidCoin, foundState.Id, err)
		}

		// Create a reference to the found state.
		stateRef := &pb.StateRef{
			SchemaId: foundState.SchemaId,
			Id:       foundState.Id,
		}

		// Append the token and state reference to the result slices.
		tokens = append(tokens, token)
		stateRefs = append(stateRefs, stateRef)

		// Update each transfer parameter that matches this token's TokenID with the token's URI.
		for _, p := range params {
			if p.TokenID.String() == token.TokenID.String() {
				p.URI = token.URI
			}
		}
	}
	return tokens, stateRefs, nil
}
