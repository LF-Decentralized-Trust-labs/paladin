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

package noto

import (
	"context"
	"encoding/json"
	"fmt"

	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
)

type mintHandler struct {
	domainHandler
}

func (h *mintHandler) ParseParams(params string) (interface{}, error) {
	var mintParams NotoMintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}
	if mintParams.To == "" {
		return nil, fmt.Errorf("parameter 'to' is required")
	}
	if mintParams.Amount.BigInt().Sign() != 1 {
		return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
	}
	return mintParams, nil
}

func (h *mintHandler) Init(ctx context.Context, tx *parsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.params.(NotoMintParams)
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    tx.domainConfig.Notary,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
			{
				Lookup:    params.To,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (h *mintHandler) Assemble(ctx context.Context, tx *parsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.params.(NotoMintParams)
	_, outputStates, err := h.noto.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			NewStates: outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "notary",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				Parties: []string{
					"notary", // TODO: why can't we pass notary address here?
				},
			},
		},
	}, nil
}

func (h *mintHandler) Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	params := tx.params.(NotoMintParams)
	coins, err := h.gatherCoins(req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}

	if len(coins.inCoins) > 0 {
		return nil, fmt.Errorf("invalid inputs to 'mint': %v", coins.inCoins)
	}
	if coins.outTotal.Cmp(params.Amount.BigInt()) != 0 {
		return nil, fmt.Errorf("invalid amount for 'mint'")
	}

	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}
