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

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

//go:embed abis/NotoGuardSimple.json
var NotoGuardJSON []byte

type NotoGuardHelper struct {
	t       *testing.T
	tb      testbed.Testbed
	eth     ethclient.EthClient
	Address *tktypes.EthAddress
	ABI     abi.ABI
}

func DeployGuard(
	ctx context.Context,
	t *testing.T,
	tb testbed.Testbed,
	signer string,
) *NotoGuardHelper {
	build := domain.LoadBuild(NotoGuardJSON)
	eth := tb.Components().EthClientFactory().HTTPClient()
	builder := deployBuilder(ctx, t, eth, build.ABI, build.Bytecode)
	deploy := NewTransactionHelper(ctx, t, tb, builder).SignAndSend(signer).Wait()
	assert.NotNil(t, deploy.ContractAddress)
	return &NotoGuardHelper{
		t:       t,
		tb:      tb,
		eth:     eth,
		Address: deploy.ContractAddress,
		ABI:     build.ABI,
	}
}
