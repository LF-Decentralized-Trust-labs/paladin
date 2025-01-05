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

type unlockHandler struct {
	noto *Noto
}

func (h *unlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.UnlockParams
	if err := json.Unmarshal([]byte(params), &unlockParams); err != nil {
		return nil, err
	}
	if unlockParams.LockID.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	if len(unlockParams.From) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
	}
	if len(unlockParams.To) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "to")
	}
	if len(unlockParams.Amounts) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "amounts")
	}
	if len(unlockParams.To) != len(unlockParams.Amounts) {
		return nil, i18n.NewError(ctx, msgs.MsgArraysMustBeSameLength, "to", "amounts")
	}
	return &unlockParams, nil
}

func (h *unlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup

	request := make([]*prototk.ResolveVerifierRequest, 0, len(params.To)+3)
	request = append(request,
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
		&prototk.ResolveVerifierRequest{
			Lookup:       params.From,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		},
	)
	for _, to := range params.To {
		request = append(request, &prototk.ResolveVerifierRequest{
			Lookup:       to,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		})
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: request,
	}, nil
}

func (h *unlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	requiredTotal := big.NewInt(0)
	for _, amount := range params.Amounts {
		requiredTotal = requiredTotal.Add(requiredTotal, amount.Int())
	}

	lockedInputStates, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, params.LockID, fromAddress, requiredTotal)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil
		}
		return nil, err
	}
	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, params.From})
	if err != nil {
		return nil, err
	}

	unlockedOutputCoins := []*types.NotoCoin{}
	outputStates := []*prototk.NewState{}
	for i, to := range params.To {
		toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", to, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		states, err := h.noto.prepareOutputs(toAddress, params.Amounts[i], []string{notary, params.From, to})
		if err != nil {
			return nil, err
		}
		unlockedOutputCoins = append(unlockedOutputCoins, states.coins...)
		outputStates = append(outputStates, states.states...)
	}

	lockedOutputCoins := []*types.NotoLockedCoin{}
	if lockedInputStates.total.Cmp(requiredTotal) == 1 {
		remainder := big.NewInt(0).Sub(lockedInputStates.total, requiredTotal)
		states, err := h.noto.prepareLockedOutputs(params.LockID, fromAddress, (*tktypes.HexUint256)(remainder), []string{notary, params.From})
		if err != nil {
			return nil, err
		}
		lockedOutputCoins = append(lockedOutputCoins, states.coins...)
		outputStates = append(outputStates, states.states...)
	}

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedInputStates.coins, lockedOutputCoins, unlockedOutputCoins)
	if err != nil {
		return nil, err
	}

	attestation := []*prototk.AttestationRequest{
		// Sender confirms the initial request with a signature
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_SIGN,
			Algorithm:       algorithms.ECDSA_SECP256K1,
			VerifierType:    verifiers.ETH_ADDRESS,
			Payload:         encodedUnlock,
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
			InputStates:  lockedInputStates.states,
			OutputStates: outputStates,
			InfoStates:   infoStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *unlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	coins, lockedCoins, err := h.noto.gatherCoins(ctx, req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}

	senderAddress, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// Validate the amounts, and sender's ownership of all locked inputs/outputs
	if err := h.noto.validateUnlockAmounts(ctx, coins, lockedCoins); err != nil {
		return nil, err
	}
	if !senderAddress.Equals(fromAddress) {
		// For now, the sender can only unlock their own locked coins
		// TODO: make this configurable
		return nil, i18n.NewError(ctx, msgs.MsgUnlockNotAllowed, params.From)
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req, lockedCoins.inCoins, lockedCoins.inStates); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req, lockedCoins.outCoins, lockedCoins.outStates); err != nil {
		return nil, err
	}

	// Notary checks the signatures from the sender, then submits the transaction
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedCoins.inCoins, lockedCoins.outCoins, coins.outCoins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req, encodedUnlock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *unlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	lockedInput := req.InputStates[0].Id
	unlockedOutput := req.OutputStates[0].Id
	lockedOutputs := make([]string, len(req.OutputStates)-1)
	for i, state := range req.OutputStates[1:] {
		lockedOutputs[i] = state.Id
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	unlockSignature := domain.FindAttestation("sender", req.AttestationResult)
	if unlockSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}
	params := &NotoUnlockParams{
		LockID:        inParams.LockID,
		LockedInputs:  []string{lockedInput},
		LockedOutputs: lockedOutputs,
		Outputs:       []string{unlockedOutput},
		Signature:     unlockSignature.Payload,
		Data:          data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()["unlock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *unlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	senderAddress, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", inParams.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	toAddresses := make([]*tktypes.EthAddress, len(inParams.To))
	for i, to := range inParams.To {
		toAddresses[i], err = h.noto.findEthAddressVerifier(ctx, "to", to, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &UnlockHookParams{
		Sender:  senderAddress,
		LockID:  inParams.LockID,
		From:    fromAddress,
		To:      toAddresses,
		Amounts: inParams.Amounts,
		Data:    inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*tktypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		solutils.MustLoadBuild(notoHooksJSON).ABI.Functions()["onUnlock"],
		params,
	)
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		transactionType: mapPrepareTransactionType(transactionType),
		functionABI:     functionABI,
		paramsJSON:      paramsJSON,
		contractAddress: &tx.DomainConfig.Options.Hooks.NotaryAddress,
	}, nil
}

func (h *unlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare(nil)
	}

	return baseTransaction.prepare(nil)
}
