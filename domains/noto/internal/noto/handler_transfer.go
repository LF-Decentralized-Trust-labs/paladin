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

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type transferHandler struct {
	transferCommon
}

func (h *transferHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var transferParams types.TransferParams
	err := json.Unmarshal([]byte(params), &transferParams)
	if err == nil {
		err = h.validateTransferParams(ctx, transferParams.To, transferParams.Amount)
	}
	return &transferParams, err
}

func (h *transferHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)
	return h.initTransfer(ctx, tx, tx.Transaction.From, params.To)
}

func (h *transferHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)
	return h.assembleTransfer(ctx, tx, req, tx.Transaction.From, params.To, params.Amount, params.Data)
}

func (h *transferHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	return h.endorseTransfer(ctx, tx, req, tx.Transaction.From)
}

func (h *transferHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.TransferParams)
	return h.prepareTransfer(ctx, tx, req, tx.Transaction.From, params.To, params.Amount, params.Data)
}
