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
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoTrackerERC20.json
var NotoTrackerJSON []byte

type NotoTrackerHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	eth     ethclient.EthClient
	Address *tktypes.EthAddress
	ABI     abi.ABI
}

func DeployTracker(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	signer string,
) *NotoTrackerHelper {
	build := domain.LoadBuild(NotoTrackerJSON)
	eth := tb.Components().EthClientFactory().HTTPClient()
	builder := deployBuilder(ctx, t, eth, build.ABI, build.Bytecode).Input(map[string]any{
		"name":   "NotoTracker",
		"symbol": "NOTO",
	})
	deploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait()
	assert.NotNil(t, deploy.ContractAddress)
	return &NotoTrackerHelper{
		t:       t,
		tb:      tb,
		eth:     eth,
		Address: deploy.ContractAddress,
		ABI:     build.ABI,
	}
}

func (h *NotoTrackerHelper) GetBalance(ctx context.Context, account string) int64 {
	output, err := functionBuilder(ctx, h.t, h.eth, h.ABI, "balanceOf").
		To(h.Address.Address0xHex()).
		Input(map[string]any{"account": account}).
		CallResult()
	require.NoError(h.t, err)
	var jsonOutput map[string]any
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(h.t, err)
	var balance big.Int
	balance.SetString(jsonOutput["0"].(string), 10)
	return balance.Int64()
}
