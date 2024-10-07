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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type lockHandler struct {
	zeto *Zeto
}

type TransferParams struct {
	Inputs  []interface{}             `json:"inputs"`
	Outputs []interface{}             `json:"outputs"`
	Proof   map[string]any            `json:"proof"`
	Data    ethtypes.HexBytes0xPrefix `json:"data"`
}

func (h *lockHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, params string) (interface{}, error) {
	var lockParams types.LockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lockProof parameters. %s", err)
	}
	// the lockProof() function expects an encoded call to the transfer() function
	_, err := h.decodeTransferCall(ctx, config, lockParams.Call)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the transfer call. %s", err)
	}
	return &lockParams, nil
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return &prototk.InitTransactionResponse{}, nil
}

func (h *lockHandler) decodeTransferCall(ctx context.Context, config *types.DomainInstanceConfig, encodedCall []byte) (*TransferParams, error) {
	contractAbi, err := h.zeto.config.GetContractAbi(config.TokenName)
	if err != nil {
		return nil, err
	}
	transfer := contractAbi.Functions()["transfer"]
	if transfer == nil {
		return nil, fmt.Errorf("unknown function: transfer")
	}
	paramsJSON, err := decodeParams(ctx, transfer, encodedCall)
	if err != nil {
		return nil, err
	}
	var params TransferParams
	err = json.Unmarshal(paramsJSON, &params)
	return &params, err
}

func (h *lockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  []*prototk.StateRef{},
			OutputStates: []*prototk.NewState{},
		},
		AttestationPlan: []*prototk.AttestationRequest{
			{
				Name:            "submitter",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{tx.Transaction.From},
			},
		},
	}, nil
}

func (h *lockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func decodeParams(ctx context.Context, abi *abi.Entry, encodedCall []byte) ([]byte, error) {
	callData, err := abi.DecodeCallDataCtx(ctx, encodedCall)
	if err != nil {
		return nil, err
	}
	return tktypes.StandardABISerializer().SerializeJSON(callData)
}

func (h *lockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)
	decodedTransfer, err := h.decodeTransferCall(context.Background(), tx.DomainConfig, params.Call)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transfer call data. %s", err)
	}

	data, err := encodeTransactionData(ctx, req.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction data. %s", err)
	}
	LockParams := map[string]interface{}{
		"delegate": params.Delegate,
		"proof":    decodedTransfer.Proof,
		"data":     data,
	}
	paramsJSON, err := json.Marshal(LockParams)
	if err != nil {
		return nil, err
	}
	contractAbi, err := h.zeto.config.GetContractAbi(tx.DomainConfig.TokenName)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(contractAbi.Functions()["lockProof"])
	if err != nil {
		return nil, err
	}

	return &prototk.PrepareTransactionResponse{
		Transaction: &prototk.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}
