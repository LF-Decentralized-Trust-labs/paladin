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

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
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
	notary := tx.DomainConfig.NotaryLookup

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       notary,
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
	notary := tx.DomainConfig.NotaryLookup

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", params.To, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	inputCoins, inputStates, total, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := h.noto.prepareOutputs(toAddress, params.Amount, []string{notary, tx.Transaction.From, params.To})
	if err != nil {
		return nil, err
	}
	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, tx.Transaction.From, params.To})
	if err != nil {
		return nil, err
	}

	if total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.Int())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(fromAddress, (*tktypes.HexUint256)(remainder), []string{notary, tx.Transaction.From})
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
				Parties:         []string{notary},
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
			InfoStates:   infoStates,
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
			if err := h.noto.validateTransferSignature(ctx, tx, "sender", req, coins); err != nil {
				return nil, err
			}
			return &prototk.EndorseTransactionResponse{
				EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
			}, nil
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}

	return nil, i18n.NewError(ctx, msgs.MsgUnrecognizedEndorsement, req.EndorsementRequest.Name)
}

func (h *transferHandler) baseLedgerTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, withApproval bool) (*TransactionWrapper, error) {
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
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, tx.DomainConfig.Variant)
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
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
	fn := "transfer"
	if withApproval {
		fn = "transferWithApproval"
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()[fn],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *transferHandler) hookTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.TransferParams)

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", inParams.To, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &TransferHookParams{
		Sender: fromAddress,
		From:   fromAddress,
		To:     toAddress,
		Amount: inParams.Amount,
		Prepared: PreparedTransaction{
			ContractAddress: (*tktypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType := prototk.PreparedTransaction_PUBLIC
	functionABI := solutils.MustLoadBuild(notoHooksJSON).ABI.Functions()["onTransfer"]
	var paramsJSON []byte

	if tx.DomainConfig.PrivateAddress != nil {
		transactionType = prototk.PreparedTransaction_PRIVATE
		functionABI = penteInvokeABI("onTransfer", functionABI.Inputs)
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

func (h *transferHandler) makeDomainData(ctx context.Context, withApprovalTX *TransactionWrapper, req *prototk.PrepareTransactionRequest) ([]byte, error) {
	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}
	encodedCall, err := withApprovalTX.functionABI.EncodeCallDataJSONCtx(ctx, withApprovalTX.paramsJSON)
	if err != nil {
		return nil, err
	}
	domainData := &types.NotoTransferMetadata{
		ApprovalParams: types.ApproveExtraParams{
			Data: data,
		},
		TransferWithApproval: types.NotoPublicTransaction{
			FunctionABI: withApprovalTX.functionABI,
			ParamsJSON:  withApprovalTX.paramsJSON,
			EncodedCall: encodedCall,
		},
	}
	return json.Marshal(domainData)
}

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	var err error
	var baseTransaction *TransactionWrapper
	var withApprovalTransaction *TransactionWrapper
	var hookTransaction *TransactionWrapper
	var withApprovalHookTransaction *TransactionWrapper
	var metadata []byte

	// If preparing a transaction for later use, return metadata allowing it to be delegated to an approved party
	prepareApprovals := req.Transaction.Intent == prototk.TransactionSpecification_PREPARE_TRANSACTION

	baseTransaction, err = h.baseLedgerTransfer(ctx, tx, req, false)
	if err != nil {
		return nil, err
	}
	if prepareApprovals {
		withApprovalTransaction, err = h.baseLedgerTransfer(ctx, tx, req, true)
		if err != nil {
			return nil, err
		}
	}

	if tx.DomainConfig.NotaryType == types.NotaryTypePente {
		hookTransaction, err = h.hookTransfer(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		if prepareApprovals {
			withApprovalHookTransaction, err = h.hookTransfer(ctx, tx, req, withApprovalTransaction)
			if err != nil {
				return nil, err
			}
			metadata, err = h.makeDomainData(ctx, withApprovalHookTransaction, req)
			if err != nil {
				return nil, err
			}
		}
		return hookTransaction.prepare(metadata)
	}

	if prepareApprovals {
		metadata, err = h.makeDomainData(ctx, withApprovalTransaction, req)
		if err != nil {
			return nil, err
		}
	}
	return baseTransaction.prepare(metadata)
}
