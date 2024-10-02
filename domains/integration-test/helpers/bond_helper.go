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

package helpers

import (
	"context"
	_ "embed"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/FixedCouponBond.json
var BondJSON []byte

type BondHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	eth     ethclient.EthClient
	Address ethtypes.Address0xHex
	ABI     abi.ABI
}

func DeployBond(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	signer string,
	recipient string,
	bondDetails map[string]any,
) *BondHelper {
	build := domain.LoadBuild(BondJSON)
	eth := tb.Components().EthClientFactory().HTTPClient()
	builder := deployBuilder(ctx, t, eth, build.ABI, build.Bytecode).
		Input(toJSON(t, map[string]any{
			"recipient_": recipient,
			"details_":   bondDetails,
		}))
	bondDeploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait()
	address := ethtypes.Address0xHex(*bondDeploy.ContractAddress)
	assert.NotNil(t, address)
	return &BondHelper{
		t:       t,
		tb:      tb,
		eth:     eth,
		Address: address,
		ABI:     build.ABI,
	}
}

func (b *BondHelper) CouponPayment(ctx context.Context) uint64 {
	output, err := functionBuilder(ctx, b.t, b.eth, b.ABI, "couponPayment").
		To(&b.Address).
		CallResult()
	require.NoError(b.t, err)
	var jsonOutput map[string]string
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(b.t, err)
	var intVal big.Int
	intVal.SetString(jsonOutput["0"], 10)
	return intVal.Uint64()
}

func (b *BondHelper) ClaimedPayments(ctx context.Context) uint64 {
	output, err := functionBuilder(ctx, b.t, b.eth, b.ABI, "claimedPayments").
		To(&b.Address).
		CallResult()
	require.NoError(b.t, err)
	var jsonOutput map[string]string
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(b.t, err)
	var intVal big.Int
	intVal.SetString(jsonOutput["0"], 10)
	return intVal.Uint64()
}

func (b *BondHelper) AvailablePayments(ctx context.Context) uint64 {
	output, err := functionBuilder(ctx, b.t, b.eth, b.ABI, "availablePayments").
		To(&b.Address).
		CallResult()
	require.NoError(b.t, err)
	var jsonOutput map[string]string
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(b.t, err)
	var intVal big.Int
	intVal.SetString(jsonOutput["0"], 10)
	return intVal.Uint64()
}

func (b *BondHelper) Issue(ctx context.Context, quantity uint64) *TransactionHelper {
	builder := functionBuilder(ctx, b.t, b.eth, b.ABI, "issue").
		To(&b.Address).
		Input(toJSON(b.t, map[string]any{
			"quantity_":  quantity,
			"issueDate_": 0,
		}))
	return NewTransactionHelper(ctx, b.t, b.tb, builder)
}

func (b *BondHelper) RecordTransfer(ctx context.Context, quantity, claimedPayments uint64) *TransactionHelper {
	builder := functionBuilder(ctx, b.t, b.eth, b.ABI, "recordTransfer").
		To(&b.Address).
		Input(toJSON(b.t, map[string]any{
			"quantity_":        quantity,
			"issueDate_":       0,
			"claimedPayments_": claimedPayments,
		}))
	return NewTransactionHelper(ctx, b.t, b.tb, builder)
}

func (b *BondHelper) Approve(ctx context.Context, delegate ethtypes.Address0xHex) *TransactionHelper {
	builder := functionBuilder(ctx, b.t, b.eth, b.ABI, "approve").
		To(&b.Address).
		Input(map[string]any{
			"delegate": delegate,
			"approved": true,
		})
	return NewTransactionHelper(ctx, b.t, b.tb, builder)
}

func (b *BondHelper) ClaimPayment(ctx context.Context) *TransactionHelper {
	builder := functionBuilder(ctx, b.t, b.eth, b.ABI, "claimPayment").
		To(&b.Address)
	return NewTransactionHelper(ctx, b.t, b.tb, builder)
}

func (b *BondHelper) Transfer(ctx context.Context, amount uint64) *TransactionHelper {
	builder := functionBuilder(ctx, b.t, b.eth, b.ABI, "transfer").
		To(&b.Address).
		Input(toJSON(b.t, map[string]any{
			"amount": amount,
		}))
	return NewTransactionHelper(ctx, b.t, b.tb, builder)
}
