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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

var mintABI = &abi.Entry{
	Type: abi.Function,
	Name: types.METHOD_MINT,
	Inputs: abi.ParameterArray{
		{Name: "utxos", Type: "uint256[]"},
		{Name: "data", Type: "bytes"},
	},
}

var _ types.DomainHandler = &mintHandler{}

type mintHandler struct {
	baseHandler
	stateSchema *pb.StateSchema
}

func NewMintHandler(name string, stateSchema *pb.StateSchema) *mintHandler {
	return &mintHandler{
		baseHandler: baseHandler{
			name: name,
		},
		stateSchema: stateSchema,
	}
}

func (h *mintHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var mintParams types.NonFungibleMintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}

	if err := validateMintParams(ctx, mintParams.Mints); err != nil {
		return nil, err
	}

	return mintParams.Mints, nil
}

func validateMintParams(ctx context.Context, params []*types.NonFungibleTransferParamEntry) error {
	if len(params) == 0 {
		return i18n.NewError(ctx, msgs.MsgNoTransferParams)
	}
	for i, param := range params {
		if param.To == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamTo, i)
		}
		if !param.TokenID.NilOrZero() { // token should be empty
			return i18n.NewError(ctx, msgs.MsgNoParamTokenID, i)
		}
		if param.URI == "" {
			return i18n.NewError(ctx, msgs.MsgNoParamURI, i)
		}
	}
	return nil
}
func (h *mintHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.([]*types.NonFungibleTransferParamEntry)

	res := &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{},
	}
	for _, param := range params {
		verifier := &pb.ResolveVerifierRequest{
			Lookup:       param.To,
			Algorithm:    h.getAlgoZetoSnarkBJJ(),
			VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		}
		res.RequiredVerifiers = append(res.RequiredVerifiers, verifier)
	}

	return res, nil
}

func (h *mintHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.([]*types.NonFungibleTransferParamEntry)

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	_, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params, req.ResolvedVerifiers, h.stateSchema, h.name)
	if err != nil {
		return nil, err
	}
	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			OutputStates: outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{},
	}, nil
}

func (h *mintHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return nil, nil
}

func (h *mintHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {

	outputs := make([]string, len(req.OutputStates))
	for i, state := range req.OutputStates {
		token, err := makeNFToken(state.StateDataJson)
		if err != nil {
			return nil, err
		}
		hash, err := token.Hash(ctx)
		if err != nil {
			return nil, err
		}
		outputs[i] = hash.String()
	}

	data, err := encodeTransactionDataFunc(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorEncodeTxData, err)
	}
	params := map[string]interface{}{
		"utxos": outputs,
		"data":  data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(mintABI)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
			RequiredSigner:  &req.Transaction.From, // must be signed by the authorized minter on-chain
		},
	}, nil
}
