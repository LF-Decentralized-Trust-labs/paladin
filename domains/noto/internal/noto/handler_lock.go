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
	var transferParams types.LockParams
	if err := json.Unmarshal([]byte(params), &transferParams); err != nil {
		return nil, err
	}
	if transferParams.Delegate.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "delegate")
	}
	if transferParams.Amount == nil || transferParams.Amount.Int().Sign() != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgParameterGreaterThanZero, "amount")
	}
	return &transferParams, nil
}

func (h *lockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
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
		},
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
		ID:     tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  fromAddress,
		Amount: params.Amount,
	}
	// A single output coin, unlocked, with original owner
	// TODO: make this configurable
	revertCoin := &types.NotoCoin{
		Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  fromAddress,
		Amount: params.Amount,
	}

	lockedCoinState, err := h.noto.makeNewLockedCoinState(lockedCoin, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	revertState, err := h.noto.makeNewCoinState(revertCoin, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}

	outputCoins := []*types.NotoCoin{}
	outputStates := []*prototk.NewState{lockedCoinState, revertState}

	if total.Cmp(params.Amount.Int()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.Int())
		returnedCoins, returnedStates, err := h.noto.prepareOutputs(fromAddress, (*tktypes.HexUint256)(remainder), []string{notary, tx.Transaction.From})
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}

	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, inputCoins, outputCoins, lockedCoin, revertCoin)
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
	lockedState := req.Outputs[0]
	revertState := req.Outputs[1]
	remainderStates := req.Outputs[2:]

	if lockedState.SchemaId != h.noto.LockedCoinSchemaID() {
		return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, lockedState.SchemaId)
	}
	lockedCoin, err := h.noto.unmarshalLockedCoin(lockedState.StateDataJson)
	if err != nil {
		return nil, err
	}

	if revertState.SchemaId != h.noto.CoinSchemaID() {
		return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, revertState.SchemaId)
	}
	revertCoin, err := h.noto.unmarshalCoin(revertState.StateDataJson)
	if err != nil {
		return nil, err
	}

	coins, err := h.noto.gatherCoins(ctx, req.Inputs, remainderStates)
	if err != nil {
		return nil, err
	}
	if err := h.noto.validateLockAmounts(ctx, coins, lockedCoin, revertCoin); err != nil {
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
	if !revertCoin.Owner.Equals(fromAddress) {
		return nil, i18n.NewError(ctx, msgs.MsgStateWrongOwner, revertState.Id, tx.Transaction.From)
	}

	if req.EndorsementRequest.Name == "notary" {
		// Notary checks the signature from the sender, then submits the transaction
		if err := h.noto.validateLockSignature(ctx, tx, req, coins, lockedCoin, revertCoin); err != nil {
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

	lockedOutput := req.OutputStates[0].Id
	revertOutput := req.OutputStates[1].Id
	remainderOutputs := make([]string, len(req.OutputStates)-2)
	for i, state := range req.OutputStates[2:] {
		remainderOutputs[i] = state.Id
	}

	// Include the signature from the sender
	// This is not verified on the base ledger, but can be verified by anyone with the unmasked state data
	signature := domain.FindAttestation("sender", req.AttestationResult)
	if signature == nil {
		return nil, i18n.NewError(ctx, msgs.MsgAttestationNotFound, "sender")
	}

	data, err := h.noto.encodeTransactionData(ctx, req.Transaction, req.InfoStates)
	if err != nil {
		return nil, err
	}
	params := &NotoTransferAndLockParams{
		Inputs:          inputs,
		UnlockedOutputs: remainderOutputs,
		LockedOutput:    lockedOutput,
		Lock: LockInput{
			RevertOutput: revertOutput,
			Delegate:     inputParams.Delegate,
		},
		Signature: signature.Payload,
		Data:      data,
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
