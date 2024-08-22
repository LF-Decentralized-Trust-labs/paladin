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

func (h *transferHandler) ValidateParams(params string) (interface{}, error) {
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
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    tx.domainConfig.NotaryLookup,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
			// TODO: should we also resolve "From"/"To" parties?
		},
	}, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *parsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.params.(NotoTransferParams)

	notary := findVerifier(tx.domainConfig.NotaryLookup, req.ResolvedVerifiers)
	if notary == nil || notary.Verifier != tx.domainConfig.NotaryAddress {
		// TODO: do we need to verify every time?
		return nil, fmt.Errorf("notary resolved to unexpected address")
	}

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
				Parties:         []string{req.Transaction.From},
			},
			{
				Name:            "notary",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.domainConfig.NotaryLookup},
			},
		},
	}, nil
}

func (h *transferHandler) validateAmounts(coins *gatheredCoins) error {
	if coins.inTotal.Cmp(coins.outTotal) != 0 {
		return fmt.Errorf("invalid amount for 'transfer'")
	}
	return nil
}

func (h *transferHandler) validateSignature(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest, coins *gatheredCoins) error {
	senderSignature := findAttestation("sender", req.Signatures)
	if senderSignature == nil {
		return fmt.Errorf("did not find 'sender' attestation")
	}
	encodedTransfer, err := h.noto.encodeTransferData(ctx, tx.contractAddress, coins.inCoins, coins.outCoins)
	if err != nil {
		return err
	}
	signingAddress, err := h.noto.recoverSignature(ctx, encodedTransfer, senderSignature.Payload)
	if err != nil {
		return err
	}
	if signingAddress.String() != senderSignature.Verifier.Verifier {
		return fmt.Errorf("sender signature does not match")
	}
	return nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *parsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	coins, err := h.gatherCoins(req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}
	if err := h.validateAmounts(coins); err != nil {
		return nil, err
	}
	if err := h.validateSignature(ctx, tx, req, coins); err != nil {
		return nil, err
	}
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *transferHandler) Prepare(ctx context.Context, tx *parsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	inputs := make([]string, len(req.FinalizedTransaction.SpentStates))
	for i, state := range req.FinalizedTransaction.SpentStates {
		inputs[i] = state.HashId
	}
	outputs := make([]string, len(req.FinalizedTransaction.NewStates))
	for i, state := range req.FinalizedTransaction.NewStates {
		outputs[i] = state.HashId
	}

	var senderSignature ethtypes.HexBytes0xPrefix
	for _, att := range req.AttestationResult {
		if att.AttestationType == pb.AttestationType_SIGN && att.Name == "sender" {
			senderSignature = att.Payload
		}
	}

	params := map[string]interface{}{
		"inputs":    inputs,
		"outputs":   outputs,
		"signature": senderSignature,
		"data":      req.Transaction.TransactionId,
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
