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

func (h *unlockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, sender, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if !*tx.DomainConfig.Options.Basic.RestrictUnlock {
		return nil
	}

	params := tx.Params.(*types.UnlockParams)
	localNodeName, _ := h.noto.Callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	fromQualified, err := tktypes.PrivateIdentityLocator(params.From).FullyQualified(ctx, localNodeName.Name)
	if err != nil {
		return err
	}
	if sender == fromQualified.String() {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgUnlockOnlyCreator, sender, from)
}

func (h *unlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, req.Transaction.From, params.From); err != nil {
		return nil, err
	}

	verifierMap := make(map[string]bool, len(params.To)+3)
	verifierList := make([]string, 0, len(params.To)+3)
	for _, lookup := range []string{notary, tx.Transaction.From, params.From} {
		if _, ok := verifierMap[lookup]; !ok {
			verifierMap[lookup] = true
			verifierList = append(verifierList, lookup)
		}
	}
	for _, lookup := range params.To {
		if _, ok := verifierMap[lookup]; !ok {
			verifierMap[lookup] = true
			verifierList = append(verifierList, lookup)
		}
	}

	request := make([]*prototk.ResolveVerifierRequest, len(verifierList))
	for i, lookup := range verifierList {
		request[i] = &prototk.ResolveVerifierRequest{
			Lookup:       lookup,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		}
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: request,
	}, nil
}

func (h *unlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	res, lockedInputStates, lockedOutputStates, unlockedOutputStates, infoStates, err := h.assembleUnlock(ctx, tx, req)
	if err == nil && res.AssembledTransaction != nil {
		res.AssembledTransaction.InputStates = lockedInputStates
		res.AssembledTransaction.OutputStates = unlockedOutputStates
		res.AssembledTransaction.OutputStates = append(res.AssembledTransaction.OutputStates, lockedOutputStates...)
		res.AssembledTransaction.InfoStates = infoStates
	}
	return res, err
}

func (h *unlockHandler) assembleUnlock(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, []*prototk.StateRef, []*prototk.NewState, []*prototk.NewState, []*prototk.NewState, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, nil, nil, nil, err
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
			}, nil, nil, nil, nil, nil
		}
		return nil, nil, nil, nil, nil, err
	}
	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, params.From})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	unlockedOutputCoins := []*types.NotoCoin{}
	unlockedOutputStates := []*prototk.NewState{}
	for i, to := range params.To {
		toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", to, req.ResolvedVerifiers)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		states, err := h.noto.prepareOutputs(toAddress, params.Amounts[i], []string{notary, params.From, to})
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		unlockedOutputCoins = append(unlockedOutputCoins, states.coins...)
		unlockedOutputStates = append(unlockedOutputStates, states.states...)
	}

	lockedOutputCoins := []*types.NotoLockedCoin{}
	lockedOutputStates := []*prototk.NewState{}
	if lockedInputStates.total.Cmp(requiredTotal) == 1 {
		remainder := big.NewInt(0).Sub(lockedInputStates.total, requiredTotal)
		states, err := h.noto.prepareLockedOutputs(params.LockID, fromAddress, (*tktypes.HexUint256)(remainder), []string{notary, params.From})
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		lockedOutputCoins = append(lockedOutputCoins, states.coins...)
		lockedOutputStates = append(lockedOutputStates, states.states...)
	}

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedInputStates.coins, lockedOutputCoins, unlockedOutputCoins)
	if err != nil {
		return nil, nil, nil, nil, nil, err
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
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AttestationPlan:      attestation,
		AssembledTransaction: &prototk.AssembledTransaction{}, // will be filled in by caller
	}, lockedInputStates.states, lockedOutputStates, unlockedOutputStates, infoStates, nil
}

func (h *unlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return h.endorseUnlock(ctx, tx, req.Transaction.From, req.ResolvedVerifiers, req.Signatures, req.Inputs, req.Outputs)
}

func (h *unlockHandler) endorseUnlock(
	ctx context.Context,
	tx *types.ParsedTransaction,
	sender string,
	verifiers []*prototk.ResolvedVerifier,
	attestations []*prototk.AttestationResult,
	inputs, outputs []*prototk.EndorsableState,
) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	if err := h.checkAllowed(ctx, tx, sender, params.From); err != nil {
		return nil, err
	}

	coins, lockedCoins, err := h.noto.gatherCoins(ctx, inputs, outputs)
	if err != nil {
		return nil, err
	}

	// Validate the amounts, and lock creator's ownership of all locked inputs/outputs
	if err := h.noto.validateUnlockAmounts(ctx, coins, lockedCoins); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, verifiers, lockedCoins.inCoins, lockedCoins.inStates); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, verifiers, lockedCoins.outCoins, lockedCoins.outStates); err != nil {
		return nil, err
	}

	// Notary checks the signatures from the sender, then submits the transaction
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedCoins.inCoins, lockedCoins.outCoins, coins.outCoins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", attestations, encodedUnlock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *unlockHandler) extractStates(inputs, outputs []*prototk.EndorsableState) (lockedInputs []*prototk.EndorsableState, lockedOutputs []*prototk.EndorsableState, unlockedOutputs []*prototk.EndorsableState) {
	lockedInputs = inputs
	for _, output := range outputs {
		switch output.SchemaId {
		case h.noto.coinSchema.Id:
			unlockedOutputs = append(unlockedOutputs, output)
		case h.noto.lockedCoinSchema.Id:
			lockedOutputs = append(lockedOutputs, output)
		}
	}
	return lockedInputs, lockedOutputs, unlockedOutputs
}

func (h *unlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)
	lockedInputs, lockedOutputs, outputs := h.extractStates(req.InputStates, req.OutputStates)

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
		LockedInputs:  endorsableStateIDs(lockedInputs),
		LockedOutputs: endorsableStateIDs(lockedOutputs),
		Outputs:       endorsableStateIDs(outputs),
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
		h.noto.hooksABI.Functions()["onUnlock"],
		params,
	)
	if err != nil {
		return nil, err
	}

	return &TransactionWrapper{
		transactionType: mapPrepareTransactionType(transactionType),
		functionABI:     functionABI,
		paramsJSON:      paramsJSON,
		contractAddress: tx.DomainConfig.Options.Hooks.PublicAddress,
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
