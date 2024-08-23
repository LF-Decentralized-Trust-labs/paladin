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

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
)

type ZetoCoin struct {
	Salt   *ethtypes.HexInteger `json:"salt"`
	Owner  string               `json:"owner"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

var ZetoCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct ZetoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
}

func (n *Zeto) makeCoin(stateData string) (*ZetoCoin, error) {
	coin := &ZetoCoin{}
	err := json.Unmarshal([]byte(stateData), &coin)
	return coin, err
}

func (z *Zeto) makeNewState(coin *ZetoCoin) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &pb.NewState{
		SchemaId:      z.coinSchema.Id,
		StateDataJson: string(coinJSON),
	}, nil
}

func (z *Zeto) prepareOutputs(owner string, amount *ethtypes.HexInteger) ([]*ZetoCoin, []*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	salt := utxo.NewSalt()
	newCoin := &ZetoCoin{
		Salt:   (*ethtypes.HexInteger)(salt),
		Owner:  owner,
		Amount: amount,
	}
	newState, err := z.makeNewState(newCoin)
	return []*ZetoCoin{newCoin}, []*pb.NewState{newState}, err
}

func (z *Zeto) findAvailableStates(ctx context.Context, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		DomainUuid: z.domainID,
		SchemaId:   z.coinSchema.Id,
		QueryJson:  query,
	}

	res := &pb.FindAvailableStatesResponse{}
	err := requestReply(ctx, z.replies, fromDomain, *z.dest, req, &res)
	return res.States, err
}

func (z *Zeto) FindCoins(ctx context.Context, query string) ([]*ZetoCoin, error) {
	states, err := z.findAvailableStates(ctx, query)
	if err != nil {
		return nil, err
	}

	coins := make([]*ZetoCoin, len(states))
	for i, state := range states {
		if coins[i], err = z.makeCoin(state.DataJson); err != nil {
			return nil, err
		}
	}
	return coins, err
}
