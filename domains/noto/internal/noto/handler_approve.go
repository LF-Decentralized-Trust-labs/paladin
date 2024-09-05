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
	_, err := h.decodeTransfer(context.Background(), approveParams.Call)
	if err != nil {
		return nil, err
	}
	return &approveParams, nil
}

func (h *approveHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{},
	}, nil
}

func (h *approveHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			InputStates:  []*pb.StateRef{},
			OutputStates: []*pb.NewState{},
		},
		AttestationPlan: []*pb.AttestationRequest{
			{
				Name:            "notary",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.DomainConfig.NotaryLookup},
			},
		},
	}, nil
}

func (h *approveHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
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

func (h *approveHandler) decodeTransfer(ctx context.Context, encodedCall []byte) (*ApprovedTransferParams, error) {
	approvedTransfer := h.noto.contract.ABI.Functions()["approvedTransfer"]
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

func (h *approveHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.ApproveParams)
	transferParams, err := h.decodeTransfer(ctx, params.Call)
	if err != nil {
		return nil, err
	}
	txhash, err := h.noto.encodeTransferMasked(ctx,
		tx.ContractAddress,
		transferParams.Inputs,
		transferParams.Outputs,
		transferParams.Data)
	if err != nil {
		return nil, err
	}

	approveParams := map[string]interface{}{
		"delegate":  params.Delegate,
		"txhash":    txhash,
		"signature": "0x",
	}
	paramsJSON, err := json.Marshal(approveParams)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "approve",
			ParamsJson:   string(paramsJSON),
		},
	}, nil
}
