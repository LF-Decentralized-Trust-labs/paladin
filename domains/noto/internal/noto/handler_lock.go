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

type lockHandler struct {
	noto *Noto
}

func (h *lockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var lockParams types.LockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, err
	}
	if lockParams.Amount == nil || lockParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &lockParams, nil
}

func (h *lockHandler) checkAllowed(ctx context.Context, tx *types.ParsedTransaction) error {
	if tx.DomainConfig.NotaryMode != types.NotaryModeBasic.Enum() {
		return nil
	}
	if *tx.DomainConfig.Options.Basic.AllowLock {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgLockNotAllowed)
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	notary := tx.DomainConfig.NotaryLookup
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	return &prototk.InitTransactionResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, tx.Transaction.From),
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

	inputStates, revert, err := h.noto.prepareInputs(ctx, req.StateQueryContext, fromAddress, params.Amount)
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

	lockID := pldtypes.RandBytes32()
	lockedOutputStates, err := h.noto.prepareLockedOutputs(lockID, fromAddress, params.Amount, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}

	unlockedOutputStates := &preparedOutputs{}
	if inputStates.total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(inputStates.total, params.Amount.Int())
		returnedStates, err := h.noto.prepareOutputs(fromAddress, (*pldtypes.HexUint256)(remainder), []string{notary, tx.Transaction.From})
		if err != nil {
			return nil, err
		}
		unlockedOutputStates.coins = append(unlockedOutputStates.coins, returnedStates.coins...)
		unlockedOutputStates.states = append(unlockedOutputStates.states, returnedStates.states...)
	}

	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	lockState, err := h.noto.prepareLockInfo(lockID, fromAddress, nil, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	infoStates = append(infoStates, lockState)

	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, inputStates.coins, unlockedOutputStates.coins, lockedOutputStates.coins)
	if err != nil {
		return nil, err
	}

	var outputStates []*prototk.NewState
	outputStates = append(outputStates, lockedOutputStates.states...)
	outputStates = append(outputStates, unlockedOutputStates.states...)

	attestation := []*prototk.AttestationRequest{
		// Sender confirms the initial request with a signature
		{
			Name:            "sender",
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
			InputStates:  inputStates.states,
			OutputStates: outputStates,
			InfoStates:   infoStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *lockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	if err := h.checkAllowed(ctx, tx); err != nil {
		return nil, err
	}

	inputs, err := h.noto.parseCoinList(ctx, "input", req.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := h.noto.parseCoinList(ctx, "output", req.Outputs)
	if err != nil {
		return nil, err
	}

	// Validate the amounts, and sender's ownership of the inputs and locked outputs
	if err := h.noto.validateLockAmounts(ctx, inputs, outputs); err != nil {
		return nil, err
	}
	if err := h.noto.validateOwners(ctx, tx.Transaction.From, req, inputs.coins, inputs.states); err != nil {
		return nil, err
	}
	if err := h.noto.validateLockOwners(ctx, tx.Transaction.From, req.ResolvedVerifiers, outputs.lockedCoins, outputs.lockedStates); err != nil {
		return nil, err
	}

	// Notary checks the signature from the sender, then submits the transaction
	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, inputs.coins, outputs.coins, outputs.lockedCoins)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateSignature(ctx, "sender", req.Signatures, encodedLock); err != nil {
		return nil, err
	}
	return &prototk.EndorseTransactionResponse{
		EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *lockHandler) baseLedgerInvoke(ctx context.Context, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inputs := req.InputStates
	outputs, lockedOutputs := h.noto.splitStates(req.OutputStates)

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	lockSignature := domain.FindAttestation("sender", req.AttestationResult)
	if lockSignature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}
	params := &NotoLockParams{
		TxId:          req.Transaction.TransactionId,
		Inputs:        endorsableStateIDs(inputs),
		Outputs:       endorsableStateIDs(outputs),
		LockedOutputs: endorsableStateIDs(lockedOutputs),
		Signature:     lockSignature.Payload,
		Data:          data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: interfaceBuild.ABI.Functions()["lock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *lockHandler) hookInvoke(ctx context.Context, lockID pldtypes.Bytes32, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest, baseTransaction *TransactionWrapper) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.LockParams)

	fromAddress, err := h.noto.findEthAddressVerifier(ctx, "from", tx.Transaction.From, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	encodedCall, err := baseTransaction.encode(ctx)
	if err != nil {
		return nil, err
	}
	params := &LockHookParams{
		Sender: fromAddress,
		LockID: lockID,
		From:   fromAddress,
		Amount: inParams.Amount,
		Data:   inParams.Data,
		Prepared: PreparedTransaction{
			ContractAddress: (*pldtypes.EthAddress)(tx.ContractAddress),
			EncodedCall:     encodedCall,
		},
	}

	transactionType, functionABI, paramsJSON, err := h.noto.wrapHookTransaction(
		tx.DomainConfig,
		hooksBuild.ABI.Functions()["onLock"],
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

func (h *lockHandler) extractLockID(ctx context.Context, req *prototk.PrepareTransactionRequest) (pldtypes.Bytes32, error) {
	lockStates := h.noto.filterSchema(req.InfoStates, []string{h.noto.lockInfoSchema.Id})
	if len(lockStates) == 1 {
		lock, err := h.noto.unmarshalLock(lockStates[0].StateDataJson)
		if err != nil {
			return pldtypes.Bytes32{}, err
		}
		return lock.LockID, nil
	}
	return pldtypes.Bytes32{}, i18n.NewError(ctx, msgs.MsgLockIDNotFound)
}

func (h *lockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	lockID, err := h.extractLockID(ctx, req)
	if err != nil {
		return nil, err
	}

	endorsement := domain.FindAttestation("notary", req.AttestationResult)
	if endorsement == nil || endorsement.Verifier.Lookup != tx.DomainConfig.NotaryLookup {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "notary")
	}

	baseTransaction, err := h.baseLedgerInvoke(ctx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryMode == types.NotaryModeHooks.Enum() {
		hookTransaction, err := h.hookInvoke(ctx, lockID, tx, req, baseTransaction)
		if err != nil {
			return nil, err
		}
		return hookTransaction.prepare(nil)
	}

	return baseTransaction.prepare(nil)
}
