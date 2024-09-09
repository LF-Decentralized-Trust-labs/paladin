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

	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

var EIP712DomainName = "noto"
var EIP712DomainVersion = "0.0.1"

var NotoTransferUnmaskedTypeSet = eip712.TypeSet{
	"Transfer": {
		{Name: "inputs", Type: "Coin[]"},
		{Name: "outputs", Type: "Coin[]"},
	},
	"Coin": {
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "address"},
		{Name: "amount", Type: "uint256"},
	},
	eip712.EIP712Domain: {
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	},
}

var NotoTransferMaskedTypeSet = eip712.TypeSet{
	"Transfer": {
		{Name: "inputs", Type: "bytes32[]"},
		{Name: "outputs", Type: "bytes32[]"},
		{Name: "data", Type: "bytes"},
	},
	eip712.EIP712Domain: {
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	},
}

func (n *Noto) unmarshalCoin(stateData string) (*types.NotoCoin, error) {
	var coin types.NotoCoin
	err := json.Unmarshal([]byte(stateData), &coin)
	return &coin, err
}

func (n *Noto) makeNewState(coin *types.NotoCoin) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &pb.NewState{
		SchemaId:      n.coinSchema.Id,
		StateDataJson: string(coinJSON),
	}, nil
}

func (n *Noto) prepareInputs(ctx context.Context, owner ethtypes.Address0xHex, amount *ethtypes.HexInteger) ([]*types.NotoCoin, []*pb.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	coins := []*types.NotoCoin{}
	for {
		// Simple oldest coin first algorithm
		// TODO: make this configurable
		// TODO: why is filters.QueryJSON not a public interface?
		query := map[string]interface{}{
			"limit": 10,
			"sort":  []string{".created"},
			"eq": []map[string]string{{
				"field": "owner",
				"value": owner.String(),
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

		states, err := n.findAvailableStates(ctx, string(queryJSON))
		if err != nil {
			return nil, nil, nil, err
		}
		if len(states) == 0 {
			return nil, nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.StoredAt
			coin, err := n.unmarshalCoin(state.DataJson)
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
		}
	}
}

func (n *Noto) prepareOutputs(owner ethtypes.Address0xHex, amount *ethtypes.HexInteger) ([]*types.NotoCoin, []*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &types.NotoCoin{
		Salt:   tktypes.RandHex(32),
		Owner:  owner,
		Amount: amount,
	}
	newState, err := n.makeNewState(newCoin)
	return []*types.NotoCoin{newCoin}, []*pb.NewState{newState}, err
}

func (n *Noto) findAvailableStates(ctx context.Context, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		SchemaId:  n.coinSchema.Id,
		QueryJson: query,
	}
	res, err := n.Callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}

func (n *Noto) encodeTransferUnmasked(ctx context.Context, contract *ethtypes.Address0xHex, inputs, outputs []*types.NotoCoin) (ethtypes.HexBytes0xPrefix, error) {
	messageInputs := make([]interface{}, len(inputs))
	for i, input := range inputs {
		messageInputs[i] = map[string]interface{}{
			"salt":   input.Salt,
			"owner":  input.Owner,
			"amount": input.Amount.String(),
		}
	}
	messageOutputs := make([]interface{}, len(outputs))
	for i, output := range outputs {
		messageOutputs[i] = map[string]interface{}{
			"salt":   output.Salt,
			"owner":  output.Owner,
			"amount": output.Amount.String(),
		}
	}
	return eip712.EncodeTypedDataV4(ctx, &eip712.TypedData{
		Types:       NotoTransferUnmaskedTypeSet,
		PrimaryType: "Transfer",
		Domain: map[string]interface{}{
			"name":              EIP712DomainName,
			"version":           EIP712DomainVersion,
			"chainId":           n.chainID,
			"verifyingContract": contract,
		},
		Message: map[string]interface{}{
			"inputs":  messageInputs,
			"outputs": messageOutputs,
		},
	})
}

func (n *Noto) encodeTransferMasked(ctx context.Context, contract *ethtypes.Address0xHex, inputs, outputs []interface{}, data ethtypes.HexBytes0xPrefix) (ethtypes.HexBytes0xPrefix, error) {
	return eip712.EncodeTypedDataV4(ctx, &eip712.TypedData{
		Types:       NotoTransferMaskedTypeSet,
		PrimaryType: "Transfer",
		Domain: map[string]interface{}{
			"name":              EIP712DomainName,
			"version":           EIP712DomainVersion,
			"chainId":           n.chainID,
			"verifyingContract": contract,
		},
		Message: map[string]interface{}{
			"inputs":  inputs,
			"outputs": outputs,
			"data":    data,
		},
	})
}
