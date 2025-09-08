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
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoTrackerPublicERC20.json
var NotoTrackerJSON []byte

type NotoTrackerHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	pld     pldclient.PaladinClient
	Address *pldtypes.EthAddress
	ABI     abi.ABI
}

func DeployTracker(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	pld pldclient.PaladinClient,
	signer string,
) *NotoTrackerHelper {
	build := solutils.MustLoadBuild(NotoTrackerJSON)
	builder := deployBuilder(ctx, pld, build.ABI, build.Bytecode).Inputs(map[string]any{
		"name":   "NotoTracker",
		"symbol": "NOTO",
	})
	deploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait(5 * time.Second)
	require.NoError(t, deploy.Error())
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
	var jsonOutput map[string]any
	err := functionBuilder(ctx, h.pld, h.ABI, "balanceOf").
		Public().
		To(h.Address).
		Inputs(map[string]any{"account": account}).
		Outputs(&jsonOutput).
		BuildTX().
		Call()
	require.NoError(h.t, err)
	var balance big.Int
	balance.SetString(jsonOutput["0"].(string), 10)
	return balance.Int64()
}
