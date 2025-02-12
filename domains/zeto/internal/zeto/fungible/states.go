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

package fungible

import (
	"context"
	"encoding/json"
	"math/big"
	"math/rand/v2"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var MAX_INPUT_COUNT = 10
var MAX_OUTPUT_COUNT = 10

func makeCoin(stateData string) (*types.ZetoCoin, error) {
	coin := &types.ZetoCoin{}
	err := json.Unmarshal([]byte(stateData), &coin)
	return coin, err
}

func makeNewState(ctx context.Context, coinSchema *prototk.StateSchema, useNullifiers bool, coin *types.ZetoCoin, name, owner string) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	hash, err := coin.Hash(ctx)
	if err != nil {
		return nil, err
	}
	hashStr := common.HexUint256To32ByteHexString(hash)
	newState := &pb.NewState{
		Id:               &hashStr,
		SchemaId:         coinSchema.Id,
		StateDataJson:    string(coinJSON),
		DistributionList: []string{owner},
	}
	if useNullifiers {
		newState.NullifierSpecs = []*pb.NullifierSpec{
			{
				Party:        owner,
				Algorithm:    getAlgoZetoSnarkBJJ(name),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				PayloadType:  zetosignerapi.PAYLOAD_DOMAIN_ZETO_NULLIFIER,
			},
		}
	}
	return newState, nil
}

func prepareInputsForTransfer(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *pb.StateSchema, useNullifiers bool, stateQueryContext, senderKey string, params []*types.FungibleTransferParamEntry) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, *big.Int, error) {
	expectedTotal := big.NewInt(0)
	for _, param := range params {
		expectedTotal = expectedTotal.Add(expectedTotal, param.Amount.Int())
	}

	return buildInputsForExpectedTotal(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, senderKey, expectedTotal)
}

func buildInputsForExpectedTotal(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *pb.StateSchema, useNullifiers bool, stateQueryContext, senderKey string, expectedTotal *big.Int) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	coins := []*types.ZetoCoin{}
	for {
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", senderKey)

		if lastStateTimestamp > 0 {
			queryBuilder.GreaterThan(".created", lastStateTimestamp)
		}
		states, err := findAvailableStates(ctx, callbacks, coinSchema, useNullifiers, stateQueryContext, queryBuilder.Query().String())
		if err != nil {
			return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorQueryAvailCoins, err)
		}
		if len(states) == 0 {
			return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgInsufficientFunds, total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.CreatedAt
			coin, err := makeCoin(state.DataJson)
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
				remainder := big.NewInt(0).Sub(total, expectedTotal)
				return coins, stateRefs, total, remainder, nil
			}
			if len(stateRefs) >= MAX_INPUT_COUNT {
				return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgMaxCoinsReached, MAX_INPUT_COUNT)
			}
		}
	}
}

func prepareOutputsForTransfer(ctx context.Context, useNullifiers bool, params []*types.FungibleTransferParamEntry, resolvedVerifiers []*pb.ResolvedVerifier, coinSchema *prototk.StateSchema, name string) ([]*types.ZetoCoin, []*pb.NewState, error) {
	var coins []*types.ZetoCoin
	var newStates []*pb.NewState
	for _, param := range params {
		resolvedRecipient := domain.FindVerifier(param.To, getAlgoZetoSnarkBJJ(name), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, resolvedVerifiers)
		if resolvedRecipient == nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorResolveVerifier, param.To)
		}
		recipientKey, err := common.LoadBabyJubKey([]byte(resolvedRecipient.Verifier))
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorLoadOwnerPubKey, err)
		}

		salt := crypto.NewSalt()
		compressedKeyStr := zetosigner.EncodeBabyJubJubPublicKey(recipientKey)
		newCoin := &types.ZetoCoin{
			Salt:   (*tktypes.HexUint256)(salt),
			Owner:  tktypes.MustParseHexBytes(compressedKeyStr),
			Amount: param.Amount,
		}

		newState, err := makeNewState(ctx, coinSchema, useNullifiers, newCoin, name, param.To)
		if err != nil {
			return nil, nil, i18n.NewError(ctx, msgs.MsgErrorCreateNewState, err)
		}
		coins = append(coins, newCoin)
		newStates = append(newStates, newState)
	}
	return coins, newStates, nil
}

func findAvailableStates(ctx context.Context, callbacks plugintk.DomainCallbacks, coinSchema *prototk.StateSchema, useNullifiers bool, stateQueryContext, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		StateQueryContext: stateQueryContext,
		SchemaId:          coinSchema.Id,
		QueryJson:         query,
		UseNullifiers:     &useNullifiers,
	}
	res, err := callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}

func randomSlot(size int) int {
	return rand.IntN(size)
}
