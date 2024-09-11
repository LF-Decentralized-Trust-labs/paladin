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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type approveHandler struct {
	noto *Noto
}

type ApprovedTransferParams struct {
	Inputs  []interface{}             `json:"inputs"`
	Outputs []interface{}             `json:"outputs"`
	Data    ethtypes.HexBytes0xPrefix `json:"data"`
}

func (h *approveHandler) ValidateParams(ctx context.Context, params string) (interface{}, error) {
	var approveParams types.ApproveParams
	if err := json.Unmarshal([]byte(params), &approveParams); err != nil {
		return nil, err
	}
	_, err := h.decodeTransferCall(context.Background(), approveParams.Call)
	if err != nil {
		return nil, err
	}
	return &approveParams, nil
}

func (h *approveHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    tx.Transaction.From,
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (h *approveHandler) decodeTransferCall(ctx context.Context, encodedCall []byte) (*ApprovedTransferParams, error) {
	approvedTransfer := h.noto.contractABI.Functions()["approvedTransfer"]
	if approvedTransfer == nil {
		return nil, fmt.Errorf("could not find approvedTransfer method")
	}
	paramsJSON, err := decodeParams(ctx, approvedTransfer, encodedCall)
	if err != nil {
		return nil, err
	}
	var params ApprovedTransferParams
	err = json.Unmarshal(paramsJSON, &params)
	return &params, err
}

func (h *approveHandler) encodeTransfer(ctx context.Context, tx *types.ParsedTransaction, params *types.ApproveParams) (ethtypes.HexBytes0xPrefix, error) {
	transferParams, err := h.decodeTransferCall(ctx, params.Call)
	if err != nil {
		return nil, err
	}
	return h.noto.encodeTransferMasked(ctx,
		tx.ContractAddress,
		transferParams.Inputs,
		transferParams.Outputs,
		transferParams.Data)
}

func (h *approveHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.ApproveParams)
	encodedTransfer, err := h.encodeTransfer(ctx, tx, params)
	if err != nil {
		return nil, err
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  []*pb.StateRef{},
			OutputStates: []*pb.NewState{},
		},
		AttestationPlan: []*pb.AttestationRequest{
			// Sender confirms the initial request with a signature
			{
				Name:            "sender",
				AttestationType: pb.AttestationType_SIGN,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Payload:         encodedTransfer,
				Parties:         []string{req.Transaction.From},
			},
			// Notary will endorse the assembled transaction (by submitting to the ledger)
			{
				Name:            "notary",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.DomainConfig.NotaryLookup},
			},
		},
	}, nil
}

func (h *approveHandler) validateSenderSignature(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) error {
	params := tx.Params.(*types.ApproveParams)
	encodedTransfer, err := h.encodeTransfer(ctx, tx, params)
	if err != nil {
		return err
	}
	signature := domain.FindAttestation("sender", req.Signatures)
	if signature == nil {
		return fmt.Errorf("did not find 'sender' attestation")
	}
	signingAddress, err := h.noto.recoverSignature(ctx, encodedTransfer, signature.Payload)
	if err != nil {
		return err
	}
	if signingAddress.String() != signature.Verifier.Verifier {
		return fmt.Errorf("sender signature does not match")
	}
	return nil
}

func (h *approveHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	if err := h.validateSenderSignature(ctx, tx, req); err != nil {
		return nil, err
	}
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func decodeParams(ctx context.Context, abi *abi.Entry, encodedCall []byte) ([]byte, error) {
	callData, err := abi.DecodeCallDataCtx(ctx, encodedCall)
	if err != nil {
		return nil, err
	}
	return tktypes.StandardABISerializer().SerializeJSON(callData)
}

func (h *approveHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.ApproveParams)
	encodedTransfer, err := h.encodeTransfer(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	signature := domain.FindAttestation("sender", req.AttestationResult)
	if signature == nil {
		return nil, fmt.Errorf("did not find 'sender' attestation")
	}

	approveParams := map[string]interface{}{
		"delegate":  params.Delegate,
		"txhash":    encodedTransfer,
		"signature": ethtypes.HexBytes0xPrefix(signature.Payload),
	}
	paramsJSON, err := json.Marshal(approveParams)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(h.noto.contractABI.Functions()["approve"])
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}
