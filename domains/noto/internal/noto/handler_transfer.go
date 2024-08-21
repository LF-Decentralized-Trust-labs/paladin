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
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
)

type transferHandler struct {
	domainHandler
}

func (h *transferHandler) ParseParams(params string) (interface{}, error) {
	var transferParams NotoTransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}
	if transferParams.From == "" {
		return nil, fmt.Errorf("parameter 'from' is required")
	}
	if transferParams.To == "" {
		return nil, fmt.Errorf("parameter 'to' is required")
	}
	if transferParams.Amount.BigInt().Sign() != 1 {
		return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
	}
	return transferParams, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *parsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.params.(NotoTransferParams)
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    tx.domainConfig.Notary,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
			{
				Lookup:    params.From,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
			{
				Lookup:    params.To,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *parsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.params.(NotoTransferParams)
	inputCoins, inputStates, total, err := h.noto.prepareInputs(ctx, params.From, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := h.noto.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.BigInt()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.BigInt())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(params.From, *ethtypes.NewHexInteger(remainder))
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	encodedTransfer, err := h.noto.encodeTransferData(ctx, tx.contractAddress, inputCoins, outputCoins)
	if err != nil {
		return nil, err
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			SpentStates: inputStates,
			NewStates:   outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "sender",
				AttestationType: pb.AttestationType_SIGN,
				Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				Payload:         encodedTransfer,
				Parties: []string{
					req.Transaction.From,
				},
			},
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

func (h *transferHandler) Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	coins, err := h.gatherCoins(req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}

	if coins.inTotal.Cmp(coins.outTotal) != 0 {
		return nil, fmt.Errorf("invalid amount for 'transfer'")
	}

	var senderSignature *pb.AttestationResult
	for _, ar := range req.Signatures {
		if ar.AttestationType == pb.AttestationType_SIGN &&
			ar.Name == "sender" &&
			ar.Verifier.Algorithm == api.Algorithm_ECDSA_SECP256K1_PLAINBYTES {
			senderSignature = ar
			break
		}
	}
	if senderSignature == nil {
		return nil, fmt.Errorf("did not find 'sender' attestation result")
	}

	encodedTransfer, err := h.noto.encodeTransferData(ctx, tx.contractAddress, coins.inCoins, coins.outCoins)
	if err != nil {
		return nil, err
	}
	signingAddress, err := h.noto.recoverSignature(ctx, encodedTransfer, senderSignature.Payload)
	if err != nil {
		return nil, err
	}
	if signingAddress.String() != senderSignature.Verifier.Verifier {
		return nil, fmt.Errorf("sender signature does not match")
	}

	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}
