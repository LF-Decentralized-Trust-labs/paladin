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

	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

func (n *Noto) BuildReceipt(ctx context.Context, req *prototk.BuildReceiptRequest) (res *prototk.BuildReceiptResponse, err error) {
	receipt := &types.NotoDomainReceipt{}

	infoStates := n.filterSchema(req.InfoStates, []string{n.dataSchema.Id})
	if len(infoStates) == 1 {
		info, err := n.unmarshalInfo(infoStates[0].StateDataJson)
		if err != nil {
			return nil, err
		}
		receipt.Data = info.Data
	}

	lockInfoStates := n.filterSchema(req.InfoStates, []string{n.lockInfoSchema.Id})
	if len(lockInfoStates) == 1 {
		lock, err := n.unmarshalLock(lockInfoStates[0].StateDataJson)
		if err != nil {
			return nil, err
		}
		receipt.LockInfo = &types.ReceiptLockInfo{LockID: lock.LockID}
		if !lock.Delegate.IsZero() {
			receipt.LockInfo.Delegate = lock.Delegate
		}
	}

	receipt.States.Inputs, err = n.receiptStates(ctx, n.filterSchema(req.InputStates, []string{n.coinSchema.Id}))
	if err == nil {
		receipt.States.LockedInputs, err = n.receiptStates(ctx, n.filterSchema(req.InputStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.Outputs, err = n.receiptStates(ctx, n.filterSchema(req.OutputStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.LockedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.OutputStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.ReadInputs, err = n.receiptStates(ctx, n.filterSchema(req.ReadStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.ReadLockedInputs, err = n.receiptStates(ctx, n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id}))
	}
	if err == nil {
		receipt.States.PreparedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.InfoStates, []string{n.coinSchema.Id}))
	}
	if err == nil {
		receipt.States.PreparedLockedOutputs, err = n.receiptStates(ctx, n.filterSchema(req.InfoStates, []string{n.lockedCoinSchema.Id}))
	}
	if err != nil {
		return nil, err
	}

	if receipt.LockInfo != nil && len(receipt.States.ReadLockedInputs) > 0 && len(receipt.States.PreparedOutputs) > 0 {
		// For prepareUnlock transactions, include the encoded "unlock" call that can be used to unlock the coins
		unlock := interfaceBuild.ABI.Functions()["unlock"]
		receipt.LockInfo.UnlockParams = &types.UnlockPublicParams{
			LockedInputs:  endorsableStateIDs(n.filterSchema(req.ReadStates, []string{n.lockedCoinSchema.Id})),
			LockedOutputs: endorsableStateIDs(n.filterSchema(req.InfoStates, []string{n.lockedCoinSchema.Id})),
			Outputs:       endorsableStateIDs(n.filterSchema(req.InfoStates, []string{n.coinSchema.Id})),
			Signature:     pldtypes.HexBytes{},
			Data:          pldtypes.HexBytes{},
		}
		paramsJSON, err := json.Marshal(receipt.LockInfo.UnlockParams)
		if err != nil {
			return nil, err
		}
		encodedCall, err := unlock.EncodeCallDataJSONCtx(ctx, paramsJSON)
		if err != nil {
			return nil, err
		}
		receipt.LockInfo.UnlockCall = encodedCall
	}

	receipt.Transfers, err = n.receiptTransfers(ctx, req)
	if err != nil {
		return nil, err
	}

	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}

	return &prototk.BuildReceiptResponse{
		ReceiptJson: string(receiptJSON),
	}, nil
}

func (n *Noto) receiptStates(ctx context.Context, states []*prototk.EndorsableState) ([]*types.ReceiptState, error) {
	coins := make([]*types.ReceiptState, len(states))
	for i, state := range states {
		id, err := pldtypes.ParseHexBytes(ctx, state.Id)
		if err != nil {
			return nil, err
		}
		schemaID, err := pldtypes.ParseBytes32Ctx(ctx, state.SchemaId)
		if err != nil {
			return nil, err
		}
		coins[i] = &types.ReceiptState{
			ID:     id,
			Schema: schemaID,
			Data:   pldtypes.RawJSON(state.StateDataJson),
		}
	}
	return coins, nil
}

func (n *Noto) receiptTransfers(ctx context.Context, req *prototk.BuildReceiptRequest) ([]*types.ReceiptTransfer, error) {
	inputCoins, err := n.parseCoinList(ctx, "inputs", n.filterSchema(req.InputStates, []string{n.coinSchema.Id, n.lockedCoinSchema.Id}))
	if err != nil {
		return nil, err
	}
	outputCoins, err := n.parseCoinList(ctx, "outputs", n.filterSchema(req.OutputStates, []string{n.coinSchema.Id, n.lockedCoinSchema.Id}))
	if err != nil {
		return nil, err
	}

	var from *pldtypes.EthAddress
	fromAmount := big.NewInt(0)
	to := make(map[pldtypes.EthAddress]*big.Int)

	parseInput := func(owner *pldtypes.EthAddress, amount *big.Int) bool {
		if from == nil {
			from = owner
		} else if !owner.Equals(from) {
			return false
		}
		fromAmount.Add(fromAmount, amount)
		return true
	}

	parseOutput := func(owner pldtypes.EthAddress, amount *big.Int) bool {
		if owner.Equals(from) {
			fromAmount.Sub(fromAmount, amount)
		} else if toAmount, ok := to[owner]; ok {
			toAmount.Add(toAmount, amount)
		} else {
			to[owner] = amount
		}
		return true
	}

	for _, coin := range inputCoins.coins {
		if !parseInput(coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}
	for _, coin := range inputCoins.lockedCoins {
		if !parseInput(coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}
	for _, coin := range outputCoins.coins {
		if !parseOutput(*coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}
	for _, coin := range outputCoins.lockedCoins {
		if !parseOutput(*coin.Owner, coin.Amount.Int()) {
			return nil, nil
		}
	}

	if len(to) == 0 && from != nil && fromAmount.BitLen() > 0 {
		// special case for burn (no recipients)
		return []*types.ReceiptTransfer{{
			From:   from,
			Amount: (*pldtypes.HexUint256)(fromAmount),
		}}, nil
	}

	transfers := make([]*types.ReceiptTransfer, 0, len(to))
	for owner, amount := range to {
		if amount.BitLen() > 0 {
			transfers = append(transfers, &types.ReceiptTransfer{
				From:   from,
				To:     &owner,
				Amount: (*pldtypes.HexUint256)(amount),
			})
		}
	}
	return transfers, nil
}
