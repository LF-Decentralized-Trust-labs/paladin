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
	katapb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/protobuf/proto"
)

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
	if transferParams.SenderKey == "" {
		return nil, fmt.Errorf("parameter 'senderKey' is required")
	}
	if transferParams.RecipientKey == "" {
		return nil, fmt.Errorf("parameter 'recipientKey' is required")
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
				Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
			},
			{
				Lookup:    params.RecipientKey,
				Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
			},
		},
	}, nil
}

func (h *transferHandler) loadBabyJubKey(payload []byte) (*babyjub.PublicKey, error) {
	var keyCompressed babyjub.PublicKeyComp
	if err := keyCompressed.UnmarshalText(payload); err != nil {
		return nil, err
	}
	return keyCompressed.Decompress()
}

func (h *transferHandler) formatProvingRequest(inputCoins, outputCoins []*ZetoCoin) ([]byte, error) {
	inputCommitments := make([]string, INPUT_COUNT)
	inputValueInts := make([]uint64, INPUT_COUNT)
	inputSalts := make([]string, INPUT_COUNT)
	inputOwner := inputCoins[0].OwnerKey.String()
	for i := 0; i < INPUT_COUNT; i++ {
		if i < len(inputCoins) {
			coin := inputCoins[i]
			inputCommitments[i] = coin.Hash.BigInt().Text(16)
			inputValueInts[i] = coin.Amount.Uint64()
			inputSalts[i] = coin.Salt.BigInt().Text(16)
		} else {
			inputCommitments[i] = "0"
			inputSalts[i] = "0"
		}
	}

	outputValueInts := make([]uint64, OUTPUT_COUNT)
	outputSalts := make([]string, OUTPUT_COUNT)
	outputOwners := make([]string, OUTPUT_COUNT)
	for i := 0; i < OUTPUT_COUNT; i++ {
		if i < len(outputCoins) {
			coin := outputCoins[i]
			outputValueInts[i] = coin.Amount.Uint64()
			outputSalts[i] = coin.Salt.BigInt().Text(16)
			outputOwners[i] = coin.OwnerKey.String()
		} else {
			outputSalts[i] = "0"
		}
	}

	payload := &katapb.ProvingRequest{
		CircuitId: "anon",
		Common: &katapb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       inputOwner,
			OutputValues:     outputValueInts,
			OutputSalts:      outputSalts,
			OutputOwners:     outputOwners,
		},
	}
	return proto.Marshal(payload)
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

	senderKey, err := h.loadBabyJubKey([]byte(resolvedSender.Verifier))
	if err != nil {
		return nil, err
	}
	recipientKey, err := h.loadBabyJubKey([]byte(resolvedRecipient.Verifier))
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

	payloadBytes, err := h.formatProvingRequest(inputCoins, outputCoins)
	if err != nil {
		return nil, err
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
				Algorithm:       algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
				Payload:         payloadBytes,
				Parties:         []string{params.SenderKey},
			},
			{
				Name:            "submitter",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.transaction.From},
			},
		},
	}, nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *transferHandler) encodeProof(proof *katapb.SnarkProof) map[string]interface{} {
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
	var proof katapb.SnarkProof
	result := findAttestation("sender", req.AttestationResult)
	if result == nil {
		return nil, fmt.Errorf("did not find 'sender' attestation")
	}
	if err := proto.Unmarshal(result.Payload, &proof); err != nil {
		return nil, err
	}

	inputs := make([]string, INPUT_COUNT)
	for i := 0; i < INPUT_COUNT; i++ {
		if i < len(req.InputStates) {
			state := req.InputStates[i]
			coin, err := h.zeto.makeCoin(state.StateDataJson)
			if err != nil {
				return nil, err
			}
			inputs[i] = coin.Hash.String()
		} else {
			inputs[i] = "0"
		}
	}
	outputs := make([]string, OUTPUT_COUNT)
	for i := 0; i < OUTPUT_COUNT; i++ {
		if i < len(req.OutputStates) {
			state := req.OutputStates[i]
			coin, err := h.zeto.makeCoin(state.StateDataJson)
			if err != nil {
				return nil, err
			}
			outputs[i] = coin.Hash.String()
		} else {
			outputs[i] = "0"
		}
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
