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
	"fmt"

	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type balanceOfHandler struct {
	noto *Noto
}

func (h *balanceOfHandler) ValidateParams(ctx context.Context, config *types.NotoParsedConfig, params string) (interface{}, error) {
	var balanceOfParam types.BalanceOfParam
	if err := json.Unmarshal([]byte(params), &balanceOfParam); err != nil {
		return nil, err
	}
	if balanceOfParam.Account == "" {
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "Account")
	}
	return &balanceOfParam, nil
}

func (h *balanceOfHandler) InitCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	param := tx.Params.(*types.BalanceOfParam)

	notary := tx.DomainConfig.NotaryLookup

	return &prototk.InitCallResponse{
		RequiredVerifiers: h.noto.ethAddressVerifiers(notary, param.Account),
	}, nil
}

func (h *balanceOfHandler) ExecCall(ctx context.Context, tx *types.ParsedTransaction, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {

	param := tx.Params.(*types.BalanceOfParam)

	accountAddress, err := h.noto.findEthAddressVerifier(ctx, "account", param.Account, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}
	balance, _, err := h.noto.getAccountBalance(ctx, req.StateQueryContext, accountAddress)
	if err != nil {
		// don't think we are wrapping errors currently, throwing this out here see if folks are fond of it.
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorGetAccountBalance, param.Account)
	}
	// Format balance as JSON string
	balanceJson := fmt.Sprintf(`{"balance":"%s"}`, balance)
	return &prototk.ExecCallResponse{
		ResultJson: balanceJson,
	}, nil
}
