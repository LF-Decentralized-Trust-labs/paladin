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
	"math/big"
	"strconv"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type NotoCoin struct {
	Salt   string              `json:"salt"`
	Owner  string              `json:"owner"`
	Amount ethtypes.HexInteger `json:"amount"`
}

var NotoCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
}

func (d *Noto) makeCoin(state *pb.StoredState) (*NotoCoin, error) {
	coin := &NotoCoin{}
	err := json.Unmarshal([]byte(state.DataJson), &coin)
	return coin, err
}

func (d *Noto) makeState(coin *NotoCoin) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &pb.NewState{
		SchemaId:      d.coinSchema.Id,
		StateDataJson: string(coinJSON),
	}, nil
}

func (d *Noto) prepareInputs(ctx context.Context, owner string, amount ethtypes.HexInteger) ([]*pb.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
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
			return nil, nil, err
		}

		states, err := d.findAvailableStates(ctx, string(queryJSON))
		if err != nil {
			return nil, nil, err
		}
		if len(states) == 0 {
			return nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.StoredAt
			coin, err := d.makeCoin(state)
			if err != nil {
				return nil, nil, fmt.Errorf("coin %s is invalid: %s", state.HashId, err)
			}
			total = total.Add(total, coin.Amount.BigInt())
			stateRefs = append(stateRefs, &pb.StateRef{
				HashId:   state.HashId,
				SchemaId: state.SchemaId,
			})
			if total.Cmp(amount.BigInt()) >= 0 {
				return stateRefs, total, nil
			}
		}
	}
}

func (d *Noto) prepareOutputs(owner string, amount ethtypes.HexInteger) ([]*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &NotoCoin{
		Salt:   types.RandHex(32),
		Owner:  owner,
		Amount: amount,
	}
	newState, err := d.makeState(newCoin)
	if err != nil {
		return nil, err
	}
	return []*pb.NewState{newState}, nil
}

func (d *Noto) findAvailableStates(ctx context.Context, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		DomainUuid: d.domainID,
		SchemaId:   d.coinSchema.Id,
		QueryJson:  query,
	}

	res := &pb.FindAvailableStatesResponse{}
	err := requestReply(ctx, d.replies, fromDomain, *d.dest, req, &res)
	return res.States, err
}

func (d *Noto) FindCoins(ctx context.Context, query string) ([]*NotoCoin, error) {
	states, err := d.findAvailableStates(ctx, query)
	if err != nil {
		return nil, err
	}

	coins := make([]*NotoCoin, len(states))
	for i, state := range states {
		if coins[i], err = d.makeCoin(state); err != nil {
			return nil, err
		}
	}
	return coins, err
}
