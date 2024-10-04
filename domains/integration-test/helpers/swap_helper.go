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
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/Swap.json
var SwapJSON []byte

type SwapHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	eth     ethclient.EthClient
	Address ethtypes.Address0xHex
	ABI     abi.ABI
}

type TradeRequestInput struct {
	Holder1       string                    `json:"holder1"`
	Holder2       string                    `json:"holder2"`
	TokenAddress1 ethtypes.Address0xHex     `json:"tokenAddress1"`
	TokenAddress2 ethtypes.Address0xHex     `json:"tokenAddress2"`
	TokenValue1   *ethtypes.HexInteger      `json:"tokenValue1"`
	TokenValue2   *ethtypes.HexInteger      `json:"tokenValue2"`
	TradeData1    ethtypes.HexBytes0xPrefix `json:"tradeData1"`
	TradeData2    ethtypes.HexBytes0xPrefix `json:"tradeData2"`
}

type StateData struct {
	Inputs  []*tktypes.FullState `json:"inputs"`
	Outputs []*tktypes.FullState `json:"outputs"`
}

func DeploySwap(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	signer string,
	input *TradeRequestInput,
) *SwapHelper {
	build := domain.LoadBuild(SwapJSON)
	eth := tb.Components().EthClientFactory().HTTPClient()
	builder := deployBuilder(ctx, t, eth, build.ABI, build.Bytecode).
		Input(toJSON(t, map[string]any{"inputData": input}))
	bondDeploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait()
	address := ethtypes.Address0xHex(*bondDeploy.ContractAddress)
	assert.NotNil(t, address)
	return &SwapHelper{
		t:       t,
		tb:      tb,
		eth:     eth,
		Address: address,
		ABI:     build.ABI,
	}
}

func (s *SwapHelper) Prepare(ctx context.Context, states *StateData) *TransactionHelper {
	builder := functionBuilder(ctx, s.t, s.eth, s.ABI, "prepare").
		To(&s.Address).
		Input(toJSON(s.t, map[string]any{
			"states": states,
		}))
	return NewTransactionHelper(ctx, s.t, s.tb, builder)
}

func (s *SwapHelper) Execute(ctx context.Context) *TransactionHelper {
	builder := functionBuilder(ctx, s.t, s.eth, s.ABI, "execute").To(&s.Address)
	return NewTransactionHelper(ctx, s.t, s.tb, builder)
}

func (s *SwapHelper) GetTrade(ctx context.Context) map[string]any {
	output, err := functionBuilder(ctx, s.t, s.eth, s.ABI, "trade").To(&s.Address).CallResult()
	require.NoError(s.t, err)
	var jsonOutput map[string]any
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(s.t, err)
	return jsonOutput
}
