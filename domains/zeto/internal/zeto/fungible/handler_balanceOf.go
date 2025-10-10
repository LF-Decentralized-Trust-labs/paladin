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

package fungible

import (
	"context"
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

var _ types.DomainCallHandler = &balanceOfHandler{}

type balanceOfHandler struct {
	baseHandler
	callbacks plugintk.DomainCallbacks
}

func NewBalanceOfHandler(name string, callbacks plugintk.DomainCallbacks, coinSchema *pb.StateSchema) *balanceOfHandler {
	return &balanceOfHandler{
		baseHandler: baseHandler{
			name: name,
			stateSchemas: &common.StateSchemas{
				CoinSchema: coinSchema,
			},
		},
		callbacks: callbacks,
	}
}

func (h *balanceOfHandler) ValidateParams(ctx context.Context, config *types.DomainInstanceConfig, param string) (interface{}, error) {
	var balanceOfParam types.FungibleBalanceOfParam
	if err := json.Unmarshal([]byte(param), &balanceOfParam); err != nil {
		return nil, err
	}

	if err := validateBalanceOfParams(ctx, &balanceOfParam); err != nil {
		return nil, err
	}

	return &balanceOfParam, nil
}

func (h *balanceOfHandler) InitCall(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitCallRequest) (*pb.InitCallResponse, error) {
	param := tx.Params.(*types.FungibleBalanceOfParam)

	res := &pb.InitCallResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:       param.Account,
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}

	return res, nil
}

func (h *balanceOfHandler) ExecCall(ctx context.Context, tx *types.ParsedTransaction, req *pb.ExecCallRequest) (*pb.ExecCallResponse, error) {

	param := tx.Params.(*types.FungibleBalanceOfParam)
	resolvedAccount := domain.FindVerifier(param.Account, h.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.ResolvedVerifiers)
	if resolvedAccount == nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, param.Account)
	}
	useNullifiers := common.IsNullifiersToken(tx.DomainConfig.TokenName)
	totalStates, totalBalance, overflow, err := getAccountBalance(ctx, h.callbacks, h.stateSchemas.CoinSchema, useNullifiers, req.StateQueryContext, resolvedAccount.Verifier)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorGetAccountBalance, param.Account)
	}
	// Format balance as JSON string
	balanceResult := types.BalanceOfResult{
		TotalBalance: (*pldtypes.HexUint256)(totalBalance),
		TotalStates:  pldtypes.Uint64ToUint256(uint64(totalStates)),
		Overflow:     overflow,
	}
	balanceJson, err := json.Marshal(balanceResult)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgErrorGetAccountBalance, "failed to marshal balance result")
	}
	return &pb.ExecCallResponse{
		ResultJson: string(balanceJson),
	}, nil
}
