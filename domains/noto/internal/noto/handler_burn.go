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

type burnHandler struct {
	burnCommon
}

func (h *burnHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var burnParams types.BurnParams
	err := json.Unmarshal([]byte(params), &burnParams)
	if err == nil {
		err = h.validateBurnParams(ctx, burnParams.Amount)
	}
	return &burnParams, err
}

func (h *burnHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	return h.initBurn(ctx, tx, tx.Transaction.From)
}

func (h *burnHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.BurnParams)
	return h.assembleBurn(ctx, tx, req, tx.Transaction.From, params.Amount, params.Data)
}

func (h *burnHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.BurnParams)
	return h.endorseBurn(ctx, tx, req, tx.Transaction.From, params.Amount, params.Data)
}

func (h *burnHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.BurnParams)
	return h.prepareBurn(ctx, tx, req, tx.Transaction.From, params.Amount, params.Data)
}
