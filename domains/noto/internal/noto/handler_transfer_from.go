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
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
)

type transferFromHandler struct {
	transferCommon
}

func (h *transferFromHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var transferFromParams types.TransferFromParams
	err := json.Unmarshal([]byte(params), &transferFromParams)
	if err == nil {
		err = h.validateTransferParams(ctx, transferFromParams.To, transferFromParams.Amount)
		if err == nil && transferFromParams.From == "" {
			err = i18n.NewError(ctx, msgs.MsgParameterRequired, "from")
		}
	}
	return &transferFromParams, err
}

func (h *transferFromHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	params := tx.Params.(*types.TransferFromParams)
	if tx.DomainConfig.NotaryMode == types.NotaryModeBasic.Enum() {
		return nil, i18n.NewError(ctx, msgs.MsgTransferFromNotAllowed)
	}
	return h.initTransfer(ctx, tx, params.From, params.To)
}

func (h *transferFromHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.TransferFromParams)
	return h.assembleTransfer(ctx, tx, req, params.From, params.To, params.Amount, params.Data)
}

func (h *transferFromHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.TransferFromParams)
	return h.endorseTransfer(ctx, tx, req, params.From)
}

func (h *transferFromHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	params := tx.Params.(*types.TransferFromParams)
	return h.prepareTransfer(ctx, tx, req, params.From, params.To, params.Amount, params.Data)
}
