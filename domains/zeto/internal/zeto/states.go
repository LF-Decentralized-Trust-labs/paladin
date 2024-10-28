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

package zeto

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var MAX_INPUT_COUNT = 10
var MAX_OUTPUT_COUNT = 10

func getStateSchemas(ctx context.Context) ([]string, error) {
	var schemas []string
	coinJSON, err := json.Marshal(types.ZetoCoinABI)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalZetoCoinSchemaAbi, err)
	}
	schemas = append(schemas, string(coinJSON))

	smtRootJSON, err := json.Marshal(smt.MerkleTreeRootABI)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalMerkleTreeRootSchemaAbi, err)
	}
	schemas = append(schemas, string(smtRootJSON))

	smtNodeJSON, err := json.Marshal(smt.MerkleTreeNodeABI)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMarshalMerkleTreeNodeSchemaAbi, err)
	}
	schemas = append(schemas, string(smtNodeJSON))

	return schemas, nil
}

func (n *Zeto) makeCoin(stateData string) (*types.ZetoCoin, error) {
	coin := &types.ZetoCoin{}
	err := json.Unmarshal([]byte(stateData), &coin)
	return coin, err
}

func (z *Zeto) makeNewState(ctx context.Context, coin *types.ZetoCoin, owner string) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	hash, err := coin.Hash(ctx)
	if err != nil {
		return nil, err
	}
	hashStr := hash.String()
	return &pb.NewState{
		Id:               &hashStr,
		SchemaId:         z.coinSchema.Id,
		StateDataJson:    string(coinJSON),
		DistributionList: []string{owner},
	}, nil
}

func (z *Zeto) prepareInputs(ctx context.Context, stateQueryContext, sender string, params []*types.TransferParamEntry) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	coins := []*types.ZetoCoin{}

	expectedTotal := big.NewInt(0)
	for _, param := range params {
		expectedTotal = expectedTotal.Add(expectedTotal, param.Amount.Int())
	}

	for {
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", sender)

		if lastStateTimestamp > 0 {
			queryBuilder.GreaterThan(".created", lastStateTimestamp)
		}
		states, err := z.findAvailableStates(ctx, stateQueryContext, queryBuilder.Query().String())
		if err != nil {
			return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorQueryAvailCoins, err)
		}
		if len(states) == 0 {
			return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgInsufficientFunds, total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.CreatedAt
			coin, err := z.makeCoin(state.DataJson)
			if err != nil {
				return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgInvalidCoin, state.Id, err)
			}
			total = total.Add(total, coin.Amount.Int())
			stateRefs = append(stateRefs, &pb.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})
			coins = append(coins, coin)
			if total.Cmp(expectedTotal) >= 0 {
				remainder := total.Sub(total, expectedTotal)
				return coins, stateRefs, total, remainder, nil
			}
			if len(stateRefs) >= MAX_INPUT_COUNT {
				return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgMaxCoinsReached, MAX_INPUT_COUNT)
			}
		}
	}
}

func (z *Zeto) prepareOutputs(ctx context.Context, params []*types.TransferParamEntry, resolvedVerifiers []*pb.ResolvedVerifier) ([]*types.ZetoCoin, []*pb.NewState, error) {
	var coins []*types.ZetoCoin
	var newStates []*pb.NewState
	for _, param := range params {
		resolvedRecipient := domain.FindVerifier(param.To, z.getAlgoZetoSnarkBJJ(), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, resolvedVerifiers)
		if resolvedRecipient == nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, param.To)
		}
		recipientKey, err := loadBabyJubKey([]byte(resolvedRecipient.Verifier))
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}

		salt := crypto.NewSalt()
		compressedKeyStr := zetosigner.EncodeBabyJubJubPublicKey(recipientKey)
		newCoin := &types.ZetoCoin{
			Salt:     (*tktypes.HexUint256)(salt),
			Owner:    param.To,
			OwnerKey: tktypes.MustParseHexBytes(compressedKeyStr),
			Amount:   param.Amount,
		}

		newState, err := z.makeNewState(ctx, newCoin, param.To)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorCreateNewState, err)
		}
		coins = append(coins, newCoin)
		newStates = append(newStates, newState)
	}
	return coins, newStates, nil
}

func (z *Zeto) findAvailableStates(ctx context.Context, stateQueryContext, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		StateQueryContext: stateQueryContext,
		SchemaId:          z.coinSchema.Id,
		QueryJson:         query,
	}
	res, err := z.Callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}
