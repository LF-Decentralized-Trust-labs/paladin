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
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"google.golang.org/protobuf/proto"
)

// TODO: what is the best way to pass the hashes from Assemble to Prepare?
var inputs []string
var outputs []string

type transferHandler struct {
	domainHandler
}

func (h *transferHandler) ValidateParams(params string) (interface{}, error) {
	var transferParams ZetoTransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
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
	params := tx.params.(ZetoTransferParams)

	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    params.SenderKey,
				Algorithm: api.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
			},
			{
				Lookup:    params.RecipientKey,
				Algorithm: api.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
			},
		},
	}, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *parsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.params.(ZetoTransferParams)

	resolvedSender := findVerifier(params.SenderKey, req.ResolvedVerifiers)
	if resolvedSender == nil {
		return nil, fmt.Errorf("failed to resolve: %s", params.SenderKey)
	}
	resolvedRecipient := findVerifier(params.RecipientKey, req.ResolvedVerifiers)
	if resolvedRecipient == nil {
		return nil, fmt.Errorf("failed to resolve: %s", params.RecipientKey)
	}

	var senderKeyCompressed babyjub.PublicKeyComp
	if err := senderKeyCompressed.UnmarshalText([]byte(resolvedSender.Verifier)); err != nil {
		return nil, err
	}
	senderKey, err := senderKeyCompressed.Decompress()
	if err != nil {
		return nil, err
	}

	var recipientKeyCompressed babyjub.PublicKeyComp
	if err = recipientKeyCompressed.UnmarshalText([]byte(resolvedRecipient.Verifier)); err != nil {
		return nil, err
	}
	recipientKey, err := recipientKeyCompressed.Decompress()
	if err != nil {
		return nil, err
	}

	inputCoins, inputStates, total, err := h.zeto.prepareInputs(ctx, tx.transaction.From, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := h.zeto.prepareOutputs(params.To, recipientKey, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.BigInt()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.BigInt())
		returnedCoins, returnedStates, err := h.zeto.prepareOutputs(tx.transaction.From, senderKey, ethtypes.NewHexInteger(remainder))
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	inputs = make([]string, len(inputCoins))
	inputCommitments := make([]string, len(inputCoins))
	inputValueInts := make([]uint64, len(inputCoins))
	inputSalts := make([]string, len(inputCoins))
	for i, coin := range inputCoins {
		inputCommitments[i] = coin.Hash.BigInt().Text(16)
		inputs[i] = coin.Hash.String()
		inputValueInts[i] = coin.Amount.Uint64()
		inputSalts[i] = coin.Salt.BigInt().Text(16)
	}

	outputs = make([]string, len(outputCoins))
	outputCommitments := make([]string, len(outputCoins))
	outputValueInts := make([]uint64, len(outputCoins))
	outputSalts := make([]string, len(outputCoins))
	outputOwners := make([]string, len(outputCoins))
	for i, coin := range outputCoins {
		outputCommitments[i] = coin.Hash.BigInt().Text(16)
		outputs[i] = coin.Hash.String()
		outputValueInts[i] = coin.Amount.Uint64()
		outputSalts[i] = coin.Salt.BigInt().Text(16)
		outputOwners[i] = coin.OwnerKey.String()
	}

	payload := &pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       senderKey.String(),
			OutputValues:     outputValueInts,
			OutputSalts:      outputSalts,
			OutputOwners:     outputOwners,
		},
	}
	payloadBytes, err := proto.Marshal(payload)
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
				Algorithm:       api.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
				Payload:         payloadBytes,
				Parties:         []string{params.SenderKey},
			},
		},
	}, nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *transferHandler) encodeProof(proof *pb.SnarkProof) map[string]interface{} {
	// Convert the proof json to the format that the Solidity verifier expects
	return map[string]interface{}{
		"pA": []string{proof.A[0], proof.A[1]},
		"pB": [][]string{
			{proof.B[0].Items[1], proof.B[0].Items[0]},
			{proof.B[1].Items[1], proof.B[1].Items[0]},
		},
		"pC": []string{proof.C[0], proof.C[1]},
	}
}

func (h *transferHandler) Prepare(ctx context.Context, tx *parsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	var proof pb.SnarkProof
	err := proto.Unmarshal(req.AttestationResult[0].Payload, &proof)
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"inputs":  inputs,
		"outputs": outputs,
		"proof":   h.encodeProof(&proof),
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "transfer",
			ParamsJson:   string(paramsJSON),
		},
	}, nil
}
