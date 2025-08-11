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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
)

type unlockCommon struct {
	noto *Noto
}

type unlockHandler struct {
	unlockCommon
}

type unlockStates struct {
	lockedInputs  *preparedLockedInputs
	lockedOutputs *preparedLockedOutputs
	outputs       *preparedOutputs
	info          []*prototk.NewState
}

func (h *unlockCommon) validateParams(ctx context.Context, unlockParams *types.UnlockParams) error {
	if unlockParams.LockID.IsZero() {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	if len(unlockParams.From) == 0 {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
	}
	if len(unlockParams.Recipients) == 0 {
		return i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	return nil
}

func (h *unlockCommon) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}

	localNodeName, _ := h.noto.Callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	fromQualified, err := pldtypes.PrivateIdentityLocator(from).FullyQualified(ctx, localNodeName.Name)
	if err != nil {
		return err
	}
	if tx.Transaction.From == fromQualified.String() {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgUnlockOnlyCreator, tx.Transaction.From, from)
}

func (h *unlockCommon) init(ctx context.Context, tx *types.ParsedTransaction, params *types.UnlockParams) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, params.From); err != nil {
		return nil, err
	}

	lookups := []string{notary, tx.Transaction.From, params.From}
	for _, entry := range params.Recipients {
		lookups = append(lookups, entry.To)
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(lookups...),
	}, nil
}

func (h *unlockCommon) assembleStates(ctx context.Context, tx *types.ParsedTransaction, params *types.UnlockParams, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, *unlockStates, error) {
	notary := tx.DomainConfig.NotaryLookup

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, nil, err
	}

	requiredTotal := big.NewInt(0)
	for _, entry := range params.Recipients {
		requiredTotal = requiredTotal.Add(requiredTotal, entry.Amount.Int())
	}

	lockedInputStates, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, params.LockID, fromAddress, requiredTotal)
	if err != nil {
		if revert {
			message := err.Error()
			return &prototk.AssembleTransactionResponse{
				AssemblyResult: prototk.AssembleTransactionResponse_REVERT,
				RevertReason:   &message,
			}, nil, nil
		}
		return nil, nil, err
	}

	remainder := big.NewInt(0).Sub(lockedInputStates.total, requiredTotal)
	unlockedOutputs, lockedOutputs, err := h.assembleUnlockOutputs(ctx, tx, params, req, fromAddress, remainder)
	if err != nil {
		return nil, nil, err
	}

	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, params.From})
	if err != nil {
		return nil, nil, err
	}
	lockState, err := h.noto.prepareLockInfo(params.LockID, fromAddress, nil, []string{notary, params.From})
	if err != nil {
		return nil, nil, err
	}
	infoStates = append(infoStates, lockState)

	return &prototk.AssembleTransactionResponse{
			AssemblyResult: prototk.AssembleTransactionResponse_OK,
		}, &unlockStates{
			lockedInputs:  lockedInputStates,
			lockedOutputs: lockedOutputs,
			outputs:       unlockedOutputs,
			info:          infoStates,
		}, nil
}

func (h *unlockCommon) assembleUnlockOutputs(ctx context.Context, tx *types.ParsedTransaction, params *types.UnlockParams, req *prototk.AssembleTransactionRequest, from *pldtypes.EthAddress, remainder *big.Int) (*preparedOutputs, *preparedLockedOutputs, error) {
	notary := tx.DomainConfig.NotaryLookup

	unlockedOutputs := &preparedOutputs{}
	for _, entry := range params.Recipients {
		toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, nil, err
		}
		outputs, err := h.noto.prepareOutputs(toAddress, entry.Amount, []string{notary, params.From, entry.To})
		if err != nil {
			return nil, nil, err
		}
		unlockedOutputs.coins = append(unlockedOutputs.coins, outputs.coins...)
		unlockedOutputs.states = append(unlockedOutputs.states, outputs.states...)
	}

	lockedOutputs := &preparedLockedOutputs{}
	if remainder.Cmp(big.NewInt(0)) == 1 {
		var err error
		lockedOutputs, err = h.noto.prepareLockedOutputs(params.LockID, from, (*pldtypes.HexUint256)(remainder), []string{notary, params.From})
		if err != nil {
			return nil, nil, err
		}
	}

	return unlockedOutputs, lockedOutputs, nil
}

func (h *unlockCommon) endorse(
	ctx context.Context,
	tx *types.ParsedTransaction,
	params *types.UnlockParams,
	req *prototk.EndorseTransactionRequest,
	inputs, outputs *parsedCoins,
) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkAllowed(ctx, tx, params.From); err != nil {
		return nil, err
	}

	// Validate the amounts, and lock creator's ownership of all locked inputs/outputs
	if err := h.noto.validateUnlockAmounts(ctx, inputs, outputs); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, inputs.lockedCoins, inputs.lockedStates); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, outputs.lockedCoins, outputs.lockedStates); err != nil {
		return nil, err
	}

	// Notary checks the signatures from the sender, then submits the transaction
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, inputs.lockedCoins, outputs.lockedCoins, outputs.coins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedUnlock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *unlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.UnlockParams
	err := json.Unmarshal([]byte(params), &unlockParams)
	if err == nil {
		err = h.validateParams(ctx, &unlockParams)
	}
	return &unlockParams, err
}

func (h *unlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	return h.init(ctx, tx, params)
}

func (h *unlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup

	res, states, err := h.assembleStates(ctx, tx, params, req)
	if err != nil || res.AssemblyResult != prototk.AssembleTransactionResponse_OK {
		return res, err
	}

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.InputStates = states.lockedInputs.states
	assembledTransaction.OutputStates = states.outputs.states
	assembledTransaction.OutputStates = append(assembledTransaction.OutputStates, states.lockedOutputs.states...)
	assembledTransaction.InfoStates = states.info

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, states.lockedInputs.coins, states.lockedOutputs.coins, states.outputs.coins)
	if err != nil {
		return nil, err
	}

	return &prototk.AssembleTransactionResponse{
		AssemblyResult:       prototk.AssembleTransactionResponse_OK,
		AssembledTransaction: assembledTransaction,
		AttestationPlan: []*prototk.AttestationRequest{
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
		},
	}, nil
}

func (h *unlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	inputs, err := h.noto.parseCoinList(ctx, "input", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", req.Outputs)
	if err != nil {
		return nil, err
	}
	return h.endorse(ctx, tx, params, req, inputs, outputs)
}

func (h *unlockHandler) baseLedgerInvoke(ctx context.Context, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	lockedInputs := req.InputStates
	outputs, lockedOutputs := h.noto.splitStates(req.OutputStates)

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
	params := &types.UnlockPublicParams{
		TxId:          req.Transaction.TransactionId,
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
		functionABI: interfaceBuild.ABI.Functions()["unlock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *unlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	senderAddress, err := h.noto.findEthAddressVerifier(ctx, "sender", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	unlock := make([]*ResolvedUnlockRecipient, len(inParams.Recipients))
	for i, entry := range inParams.Recipients {
		to, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		unlock[i] = &ResolvedUnlockRecipient{To: to, Amount: entry.Amount}
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &UnlockHookParams{
		Sender:     senderAddress,
		LockID:     inParams.LockID,
		Recipients: unlock,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onUnlock"],
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
	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	baseTransaction, err := h.baseLedgerInvoke(ctx, req)
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
