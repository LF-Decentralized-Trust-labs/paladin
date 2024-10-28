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
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/i18n"
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

type transferHandler struct {
	noto *Noto
}

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.NotoConfig_V0, params string) (interface{}, error) {
	var transferParams types.TransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}
	if transferParams.To == "" {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "to")
	}
	if transferParams.Amount == nil || transferParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &transferParams, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       tx.DomainConfig.DecodedData.NotaryLookup,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			{
				Lookup:       tx.Transaction.From,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
			{
				Lookup:       params.To,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (h *transferHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)

	notary := domain.FindVerifier(tx.DomainConfig.DecodedData.NotaryLookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if notary == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "notary")
	}
	from := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if from == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "from")
	}
	fromAddress, err := tktypes.ParseEthAddress(from.Verifier)
	if err != nil {
		return nil, err
	}
	to := domain.FindVerifier(params.To, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if to == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "to")
	}
	toAddress, err := tktypes.ParseEthAddress(to.Verifier)
	if err != nil {
		return nil, err
	}

	inputCoins, inputStates, total, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := h.noto.prepareOutputs(notary.Lookup, to.Lookup, toAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.Int())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(notary.Lookup, from.Lookup, fromAddress, (*tktypes.HexUint256)(remainder))
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	var attestation []*prototk.AttestationRequest
	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputCoins, outputCoins)
		if err != nil {
			return nil, err
		}
		attestation = []*prototk.AttestationRequest{
			// Sender confirms the initial request with a signature
			{
				Name:            "sender",
				AttestationType: prototk.AttestationType_SIGN,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Payload:         encodedTransfer,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Parties:         []string{req.Transaction.From},
			},
			// Notary will endorse the assembled transaction (by submitting to the ledger)
			{
				Name:            "notary",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{tx.DomainConfig.DecodedData.NotaryLookup},
			},
		}
	case types.NotoVariantSelfSubmit:
		attestation = []*prototk.AttestationRequest{
			// Notary will endorse the assembled transaction (by providing a signature)
			{
				Name:            "notary",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				PayloadType:     signpayloads.OPAQUE_TO_RSV,
				Parties:         []string{tx.DomainConfig.DecodedData.NotaryLookup},
			},
			// Sender will endorse the assembled transaction (by submitting to the ledger)
			{
				Name:            "sender",
				AttestationType: prototk.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1,
				VerifierType:    verifiers.ETH_ADDRESS,
				Parties:         []string{req.Transaction.From},
			},
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult: prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: &prototk.AssembledTransaction{
			InputStates:  inputStates,
			OutputStates: outputStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
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

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		if req.EndorsementRequest.Name == "notary" {
			// Notary checks the signature from the sender, then submits the transaction
			if err := h.noto.validateTransferSignature(ctx, tx, req, coins); err != nil {
				return nil, err
			}
			return &prototk.EndorseTransactionResponse{
				EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
			}, nil
		}
	case types.NotoVariantSelfSubmit:
		if req.EndorsementRequest.Name == "notary" {
			// Notary provides a signature for the assembled payload (to be verified on base ledger)
			inputIDs := make([]interface{}, len(req.Inputs))
			outputIDs := make([]interface{}, len(req.Outputs))
			for i, state := range req.Inputs {
				inputIDs[i] = state.Id
			}
			for i, state := range req.Outputs {
				outputIDs[i] = state.Id
			}
			data, err := h.noto.encodeTransactionData(ctx, req.Transaction)
			if err != nil {
				return nil, err
			}
			encodedTransfer, err := h.noto.encodeTransferMasked(ctx, tx.ContractAddress, inputIDs, outputIDs, data)
			if err != nil {
				return nil, err
			}
			return &prototk.EndorseTransactionResponse{
				EndorsementResult: prototk.EndorseTransactionResponse_SIGN,
				Payload:           encodedTransfer,
			}, nil
		} else if req.EndorsementRequest.Name == "sender" {
			if req.EndorsementVerifier.Lookup == tx.Transaction.From {
				// Sender submits the transaction
				return &prototk.EndorseTransactionResponse{
					EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
				}, nil
			}
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}

	return nil, i18n.NewError(ctx, msgs.MsgUnrecognizedEndorsement, req.EndorsementRequest.Name)
}

func (h *transferHandler) baseLedgerTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inputs := make([]string, len(req.InputStates))
	for i, state := range req.InputStates {
		inputs[i] = state.Id
	}
	outputs := make([]string, len(req.OutputStates))
	for i, state := range req.OutputStates {
		outputs[i] = state.Id
	}

	var signature *prototk.AttestationResult
	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		// Include the signature from the sender
		// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
		signature = domain.FindAttestation("sender", req.AttestationResult)
		if signature == nil {
			return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
		}
	case types.NotoVariantSelfSubmit:
		// Include the signature from the notary (will be verified on base ledger)
		signature = domain.FindAttestation("notary", req.AttestationResult)
		if signature == nil {
			return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	params := &NotoTransferParams{
		Inputs:    inputs,
		Outputs:   outputs,
		Signature: signature.Payload,
		Data:      data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()[tx.FunctionABI.Name],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *transferHandler) guardTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.TransferParams)

	from := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if from == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "from")
	}
	fromAddress, err := tktypes.ParseEthAddress(from.Verifier)
	if err != nil {
		return nil, err
	}
	to := domain.FindVerifier(inParams.To, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if to == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "to")
	}
	toAddress, err := tktypes.ParseEthAddress(to.Verifier)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &GuardTransferParams{
		From:   fromAddress,
		To:     toAddress,
		Amount: inParams.Amount,
		Prepared: PreparedTransaction{
			ContractAddress: (*tktypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType := prototk.PreparedTransaction_PUBLIC
	functionABI := solutils.MustLoadBuild(notoGuardJSON).ABI.Functions()["onTransfer"]
	var paramsJSON []byte

	if tx.DomainConfig.DecodedData.PrivateAddress != nil {
		transactionType = prototk.PreparedTransaction_PRIVATE
		functionABI = penteInvokeABI("onMint", functionABI.Inputs)
		penteParams := &PenteInvokeParams{
			Group:  tx.DomainConfig.DecodedData.PrivateGroup,
			To:     tx.DomainConfig.DecodedData.PrivateAddress,
			Inputs: params,
		}
		paramsJSON, err = json.Marshal(penteParams)
	} else {
		// Note: public guards aren't really useful except in testing
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

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerTransfer(ctx, tx, req)
	if err != nil {
		return nil, err
	}
	if tx.DomainConfig.NotaryType.Equals(&types.NotaryTypeContract) {
		guardTransaction, err := h.guardTransfer(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return guardTransaction.prepare()
	}
	return baseTransaction.prepare()
}
