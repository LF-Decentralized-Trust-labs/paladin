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
	"encoding/json"
	"fmt"

	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type mintHandler struct {
	zeto *Zeto
}

func (h *mintHandler) ValidateParams(ctx context.Context, params string) (interface{}, error) {
	var mintParams types.MintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}
	if mintParams.To == "" {
		return nil, fmt.Errorf("parameter 'to' is required")
	}
	if mintParams.Amount.BigInt().Sign() != 1 {
		return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
	}
	return &mintParams, nil
}

func (h *mintHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)

	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    params.To,
				Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
			},
		},
	}, nil
}

func (h *mintHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)

	resolvedRecipient := domain.FindVerifier(params.To, req.ResolvedVerifiers)
	if resolvedRecipient == nil {
		return nil, fmt.Errorf("failed to resolve: %s", params.To)
	}

	var recipientKeyCompressed babyjub.PublicKeyComp
	if err := recipientKeyCompressed.UnmarshalText([]byte(resolvedRecipient.Verifier)); err != nil {
		return nil, err
	}
	recipientKey, err := recipientKeyCompressed.Decompress()
	if err != nil {
		return nil, err
	}

	_, outputStates, err := h.zeto.prepareOutputs(params.To, recipientKey, params.Amount)
	if err != nil {
		return nil, err
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			OutputStates: outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "submitter",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.Transaction.From},
			},
		},
	}, nil
}

func (h *mintHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *mintHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	outputs := make([]string, len(req.OutputStates))
	for i, state := range req.OutputStates {
		coin, err := h.zeto.makeCoin(state.StateDataJson)
		if err != nil {
			return nil, err
		}
		outputs[i] = coin.Hash.String()
	}

	params := map[string]interface{}{
		"utxos": outputs,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	zetoJSON, err := json.Marshal(h.zeto.contractABI)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			ContractAbiJson: string(zetoJSON),
			FunctionName:    "mint",
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}
