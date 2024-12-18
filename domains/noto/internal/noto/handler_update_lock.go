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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/signpayloads"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

type updateLockHandler struct {
	noto *Noto
}

func (h *updateLockHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var lockParams types.UpdateLockParams
	if err := json.Unmarshal([]byte(params), &lockParams); err != nil {
		return nil, err
	}
	if lockParams.ID.IsZero() {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "id")
	}
	return &lockParams, nil
}

func (h *updateLockHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.UpdateLockParams)
	notary := tx.DomainConfig.NotaryLookup

	if !tx.DomainConfig.AllowUpdateLock {
		return nil, i18n.NewError(ctx, msgs.MsgNoBurning)
	}

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

func (h *updateLockHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.UpdateLockParams)
	notary := tx.DomainConfig.NotaryLookup

	queryBuilder := query.NewQueryBuilder().Limit(1).Equal("id", params.ID)
	locked, err := h.noto.findLockedStates(ctx, req.StateQueryContext, queryBuilder.Query().String())
	if err != nil {
		return nil, err
	}
	if len(locked) != 1 {
		return nil, i18n.NewError(ctx, msgs.MsgLockNotFound, params.ID.String())
	}
	lockedCoinState := locked[0]
	lockedCoin, err := h.noto.unmarshalLockedCoin(lockedCoinState.DataJson)
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
			Amount: lockedCoin.Amount,
		}
		recipientStates[i], err = h.noto.makeNewCoinState(recipientCoins[i], []string{notary, tx.Transaction.From, recipient.Recipient})
		if err != nil {
			return nil, err
		}
	}

	infoStates, err := h.noto.prepareInfo(params.Data, []string{notary, tx.Transaction.From})
	if err != nil {
		return nil, err
	}
	readStates := []*prototk.StateRef{
		{Id: lockedCoinState.Id, SchemaId: lockedCoinState.SchemaId},
	}

	encodedLock, err := h.noto.encodeLock(ctx, tx.ContractAddress, lockedCoin, recipientCoins)
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
			ReadStates:   readStates,
			OutputStates: recipientStates,
			InfoStates:   infoStates,
		},
		AttestationPlan: attestation,
	}, nil
}

func (h *updateLockHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	lockedState := req.Reads[0]
	recipientStates := req.Outputs

	if !tx.DomainConfig.AllowUpdateLock {
		return nil, i18n.NewError(ctx, msgs.MsgNoBurning)
	}

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

	if err := h.noto.validateUpdateLockAmounts(ctx, lockedCoin, recipientCoins); err != nil {
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
		// Notary checks the signature from the sender, then submits the transaction
		if err := h.noto.validateLockSignature(ctx, tx, "sender", req, lockedCoin, recipientCoins); err != nil {
			return nil, err
		}
		return &prototk.EndorseTransactionResponse{
			EndorsementResult: prototk.EndorseTransactionResponse_ENDORSER_SUBMIT,
		}, nil
	}

	return nil, i18n.NewError(ctx, msgs.MsgUnrecognizedEndorsement, req.EndorsementRequest.Name)
}

func (h *updateLockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inputParams := tx.Params.(*types.UpdateLockParams)

	lockedOutput, err := tktypes.ParseBytes32Ctx(ctx, req.ReadStates[0].Id)
	if err != nil {
		return nil, err
	}

	lockOutcomes := make([]*LockOutcome, len(inputParams.Recipients))
	for i, state := range req.OutputStates {
		id, err := tktypes.ParseBytes32Ctx(ctx, state.Id)
		if err != nil {
			return nil, err
		}
		lockOutcomes[i] = &LockOutcome{
			Ref:   inputParams.Recipients[i].Ref,
			State: id,
		}
	}

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
	params := &NotoUpdateLockParams{
		Locked:    lockedOutput,
		Outcomes:  lockOutcomes,
		Signature: lockSignature.Payload,
		Data:      data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()["updateLock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *updateLockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	var err error
	var baseTransaction *TransactionWrapper

	baseTransaction, err = h.baseLedgerInvoke(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryType == types.NotaryTypePente {
		return nil, fmt.Errorf("not supported") // TODO
	}

	return baseTransaction.prepare(nil)
}
