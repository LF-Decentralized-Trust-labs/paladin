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
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// This handler is identical to the unlockHandler, except for the Prepare() method
type prepareUnlockHandler struct {
	unlockHandler
}

func (h *prepareUnlockHandler) baseLedgerInvoke(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*TransactionWrapper, error) {
	inParams := tx.Params.(*types.UnlockParams)

	lockedInput := req.InputStates[0]
	unlockedOutput := req.OutputStates[0]
	lockedOutputs := req.OutputStates[1:]

	txhash, err := h.noto.encodeUnlockMasked(ctx, tx.ContractAddress,
		[]*prototk.EndorsableState{lockedInput},
		lockedOutputs,
		[]*prototk.EndorsableState{unlockedOutput},
		inParams.Data,
	)
	if err != nil {
		return nil, err
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
	params := &NotoPrepareUnlockParams{
		LockID:    inParams.LockID,
		TXHash:    tktypes.HexBytes(txhash),
		Signature: unlockSignature.Payload,
		Data:      data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &TransactionWrapper{
		functionABI: h.noto.contractABI.Functions()["prepareUnlock"],
		paramsJSON:  paramsJSON,
	}, nil
}

func (h *prepareUnlockHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	baseTransaction, err := h.baseLedgerInvoke(ctx, tx, req)
	if err != nil {
		return nil, err
	}

	if tx.DomainConfig.NotaryType == types.NotaryTypePente {
		return nil, fmt.Errorf("Pente notary type not supported")
	}

	return baseTransaction.prepare(nil)
}
