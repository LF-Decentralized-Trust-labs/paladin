/*
 * Copyright © 2025 Kaleido, Inc.
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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/domain"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/signpayloads"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
)

type prepareBurnUnlockHandler struct {
	noto *Noto
}

func (h *prepareBurnUnlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "prepareBurnUnlock is not supported in Noto V0")
	}

	var burnLockParams types.PrepareBurnUnlockParams
	if err := json.Unmarshal([]byte(params), &burnLockParams); err != nil {
		return nil, err
	}
	if burnLockParams.LockID.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "lockId")
	}
	if len(burnLockParams.From) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
	}
	if burnLockParams.Amount == nil || burnLockParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &burnLockParams, nil
}

func (h *prepareBurnUnlockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.AllowBurn {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgBurnNotAllowed)
}

func (h *prepareBurnUnlockHandler) checkAllowedForFrom(ctx context.Context, tx *types.ParsedTransaction, from string) error {
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

func (h *prepareBurnUnlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.PrepareBurnUnlockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}
	if err := h.checkAllowedForFrom(ctx, tx, params.From); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From, params.From),
	}, nil
}

func (h *prepareBurnUnlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.PrepareBurnUnlockParams)
	notary := tx.DomainConfig.NotaryLookup
	unlockTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	_, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// Read the locked inputs for the existing lock
	lockedInputStates, revert, err := h.noto.prepareLockedInputs(ctx, req.StateQueryContext, params.LockID, fromAddress, params.Amount.Int())
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

	// Validate the amount matches exactly (no remainder for burn)
	if lockedInputStates.total.Cmp(params.Amount.Int()) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "prepareBurnUnlock", params.Amount.Int().Text(10), lockedInputStates.total.Text(10))
	}

	// No outputs - this will burn when unlocked
	infoStates, err := h.noto.prepareInfo(params.Data, tx.DomainConfig.Variant, []string{notary, params.From})
	if err != nil {
		return nil, err
	}
	lockState, err := h.noto.prepareLockInfo(params.LockID, fromAddress, nil, &unlockTxId, []string{notary, params.From})
	if err != nil {
		return nil, err
	}
	infoStates = append(infoStates, lockState)

	assembledTransaction := &prototk.AssembledTransaction{
		ReadStates: lockedInputStates.states,
		InfoStates: infoStates,
	}

	// Prepare unlock with no outputs (for burning)
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, lockedInputStates.coins, nil, nil)
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

func (h *prepareBurnUnlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.PrepareBurnUnlockParams)
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}
	if err := h.checkAllowedForFrom(ctx, tx, params.From); err != nil {
		return nil, err
	}

	lockedInputs := req.Reads
	inputs, err := h.noto.parseCoinList(ctx, "input", lockedInputs)
	if err != nil {
		return nil, err
	}

	// Validate the amounts and sender's ownership of the locked inputs
	if inputs.lockedTotal.Cmp(params.Amount.Int()) != 0 {
		return nil, i18n.NewError(ctx, msgs.MsgInvalidAmount, "prepareBurnUnlock", params.Amount.Int().Text(10), inputs.lockedTotal.Text(10))
	}
	if err := h.noto.validateLockOwners(ctx, params.From, req.ResolvedVerifiers, inputs.lockedCoins, inputs.lockedStates); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	// No outputs - this will burn when unlocked
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, inputs.lockedCoins, nil, nil)
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

func (h *prepareBurnUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	params := tx.Params.(*types.PrepareBurnUnlockParams)

	lockedInputs := req.ReadStates
	// No outputs for burn scenario
	unlockHash, err := h.noto.unlockHashFromStates(ctx, tx.ContractAddress, lockedInputs, nil, nil, params.Data)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	// Read unlockTxId from LockInfo state
	lockInfo, err := h.noto.extractLockInfo(ctx, req)
	if err != nil {
		return nil, err
	}
	unlockTxId := lockInfo.UnlockTxId.String()
	baseParams := &NotoPrepareUnlockParams{
		TxId:         &req.Transaction.TransactionId,
		LockId:       &params.LockID,
		UnlockTxId:   &unlockTxId,
		LockedInputs: endorsableStateIDs(lockedInputs),
		UnlockHash:   pldtypes.Bytes32(unlockHash),
		Signature:    sender.Payload,
		Data:         data,
	}
	paramsJSON, err := json.Marshal(baseParams)
	if err != nil {
		return nil, err
	}
	interfaceABI := h.noto.getInterfaceABI(types.NotoVariantDefault)
	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()["prepareUnlock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *prepareBurnUnlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	params := tx.Params.(*types.PrepareBurnUnlockParams)

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	hookParams := &PrepareBurnUnlockHookParams{
		Sender: fromAddress,
		LockId: params.LockID,
		From:   fromAddress,
		Amount: params.Amount,
		Data:   params.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onPrepareBurnUnlock"],
		hookParams,
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

func (h *prepareBurnUnlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare()
	}

	return baseTransaction.prepare()
}
