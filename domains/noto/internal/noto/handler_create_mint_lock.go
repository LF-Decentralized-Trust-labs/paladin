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

type createMintLockHandler struct {
	noto *Noto
}

func (h *createMintLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	if config.IsV0() {
		return nil, i18n.NewError(ctx, msgs.MsgUnknownDomainVariant, "createMintLock is not supported in Noto V0")
	}

	var mintLockParams types.CreateMintLockParams
	if err := json.Unmarshal([]byte(params), &mintLockParams); err != nil {
		return nil, err
	}
	if len(mintLockParams.Recipients) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "recipients")
	}
	for _, entry := range mintLockParams.Recipients {
		if entry.Amount == nil || entry.Amount.Int().Sign() != 1 {
			return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "recipient amount")
		}
	}
	return &mintLockParams, nil
}

func (h *createMintLockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction, from string) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.RestrictMint && from != tx.DomainConfig.NotaryLookup {
		return i18n.NewError(ctx, msgs.MsgMintOnlyNotary, tx.DomainConfig.NotaryLookup, from)
	}
	if !*tx.DomainConfig.Options.Basic.AllowLock {
		return i18n.NewError(ctx, msgs.MsgLockNotAllowed)
	}
	return nil
}

func (h *createMintLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.CreateMintLockParams)
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	lookups := []string{notary, tx.Transaction.From}
	for _, entry := range params.Recipients {
		lookups = append(lookups, entry.To)
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(lookups...),
	}, nil
}

func (h *createMintLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.CreateMintLockParams)
	notary := tx.DomainConfig.NotaryLookup
	unlockTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	notaryAddress, err := h.noto.findEthAddressVerifier(ctx, "notary", notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	// Pre-compute the lockId as it will be generated on the smart contract
	var senderAddress *pldtypes.EthAddress
	contractAddress := (*pldtypes.EthAddress)(tx.ContractAddress)
	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() &&
		tx.DomainConfig.Options.Hooks != nil &&
		tx.DomainConfig.Options.Hooks.PublicAddress != nil {
		senderAddress = tx.DomainConfig.Options.Hooks.PublicAddress
	} else {
		senderAddress = notaryAddress
	}
	lockID, err := h.noto.computeLockId(ctx, contractAddress, senderAddress, tx.Transaction.TransactionId)
	if err != nil {
		return nil, err
	}

	outputs := &preparedOutputs{}
	for _, entry := range params.Recipients {
		toAddress, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		recipientOutputs, err := h.noto.prepareOutputs(toAddress, entry.Amount, []string{notary, entry.To})
		if err != nil {
			return nil, err
		}
		outputs.coins = append(outputs.coins, recipientOutputs.coins...)
		outputs.states = append(outputs.states, recipientOutputs.states...)
	}

	infoStates, err := h.noto.prepareInfo(params.Data, tx.DomainConfig.Variant, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	lockState, err := h.noto.prepareLockInfo(lockID, fromAddress, nil, &unlockTxId, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	infoStates = append(infoStates, lockState)

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.InfoStates = infoStates
	assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, outputs.states...)

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, nil, outputs.coins)
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

func (h *createMintLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkAllowed(ctx, tx, req.Transaction.From); err != nil {
		return nil, err
	}

	allOutputs := h.noto.filterSchema(req.Info, []string{h.noto.coinSchema.Id})
	outputs, err := h.noto.parseCoinList(ctx, "output", allOutputs)
	if err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, nil, outputs.lockedCoins, outputs.coins)
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

func (h *createMintLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	params := tx.Params.(*types.CreateMintLockParams)

	// Extract lockID from info states
	lockInfo, err := h.noto.extractLockInfo(ctx, req)
	if err != nil {
		return nil, err
	}
	lockID := lockInfo.LockID
	unlockTxId := lockInfo.UnlockTxId.String()

	outputs := h.noto.filterSchema(req.InfoStates, []string{h.noto.coinSchema.Id})
	unlockHash, err := h.noto.unlockHashFromStates(ctx, tx.ContractAddress, nil, nil, outputs, params.Data)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}

	baseParams := &NotoPrepareUnlockParams{
		TxId:         &req.Transaction.TransactionId,
		LockId:       &lockID,
		UnlockTxId:   &unlockTxId,
		LockedInputs: []string{},
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

func (h *createMintLockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.CreateMintLockParams)

	// Extract lockID from info states
	lockInfo, err := h.noto.extractLockInfo(ctx, req)
	if err != nil {
		return nil, err
	}
	lockID := lockInfo.LockID

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	recipients := make([]*ResolvedUnlockRecipient, len(inParams.Recipients))
	for i, entry := range inParams.Recipients {
		to, err := h.noto.findEthAddressVerifier(ctx, "to", entry.To, req.ResolvedVerifiers)
		if err != nil {
			return nil, err
		}
		recipients[i] = &ResolvedUnlockRecipient{To: to, Amount: entry.Amount}
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &UnlockHookParams{
		Sender:     fromAddress,
		LockID:     lockID,
		Recipients: recipients,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onCreateMintLock"],
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

func (h *createMintLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
