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
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

type prepareUnlockHandler struct {
	unlockCommon
}

func (h *prepareUnlockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var unlockParams types.UnlockParams
	err := json.Unmarshal([]byte(params), &unlockParams)
	if err == nil {
		err = h.validateParams(ctx, &unlockParams)
	}
	return &unlockParams, err
}

func (h *prepareUnlockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	return h.init(ctx, tx, params)
}

func (h *prepareUnlockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	notary := tx.DomainConfig.NotaryLookup
	spendTxId := pldtypes.Bytes32UUIDFirst16(uuid.New())

	res, states, err := h.assembleStates(ctx, tx, &spendTxId, params, req)
	if err != nil || res.AssemblyResult != prototk.AssembleTransactionResponse_OK {
		return res, err
	}

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", params.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	cancelOutputs, err := h.noto.prepareOutputs(fromAddress, (*pldtypes.HexUint256)(states.lockedInputs.total), []string{notary, params.From})
	if err != nil {
		return nil, err
	}

	assembledTransaction := &prototk.AssembledTransaction{}
	assembledTransaction.ReadStates = states.lockedInputs.states
	assembledTransaction.InfoStates = states.info
	assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, states.outputs.states...)
	assembledTransaction.InfoStates = append(assembledTransaction.InfoStates, cancelOutputs.states...)

	encodedUnlock, err := h.noto.encodeUnlock(ctx, tx.ContractAddress, states.lockedInputs.coins, nil, states.outputs.coins)
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

func (h *prepareUnlockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.UnlockParams)
	lockedInputs := req.Reads
	allOutputs := h.noto.filterSchema(req.Info, []string{h.noto.coinSchema.Id})
	spendOutputs, cancelOutputs, err := h.noto.splitUnlockOutputs(ctx, allOutputs)
	if err != nil {
		return nil, err
	}

	parsedInputs, err := h.noto.parseCoinList(ctx, "input", lockedInputs)
	if err != nil {
		return nil, err
	}
	parsedSpendOutputs, err := h.noto.parseCoinList(ctx, "output", spendOutputs)
	if err != nil {
		return nil, err
	}
	parsedCancelOutputs, err := h.noto.parseCoinList(ctx, "output", cancelOutputs)
	if err != nil {
		return nil, err
	}

	return h.endorse(ctx, tx, params, req, parsedInputs, parsedSpendOutputs, parsedCancelOutputs)
}

func (h *prepareUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)
	lockedInputs := req.ReadStates
	allOutputs := h.noto.filterSchema(req.InfoStates, []string{h.noto.coinSchema.Id})
	spendOutputs, cancelOutputs, err := h.noto.splitUnlockOutputs(ctx, allOutputs)
	if err != nil {
		return nil, err
	}

	var spendTxId pldtypes.Bytes32
	lockInfoStates := h.noto.filterSchema(req.InfoStates, []string{h.noto.lockInfoSchema.Id})
	if len(lockInfoStates) > 0 {
		lock, err := h.noto.unmarshalLock(lockInfoStates[0].StateDataJson)
		if err == nil {
			spendTxId = lock.SpendTxId
		}
	}

	spendHash, err := h.noto.unlockHashFromStates(ctx, tx.ContractAddress, spendTxId.String(), lockedInputs, spendOutputs, inParams.Data)
	if err != nil {
		return nil, err
	}
	cancelHash, err := h.noto.unlockHashFromStates(ctx, tx.ContractAddress, spendTxId.String(), lockedInputs, cancelOutputs, inParams.Data)
	if err != nil {
		return nil, err
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	sender := domain.FindAttestation("sender", req.AttestationResult)
	if sender == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	lockOptions := types.NotoLockOptions{
		SpendTxId:  spendTxId,
		SpendHash:  pldtypes.Bytes32(spendHash),
		CancelHash: pldtypes.Bytes32(cancelHash),
	}
	lockOptionsJSON, err := json.Marshal(lockOptions)
	if err != nil {
		return nil, err
	}
	lockOptionsEncoded, err := types.NotoLockOptionsABI.EncodeABIDataJSONCtx(ctx, lockOptionsJSON)
	if err != nil {
		return nil, err
	}

	var interfaceABI abi.ABI
	var functionName string
	var paramsJSON []byte
	var txData pldtypes.HexBytes

	switch tx.DomainConfig.Variant {
	case types.NotoVariantDefault:
		txData, err = h.noto.encodeTransactionData(ctx, req.InfoStates)
		if err == nil {
			interfaceABI = h.noto.getInterfaceABI(types.NotoVariantDefault)
			functionName = "updateLock"
			params := &UpdateLockParams{
				LockID: inParams.LockID,
				Params: NotoUpdateLockParams{
					TxId:         req.Transaction.TransactionId,
					LockedInputs: endorsableStateIDs(lockedInputs),
					Proof:        sender.Payload,
					Options:      lockOptionsEncoded,
				},
				Data: txData,
			}
			paramsJSON, err = json.Marshal(params)
		}
	default:
		// We must use the legacy encoding for the transaction data, because there is no other place
		// to pass in the transaction ID on this method
		txData, err = h.noto.encodeTransactionData_V0(ctx, req.Transaction.TransactionId, req.InfoStates)
		if err == nil {
			interfaceABI = h.noto.getInterfaceABI(types.NotoVariantLegacy)
			functionName = "prepareUnlock"
			params := &NotoPrepareUnlock_V0_Params{
				LockedInputs: endorsableStateIDs(lockedInputs),
				UnlockHash:   spendHash.String(),
				Signature:    sender.Payload,
				Data:         txData,
			}
			paramsJSON, err = json.Marshal(params)
		}
	}
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: interfaceABI.Functions()[functionName],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *prepareUnlockHandler) hookInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

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
		LockID:     inParams.LockID,
		Recipients: recipients,
		Data:       inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onPrepareUnlock"],
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

func (h *prepareUnlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
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
