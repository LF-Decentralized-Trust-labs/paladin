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
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/hyperledger/firefly-signer/pkg/abi"
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
}

func NewMintHandler(name string, coinSchema, dataSchema *pb.StateSchema) *mintHandler {
	return &mintHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema: coinSchema,
				DataSchema: dataSchema,
			},
		},
	}
}

func (h *mintHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var mintParams types.FungibleMintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}

	if err := validateTransferParams(ctx, mintParams.Mints); err != nil {
		return nil, err
	}

	return mintParams.Mints, nil
}

func (h *mintHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.([]*types.FungibleTransferParamEntry)

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
	params := tx.Params.([]*types.FungibleTransferParamEntry)

	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	_, outputStates, err := prepareOutputsForTransfer(ctx, useNullifiers, params, req.ResolvedVerifiers, h.stateSchemas.CoinSchema, h.name)
	if err != nil {
		return nil, err
	}

	infoStates := make([]*pb.NewState, 0, len(params))
	for _, param := range params {
		info, err := prepareTransactionInfoStates(ctx, param.Data, []string{tx.Transaction.From, param.To}, h.stateSchemas.DataSchema)
		if err != nil {
			return nil, err
		}
		infoStates = append(infoStates, info...)
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			OutputStates: outputStates,
			InfoStates:   infoStates,
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
		coin, err := makeCoin(state.StateDataJson)
		if err != nil {
			return nil, err
		}
		hash, err := coin.Hash(ctx)
		if err != nil {
			return nil, err
		}
		outputs[i] = hash.String()
	}

	data, err := common.EncodeTransactionData(ctx, req.Transaction, req.InfoStates)
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
