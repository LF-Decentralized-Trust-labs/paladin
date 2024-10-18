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
	"math/big"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoTrackerERC20.json
var NotoTrackerJSON []byte

type NotoTrackerHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	pld     pldclient.PaladinClient
	Address *tktypes.EthAddress
	ABI     abi.ABI
}

func DeployTracker(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	pld pldclient.PaladinClient,
	signer string,
) *NotoTrackerHelper {
	build := domain.LoadBuild(NotoTrackerJSON)
	builder := deployBuilder(ctx, t, pld, build.ABI, build.Bytecode).Input(map[string]any{
		"name":   "NotoTracker",
		"symbol": "NOTO",
	})
	deploy, err := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait(ctx)
	require.NoError(t, err)
	assert.NotNil(t, deploy.Receipt().ContractAddress)
	return &NotoTrackerHelper{
		t:       t,
		tb:      tb,
		pld:     pld,
		Address: deploy.Receipt().ContractAddress,
		ABI:     build.ABI,
	}
}

func (h *NotoTrackerHelper) GetBalance(ctx context.Context, account string) int64 {
	call, err := functionBuilder(ctx, h.t, h.pld, h.ABI, "balanceOf").
		Public().
		To(h.Address).
		Input(map[string]any{"account": account}).
		BuildTX()
	require.NoError(h.t, err)
	var jsonOutput map[string]any
	err = h.tb.ExecBaseLedgerCall(ctx, &jsonOutput, call)
	require.NoError(h.t, err)
	var balance big.Int
	balance.SetString(jsonOutput["0"].(string), 10)
	return balance.Int64()
}
