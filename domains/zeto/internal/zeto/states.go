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
	"strconv"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

var INPUT_COUNT = 2
var OUTPUT_COUNT = 2

func (n *Zeto) makeCoin(stateData string) (*types.ZetoCoin, error) {
	coin := &types.ZetoCoin{}
	err := json.Unmarshal([]byte(stateData), &coin)
	return coin, err
}

func (z *Zeto) makeNewState(coin *types.ZetoCoin) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &pb.NewState{
		SchemaId:      z.coinSchema.Id,
		StateDataJson: string(coinJSON),
	}, nil
}

func (z *Zeto) prepareInputs(ctx context.Context, owner string, amount *ethtypes.HexInteger) ([]*types.ZetoCoin, []*pb.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	coins := []*types.ZetoCoin{}
	for {
		// Simple oldest coin first algorithm
		// TODO: make this configurable
		// TODO: why is filters.QueryJSON not a public interface?
		query := map[string]interface{}{
			"limit": 10,
			"sort":  []string{".created"},
			"eq": []map[string]string{{
				"field": "owner",
				"value": owner,
			}},
		}
		if lastStateTimestamp > 0 {
			query["gt"] = []map[string]string{{
				"field": ".created",
				"value": strconv.FormatInt(lastStateTimestamp, 10),
			}}
		}
		queryJSON, err := json.Marshal(query)
		if err != nil {
			return nil, nil, nil, err
		}

		states, err := z.findAvailableStates(ctx, string(queryJSON))
		if err != nil {
			return nil, nil, nil, err
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
			total = total.Add(total, coin.Amount.BigInt())
			stateRefs = append(stateRefs, &pb.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})
			coins = append(coins, coin)
			if total.Cmp(amount.BigInt()) >= 0 {
				return coins, stateRefs, total, nil
			}
			if len(stateRefs) >= INPUT_COUNT {
				return nil, nil, nil, fmt.Errorf("could not find suitable coins")
			}
		}
	}
}

func (z *Zeto) addHash(newCoin *types.ZetoCoin, ownerKey *babyjub.PublicKey) error {
	commitment, err := poseidon.Hash([]*big.Int{
		newCoin.Amount.BigInt(),
		newCoin.Salt.BigInt(),
		ownerKey.X,
		ownerKey.Y,
	})
	if err != nil {
		return err
	}
	newCoin.Hash = (*ethtypes.HexInteger)(commitment)
	return nil
}

func (z *Zeto) prepareOutputs(owner string, ownerKey *babyjub.PublicKey, amount *ethtypes.HexInteger) ([]*types.ZetoCoin, []*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	salt := crypto.NewSalt()
	keyCompressed := ownerKey.Compress()
	newCoin := &types.ZetoCoin{
		Salt:     (*ethtypes.HexInteger)(salt),
		Owner:    owner,
		OwnerKey: keyCompressed[:],
		Amount:   amount,
	}
	if err := z.addHash(newCoin, ownerKey); err != nil {
		return nil, nil, err
	}

	newState, err := z.makeNewState(newCoin)
	return []*types.ZetoCoin{newCoin}, []*pb.NewState{newState}, err
}

func (z *Zeto) findAvailableStates(ctx context.Context, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		SchemaId:  z.coinSchema.Id,
		QueryJson: query,
	}
	res, err := z.Callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}
