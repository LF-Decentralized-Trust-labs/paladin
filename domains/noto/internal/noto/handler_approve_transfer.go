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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type approveHandler struct {
	noto *Noto
}

func (h *approveHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var approveParams types.ApproveParams
	if err := json.Unmarshal([]byte(params), &approveParams); err != nil {
		return nil, err
	}
	if approveParams.Delegate.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidDelegate, approveParams.Delegate)
	}
	return &approveParams, nil
}

func (h *approveHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return &prototk.InitTransactionResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (h *approveHandler) transferHash(ctx context.Context, tx *types.ParsedTransaction, params *types.ApproveParams) (ethtypes.HexBytes0xPrefix, error) {
	inputs := make([]any, len(params.Inputs))
	for i, state := range params.Inputs {
		inputs[i] = state.ID
	}
	outputs := make([]any, len(params.Outputs))
	for i, state := range params.Outputs {
		outputs[i] = state.ID
	}
	return h.noto.encodeTransferMasked(ctx, tx.ContractAddress, inputs, outputs, params.Data)
}

func (h *approveHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.ApproveParams)
	notary := tx.DomainConfig.NotaryLookup
	transferHash, err := h.transferHash(ctx, tx, params)
	if err != nil {
		return nil, err
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  []*prototk.StateRef{},
			OutputStates: []*prototk.NewState{},
		},
		AttestationPlan: []*prototk.AttestationRequest{
			// Sender confirms the initial request with a signature
			{
				Name:            "sender",
				AttestationType: prototk.AttestationType_SIGN,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Payload:         transferHash,
				Parties:         []string{req.Transaction.From},
			},
			// Notary will endorse the assembled transaction (by submitting to the ledger)
			{
				Name:            "notary",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{notary},
			},
		},
	}, nil
}

func (h *approveHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	coins, err := h.noto.gatherCoins(ctx, req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateTransferAmounts(ctx, coins); err != nil {
		return nil, err
	}
	if err := h.noto.validateOwners(ctx, tx, req, coins); err != nil {
		return nil, err
	}

	params := tx.Params.(*types.ApproveParams)
	transferHash, err := h.transferHash(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateApprovalSignature(ctx, req, transferHash); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *approveHandler) baseLedgerApprove(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.ApproveParams)
	transferHash, err := h.transferHash(ctx, tx, inParams)
	if err != nil {
		return nil, err
	}

	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	params := &NotoApproveTransferParams{
		Delegate:  inParams.Delegate,
		TXHash:    tktypes.HexBytes(transferHash),
		Signature: sender.Payload,
		Data:      data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()["approveTransfer"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *approveHandler) hookApprove(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.ApproveParams)

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &ApproveTransferHookParams{
		Sender:   fromAddress,
		From:     fromAddress,
		Delegate: inParams.Delegate,
		Prepared: PreparedTransaction{
			ContractAddress: (*tktypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType := prototk.PreparedTransaction_PUBLIC
	functionABI := solutils.MustLoadBuild(notoHooksJSON).ABI.Functions()["onApproveTransfer"]
	var paramsJSON []byte

	if tx.DomainConfig.PrivateAddress != nil {
		transactionType = prototk.PreparedTransaction_PRIVATE
		functionABI = penteInvokeABI("onApproveTransfer", functionABI.Inputs)
		penteParams := &PenteInvokeParams{
			Group:  tx.DomainConfig.PrivateGroup,
			To:     tx.DomainConfig.PrivateAddress,
			Inputs: params,
		}
		paramsJSON, err = json.Marshal(penteParams)
	} else {
		// Note: public hooks aren't really useful except in testing, as they disclose everything
		// TODO: remove this?
		paramsJSON, err = json.Marshal(params)
	}
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		transactionType: transactionType,
		functionABI:     functionABI,
		paramsJSON:      paramsJSON,
		contractAddress: &tx.DomainConfig.NotaryAddress,
	}, nil
}

func (h *approveHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerApprove(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	if tx.DomainConfig.NotaryType == types.NotaryTypePente {
		hookTransaction, err := h.hookApprove(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare(nil)
	}
	return baseTransaction.prepare(nil)
}
