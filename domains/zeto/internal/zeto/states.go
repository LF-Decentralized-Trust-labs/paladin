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
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/smt"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var INPUT_COUNT = 2
var OUTPUT_COUNT = 2

func getStateSchemas() ([]string, error) {
	var schemas []string
	coinJSON, err := json.Marshal(types.ZetoCoinABI)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Zeto Coin schema abi. %s", err)
	}
	schemas = append(schemas, string(coinJSON))

	smtRootJSON, err := json.Marshal(smt.MerkleTreeRootABI)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Merkle Tree Root schema abi. %s", err)
	}
	schemas = append(schemas, string(smtRootJSON))

	smtNodeJSON, err := json.Marshal(smt.MerkleTreeNodeABI)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Merkle Tree Node schema abi. %s", err)
	}
	schemas = append(schemas, string(smtNodeJSON))

	return schemas, nil
}

func (n *Zeto) makeCoin(stateData string) (*types.ZetoCoin, error) {
	coin := &types.ZetoCoin{}
	err := json.Unmarshal([]byte(stateData), &coin)
	return coin, err
}

func (z *Zeto) makeNewState(coin *types.ZetoCoin, owner string) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	hash, err := coin.Hash()
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

func (z *Zeto) prepareInputs(ctx context.Context, stateQueryContext, owner string, amount *tktypes.HexUint256) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	coins := []*types.ZetoCoin{}
	for {
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", owner)

		if lastStateTimestamp > 0 {
			queryBuilder.GreaterThan(".created", lastStateTimestamp)
		}
		states, err := z.findAvailableStates(ctx, stateQueryContext, queryBuilder.Query().String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to query the state store for available coins. %s", err)
		}
		if len(states) == 0 {
			return nil, nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.StoredAt
			coin, err := z.makeCoin(state.DataJson)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("coin %s is invalid: %s", state.Id, err)
			}
			total = total.Add(total, coin.Amount.Int())
			stateRefs = append(stateRefs, &pb.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})
			coins = append(coins, coin)
			if total.Cmp(amount.Int()) >= 0 {
				return coins, stateRefs, total, nil
			}
			if len(stateRefs) >= INPUT_COUNT {
				return nil, nil, nil, fmt.Errorf("could not find suitable coins")
			}
		}
	}
}

func (z *Zeto) prepareOutputs(owner string, ownerKey *babyjub.PublicKey, amount *tktypes.HexUint256) ([]*types.ZetoCoin, []*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	salt := crypto.NewSalt()
	compressedKeyStr := zetosigner.EncodeBabyJubJubPublicKey(ownerKey)
	newCoin := &types.ZetoCoin{
		Salt:     (*tktypes.HexUint256)(salt),
		Owner:    owner,
		OwnerKey: tktypes.MustParseHexBytes(compressedKeyStr),
		Amount:   amount,
	}

	newState, err := z.makeNewState(newCoin, owner)
	return []*types.ZetoCoin{newCoin}, []*pb.NewState{newState}, err
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
