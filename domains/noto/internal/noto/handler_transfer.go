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
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type transferHandler struct {
	noto *Noto
}

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.NotoConfigOutput_V0, params string) (interface{}, error) {
	var transferParams types.TransferParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}
	if transferParams.To == "" {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "to")
	}
	if transferParams.Amount.BigInt().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &transferParams, nil
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       tx.DomainConfig.NotaryLookup,
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

	notary := domain.FindVerifier(tx.DomainConfig.NotaryLookup, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if notary == nil || notary.Verifier != tx.DomainConfig.NotaryAddress {
		return nil, i18n.NewError(ctx, msgs.MsgNotaryUnexpectedAddress, tx.DomainConfig.NotaryAddress, notary.Verifier)
	}
	from := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if from == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "from")
	}
	fromAddress, err := ethtypes.NewAddress(from.Verifier)
	if err != nil {
		return nil, err
	}
	to := domain.FindVerifier(params.To, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if to == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "to")
	}
	toAddress, err := ethtypes.NewAddress(to.Verifier)
	if err != nil {
		return nil, err
	}

	inputCoins, inputStates, total, err := h.noto.prepareInputs(ctx, req.Transaction.ContractAddress, *fromAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := h.noto.prepareOutputs(notary.Lookup, to.Lookup, *toAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.BigInt()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.BigInt())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(notary.Lookup, from.Lookup, *fromAddress, ethtypes.NewHexInteger(remainder))
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	var attestation []*prototk.AttestationRequest
	switch tx.DomainConfig.Variant.String() {
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
				Parties:         []string{tx.DomainConfig.NotaryLookup},
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
				Parties:         []string{tx.DomainConfig.NotaryLookup},
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

func (h *transferHandler) validateAmounts(ctx context.Context, coins *gatheredCoins) error {
	if coins.inTotal.Cmp(coins.outTotal) != 0 {
		return i18n.NewError(ctx, msgs.MsgInvalidAmount, "transfer", coins.inTotal, coins.outTotal)
	}
	return nil
}

func (h *transferHandler) validateSenderSignature(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, coins *gatheredCoins) error {
	signature := domain.FindAttestation("sender", req.Signatures)
	if signature == nil {
		return i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}
	if signature.Verifier.Lookup != tx.Transaction.From {
		return i18n.NewError(ctx, msgs.MsgAttestationUnexpected, "sender", tx.Transaction.From, signature.Verifier.Lookup)
	}
	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, coins.inCoins, coins.outCoins)
	if err != nil {
		return err
	}
	recoveredSignature, err := h.noto.recoverSignature(ctx, encodedTransfer, signature.Payload)
	if err != nil {
		return err
	}
	if recoveredSignature.String() != signature.Verifier.Verifier {
		return i18n.NewError(ctx, msgs.MsgSignatureDoesNotMatch, "sender", signature.Verifier.Verifier, recoveredSignature.String())
	}
	return nil
}

func (h *transferHandler) validateOwners(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest, coins *gatheredCoins) error {
	from := domain.FindVerifier(tx.Transaction.From, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, req.ResolvedVerifiers)
	if from == nil {
		return i18n.NewError(ctx, msgs.MsgErrorVerifyingAddress, "from")
	}
	fromAddress, err := ethtypes.NewAddress(from.Verifier)
	if err != nil {
		return err
	}

	for i, coin := range coins.inCoins {
		if coin.Owner != *fromAddress {
			return i18n.NewError(ctx, msgs.MsgStateWrongOwner, coins.inStates[i].Id, tx.Transaction.From)
		}
	}
	return nil
}

func (h *transferHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	coins, err := h.noto.gatherCoins(ctx, req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}
	if err := h.validateAmounts(ctx, coins); err != nil {
		return nil, err
	}
	if err := h.validateOwners(ctx, tx, req, coins); err != nil {
		return nil, err
	}

	switch tx.DomainConfig.Variant.String() {
	case types.NotoVariantDefault:
		if req.EndorsementRequest.Name == "notary" {
			// Notary checks the signature from the sender, then submits the transaction
			if err := h.validateSenderSignature(ctx, tx, req, coins); err != nil {
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
			encodedTransfer, err := h.noto.encodeTransferMasked(ctx, tx.ContractAddress, inputIDs, outputIDs, ethtypes.HexBytes0xPrefix(data))
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

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	inputs := make([]string, len(req.InputStates))
	for i, state := range req.InputStates {
		inputs[i] = state.Id
	}
	outputs := make([]string, len(req.OutputStates))
	for i, state := range req.OutputStates {
		outputs[i] = state.Id
	}

	var signature *prototk.AttestationResult
	switch tx.DomainConfig.Variant.String() {
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
	params := map[string]interface{}{
		"inputs":    inputs,
		"outputs":   outputs,
		"signature": ethtypes.HexBytes0xPrefix(signature.Payload),
		"data":      data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(h.noto.contractABI.Functions()[tx.FunctionABI.Name])
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
