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
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/Swap.json
var SwapJSON []byte

type SwapHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	pld     pldclient.PaladinClient
	Address *pldtypes.EthAddress
	ABI     abi.ABI
}

type TradeRequestInput struct {
	Holder1       string               `json:"holder1"`
	Holder2       string               `json:"holder2"`
	TokenAddress1 *pldtypes.EthAddress `json:"tokenAddress1"`
	TokenAddress2 *pldtypes.EthAddress `json:"tokenAddress2"`
	TokenValue1   *pldtypes.HexUint256 `json:"tokenValue1"`
	TokenValue2   *pldtypes.HexUint256 `json:"tokenValue2"`
	TradeData1    pldtypes.HexBytes    `json:"tradeData1"`
	TradeData2    pldtypes.HexBytes    `json:"tradeData2"`
}

type StateData struct {
	Inputs  []*pldapi.StateEncoded `json:"inputs"`
	Outputs []*pldapi.StateEncoded `json:"outputs"`
}

func DeploySwap(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	pld pldclient.PaladinClient,
	signer string,
	input *TradeRequestInput,
) *SwapHelper {
	build := solutils.MustLoadBuild(SwapJSON)
	builder := deployBuilder(ctx, pld, build.ABI, build.Bytecode).
		Inputs(toJSON(t, map[string]any{"inputData": input}))
	deploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait(5 * time.Second)
	require.NoError(t, deploy.Error())
	assert.NotNil(t, deploy.Receipt().ContractAddress)
	return &SwapHelper{
		t:       t,
		tb:      tb,
		pld:     pld,
		Address: deploy.Receipt().ContractAddress,
		ABI:     build.ABI,
	}
}

func (s *SwapHelper) Prepare(ctx context.Context, states *StateData) *TransactionHelper {
	builder := functionBuilder(ctx, s.pld, s.ABI, "prepare").
		To(s.Address).
		Inputs(map[string]any{
			"states": states,
		})
	return NewTransactionHelper(ctx, s.t, s.tb, builder)
}

func (s *SwapHelper) Execute(ctx context.Context) *TransactionHelper {
	builder := functionBuilder(ctx, s.pld, s.ABI, "execute").To(s.Address)
	return NewTransactionHelper(ctx, s.t, s.tb, builder)
}

func (s *SwapHelper) GetTrade(ctx context.Context) map[string]any {
	var jsonOutput map[string]any
	err := functionBuilder(ctx, s.pld, s.ABI, "trade").
		Public().
		To(s.Address).
		Outputs(&jsonOutput).
		BuildTX().
		Call()
	require.NoError(s.t, err)
	return jsonOutput
}
