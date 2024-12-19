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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type lockHandler struct {
	noto *Noto
}

func (h *lockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var lockParams types.LockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, err
	}
	if lockParams.ID.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "id")
	}
	if lockParams.Delegate.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "delegate")
	}
	if lockParams.Amount == nil || lockParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &lockParams, nil
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)
	notary := tx.DomainConfig.NotaryLookup

	requests := make([]*prototk.ResolveVerifierRequest, 0, len(params.Recipients)+2)
	requests = append(requests,
		&prototk.ResolveVerifierRequest{
			Lookup:       notary,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
		&prototk.ResolveVerifierRequest{
			Lookup:       tx.Transaction.From,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	)
	for _, recipient := range params.Recipients {
		requests = append(requests, &prototk.ResolveVerifierRequest{
			Lookup:       recipient.Recipient,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		})
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: requests,
	}, nil
}

func (h *lockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)
	notary := tx.DomainConfig.NotaryLookup

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	inputCoins, inputStates, total, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromAddress, params.Amount)
	if err != nil {
		return nil, err
	}
	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}

	// A single locked coin, owned by the submitter and with a value equal to the transfer amount
	lockedCoin := &types.NotoLockedCoin{
		ID:     params.ID,
		Owner:  fromAddress,
		Amount: params.Amount,
	}
	lockedCoinState, err := h.noto.makeNewLockedCoinState(lockedCoin, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}

	recipientCoins := make([]*types.NotoCoin, len(params.Recipients))
	recipientStates := make([]*prototk.NewState, len(params.Recipients))
	for i, recipient := range params.Recipients {
		recipientAddress, err := h.noto.findEthAddressVerifier(ctx, recipient.Recipient, recipient.Recipient, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		// A single output coin, unlocked, with specified owner
		// TODO: make this configurable
		recipientCoins[i] = &types.NotoCoin{
			Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
			Owner:  recipientAddress,
			Amount: params.Amount,
		}
		recipientStates[i], err = h.noto.makeNewCoinState(recipientCoins[i], []string{notary, tx.Transaction.From, recipient.Recipient})
		if err != nil {
			return nil, err
		}
	}

	outputCoins := []*types.NotoCoin{}
	outputStates := []*prototk.NewState{lockedCoinState}
	outputStates = append(outputStates, recipientStates...)

	if total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.Int())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(fromAddress, (*tktypes.HexUint256)(remainder), []string{notary, tx.Transaction.From})
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	encodedTransfer, err := h.noto.encodeTransferUnmasked(ctx, tx.ContractAddress, inputCoins, outputCoins)
	if err != nil {
		return nil, err
	}
	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, lockedCoin, recipientCoins)
	if err != nil {
		return nil, err
	}

	attestation := []*prototk.AttestationRequest{
		// Sender confirms the initial request with a signature
		{
			Name:            "sender_transfer",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         encodedTransfer,
			PayloadType:     signpayloads.OPAQUE_TO_RSV,
			Parties:         []string{req.Transaction.From},
		},
		{
			Name:            "sender_lock",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         encodedLock,
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

func (h *lockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.LockParams)

	lockedState := req.Outputs[0]
	recipientStates := req.Outputs[1 : 1+len(params.Recipients)]
	remainderStates := req.Outputs[1+len(params.Recipients):]

	if lockedState.SchemaId != h.noto.LockedCoinSchemaID() {
		return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, lockedState.SchemaId)
	}
	lockedCoin, err := h.noto.unmarshalLockedCoin(lockedState.StateDataJson)
	if err != nil {
		return nil, err
	}

	recipientCoins := make([]*types.NotoCoin, len(recipientStates))
	for i, state := range recipientStates {
		if state.SchemaId != h.noto.CoinSchemaID() {
			return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, state.SchemaId)
		}
		recipientCoins[i], err = h.noto.unmarshalCoin(state.StateDataJson)
		if err != nil {
			return nil, err
		}
	}

	coins, err := h.noto.gatherCoins(ctx, req.Inputs, remainderStates)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateLockAmounts(ctx, coins, lockedCoin, recipientCoins); err != nil {
		return nil, err
	}
	if err := h.noto.validateOwners(ctx, tx, req, coins); err != nil {
		return nil, err
	}

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	if !lockedCoin.Owner.Equals(fromAddress) {
		return nil, i18n.NewError(ctx, msgs.MsgStateWrongOwner, lockedState.Id, tx.Transaction.From)
	}

	if req.EndorsementRequest.Name == "notary" {
		// Notary checks the signatures from the sender, then submits the transaction
		if err := h.noto.validateTransferSignature(ctx, tx, "sender_transfer", req, coins); err != nil {
			return nil, err
		}
		if err := h.noto.validateLockSignature(ctx, tx, "sender_lock", req, lockedCoin, recipientCoins); err != nil {
			return nil, err
		}
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		}, nil
	}

	return nil, i18n.NewError(ctx, msgs.MsgUnrecognizedEndorsement, req.EndorsementRequest.Name)
}

func (h *lockHandler) baseLedgerTransfer(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inputParams := tx.Params.(*types.LockParams)

	inputs := make([]string, len(req.InputStates))
	for i, state := range req.InputStates {
		inputs[i] = state.Id
	}

	lockedOutput, err := tktypes.ParseBytes32Ctx(ctx, req.OutputStates[0].Id)
	if err != nil {
		return nil, err
	}

	lockOutcomes := make([]*LockOutcome, len(inputParams.Recipients))
	remainderOutputs := make([]string, len(req.OutputStates)-len(inputParams.Recipients)-1)
	for i, state := range req.OutputStates[1 : 1+len(inputParams.Recipients)] {
		id, err := tktypes.ParseBytes32Ctx(ctx, state.Id)
		if err != nil {
			return nil, err
		}
		lockOutcomes[i] = &LockOutcome{
			Ref:   inputParams.Recipients[i].Ref,
			State: id,
		}
	}
	for i, state := range req.OutputStates[1+len(inputParams.Recipients):] {
		remainderOutputs[i] = state.Id
	}

	// Include the signatures from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	transferSignature := domain.FindAttestation("sender_transfer", req.AttestationResult)
	if transferSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender_transfer")
	}
	lockSignature := domain.FindAttestation("sender_lock", req.AttestationResult)
	if lockSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender_lock")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}
	params := &NotoTransferAndLockParams{
		Transfer: NotoTransferParamsNoData{
			Inputs:    inputs,
			Outputs:   remainderOutputs,
			Signature: transferSignature.Payload,
		},
		Lock: NotoLockParamsNoData{
			Locked:    lockedOutput,
			Outcomes:  lockOutcomes,
			Delegate:  inputParams.Delegate,
			Signature: lockSignature.Payload,
		},
		Data: data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()["transferAndLock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *lockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	var err error
	var baseTransaction *TransactionWrapper

	baseTransaction, err = h.baseLedgerTransfer(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryType == types.NotaryTypePente {
		return nil, fmt.Errorf("not supported") // TODO
	}

	return baseTransaction.prepare(nil)
}
