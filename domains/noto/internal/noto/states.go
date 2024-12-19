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
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/eip712"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
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

var NotoLockTypeSet = eip712.TypeSet{
	"Lock": {
		{Name: "lockedCoin", Type: "LockedCoin"},
		{Name: "recipientCoins", Type: "Coin[]"},
	},
	"LockedCoin": {
		{Name: "id", Type: "bytes32"},
		{Name: "owner", Type: "address"},
		{Name: "amount", Type: "uint256"},
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

func (n *Noto) unmarshalCoin(stateData string) (*types.NotoCoin, error) {
	var coin types.NotoCoin
	err := json.Unmarshal([]byte(stateData), &coin)
	return &coin, err
}

func (n *Noto) unmarshalLockedCoin(stateData string) (*types.NotoLockedCoin, error) {
	var coin types.NotoLockedCoin
	err := json.Unmarshal([]byte(stateData), &coin)
	return &coin, err
}

func (n *Noto) makeNewCoinState(coin *types.NotoCoin, distributionList []string) (*prototk.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &prototk.NewState{
		SchemaId:         n.coinSchema.Id,
		StateDataJson:    string(coinJSON),
		DistributionList: distributionList,
	}, nil
}

func (n *Noto) makeNewLockedCoinState(coin *types.NotoLockedCoin, distributionList []string) (*prototk.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &prototk.NewState{
		SchemaId:         n.lockedCoinSchema.Id,
		StateDataJson:    string(coinJSON),
		DistributionList: distributionList,
	}, nil
}

func (n *Noto) makeNewInfoState(info *types.TransactionData, distributionList []string) (*prototk.NewState, error) {
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	return &prototk.NewState{
		SchemaId:         n.dataSchema.Id,
		StateDataJson:    string(infoJSON),
		DistributionList: distributionList,
	}, nil
}

func (n *Noto) prepareInputs(ctx context.Context, stateQueryContext string, owner *tktypes.EthAddress, amount *tktypes.HexUint256) ([]*types.NotoCoin, []*prototk.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*prototk.StateRef{}
	coins := []*types.NotoCoin{}
	for {
		// TODO: make this configurable
		queryBuilder := query.NewQueryBuilder().
			Limit(10).
			Sort(".created").
			Equal("owner", owner.String())

		if lastStateTimestamp > 0 {
			queryBuilder.GreaterThan(".created", lastStateTimestamp)
		}

		log.L(ctx).Debugf("State query: %s", queryBuilder.Query())
		states, err := n.findAvailableStates(ctx, stateQueryContext, queryBuilder.Query().String())

		if err != nil {
			return nil, nil, nil, err
		}
		if len(states) == 0 {
			return nil, nil, nil, i18n.NewError(ctx, msgs.MsgInsufficientFunds, total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.CreatedAt
			coin, err := n.unmarshalCoin(state.DataJson)
			if err != nil {
				return nil, nil, nil, i18n.NewError(ctx, msgs.MsgInvalidStateData, state.Id, err)
			}
			total = total.Add(total, coin.Amount.Int())
			stateRefs = append(stateRefs, &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})
			coins = append(coins, coin)
			log.L(ctx).Debugf("Selecting coin %s value=%s total=%s required=%s)", state.Id, coin.Amount.Int().Text(10), total.Text(10), amount.Int().Text(10))
			if total.Cmp(amount.Int()) >= 0 {
				return coins, stateRefs, total, nil
			}
		}
	}
}

func (n *Noto) prepareOutputs(ownerAddress *tktypes.EthAddress, amount *tktypes.HexUint256, distributionList []string) ([]*types.NotoCoin, []*prototk.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &types.NotoCoin{
		Salt:   tktypes.Bytes32(tktypes.RandBytes(32)),
		Owner:  ownerAddress,
		Amount: amount,
	}
	newState, err := n.makeNewCoinState(newCoin, distributionList)
	return []*types.NotoCoin{newCoin}, []*prototk.NewState{newState}, err
}

func (n *Noto) prepareInfo(data tktypes.HexBytes, distributionList []string) ([]*prototk.NewState, error) {
	newData := &types.TransactionData{
		Salt: tktypes.RandHex(32),
		Data: data,
	}
	newState, err := n.makeNewInfoState(newData, distributionList)
	return []*prototk.NewState{newState}, err
}

func (n *Noto) findAvailableStates(ctx context.Context, stateQueryContext, query string) ([]*prototk.StoredState, error) {
	req := &prototk.FindAvailableStatesRequest{
		StateQueryContext: stateQueryContext,
		SchemaId:          n.coinSchema.Id,
		QueryJson:         query,
	}
	res, err := n.Callbacks.FindAvailableStates(ctx, req)
	if err != nil {
		return nil, err
	}
	return res.States, nil
}

func (n *Noto) eip712Domain(contract *ethtypes.Address0xHex) map[string]interface{} {
	return map[string]interface{}{
		"name":              EIP712DomainName,
		"version":           EIP712DomainVersion,
		"chainId":           n.chainID,
		"verifyingContract": contract,
	}
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
		Domain:      n.eip712Domain(contract),
		Message: map[string]interface{}{
			"inputs":  messageInputs,
			"outputs": messageOutputs,
		},
	})
}

func (n *Noto) encodeTransferMasked(ctx context.Context, contract *ethtypes.Address0xHex, inputs, outputs []interface{}, data tktypes.HexBytes) (ethtypes.HexBytes0xPrefix, error) {
	return eip712.EncodeTypedDataV4(ctx, &eip712.TypedData{
		Types:       NotoTransferMaskedTypeSet,
		PrimaryType: "Transfer",
		Domain:      n.eip712Domain(contract),
		Message: map[string]interface{}{
			"inputs":  inputs,
			"outputs": outputs,
			"data":    data,
		},
	})
}

func (n *Noto) encodeLock(ctx context.Context, contract *ethtypes.Address0xHex, lockedCoin *types.NotoLockedCoin, recipientCoins []*types.NotoCoin) (ethtypes.HexBytes0xPrefix, error) {
	lockedCoinOutput := map[string]any{
		"id":     lockedCoin.ID,
		"owner":  lockedCoin.Owner,
		"amount": lockedCoin.Amount,
	}
	recipientOutputs := make([]any, len(recipientCoins))
	for i, coin := range recipientCoins {
		recipientOutputs[i] = map[string]any{
			"salt":   coin.Salt,
			"owner":  coin.Owner,
			"amount": coin.Amount,
		}
	}
	return eip712.EncodeTypedDataV4(ctx, &eip712.TypedData{
		Types:       NotoLockTypeSet,
		PrimaryType: "Lock",
		Domain:      n.eip712Domain(contract),
		Message: map[string]interface{}{
			"lockedCoin":     lockedCoinOutput,
			"recipientCoins": recipientOutputs,
		},
	})
}
