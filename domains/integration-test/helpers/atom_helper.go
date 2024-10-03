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
	"strconv"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/AtomFactory.json
var AtomFactoryJSON []byte

//go:embed abis/Atom.json
var AtomJSON []byte

type AtomFactoryHelper struct {
	t           *testing.T
	tb          testbed.Testbed
	rpc         rpcbackend.Backend
	eth         ethclient.EthClient
	Address     *tktypes.EthAddress
	FactoryABI  abi.ABI
	InstanceABI abi.ABI
}

type AtomHelper struct {
	t           *testing.T
	tb          testbed.Testbed
	eth         ethclient.EthClient
	Address     *tktypes.EthAddress
	InstanceABI abi.ABI
}

type AtomOperation struct {
	ContractAddress *tktypes.EthAddress `json:"contractAddress"`
	CallData        tktypes.HexBytes    `json:"callData"`
}

type AtomDeployed struct {
	Address *tktypes.EthAddress `json:"addr"`
}

func InitAtom(
	t *testing.T,
	tb testbed.Testbed,
	rpc rpcbackend.Backend,
	address string,
) *AtomFactoryHelper {
	return &AtomFactoryHelper{
		t:           t,
		tb:          tb,
		rpc:         rpc,
		eth:         tb.Components().EthClientFactory().HTTPClient(),
		Address:     tktypes.MustEthAddress(address),
		FactoryABI:  domain.LoadBuild(AtomFactoryJSON).ABI,
		InstanceABI: domain.LoadBuild(AtomJSON).ABI,
	}
}

func (a *AtomFactoryHelper) Create(ctx context.Context, signer string, operations []*AtomOperation) *AtomHelper {
	builder := functionBuilder(ctx, a.t, a.eth, a.FactoryABI, "create").
		To(a.Address.Address0xHex()).
		Input(toJSON(a.t, map[string]any{"operations": operations}))
	tx := NewTransactionHelper(ctx, a.t, a.tb, builder).SignAndSend(signer)
	tx.Wait()

	var atomDeployed AtomDeployed
	tx.FindEvent(a.FactoryABI, "AtomDeployed", &atomDeployed)
	assert.NotEmpty(a.t, atomDeployed.Address)
	return &AtomHelper{
		t:           a.t,
		tb:          a.tb,
		eth:         a.eth,
		Address:     atomDeployed.Address,
		InstanceABI: a.InstanceABI,
	}
}

func (a *AtomHelper) Execute(ctx context.Context) *TransactionHelper {
	builder := functionBuilder(ctx, a.t, a.eth, a.InstanceABI, "execute").To(a.Address.Address0xHex())
	return NewTransactionHelper(ctx, a.t, a.tb, builder)
}

func (a *AtomHelper) GetOperationCount(ctx context.Context) int {
	output, err := functionBuilder(ctx, a.t, a.eth, a.InstanceABI, "getOperationCount").
		To(a.Address.Address0xHex()).
		CallResult()
	require.NoError(a.t, err)
	var jsonOutput map[string]any
	err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
	require.NoError(a.t, err)
	opCount, err := strconv.Atoi(jsonOutput["0"].(string))
	require.NoError(a.t, err)
	return opCount
}

func (a *AtomHelper) GetOperations(ctx context.Context) []map[string]any {
	opCount := a.GetOperationCount(ctx)
	var operations []map[string]any
	for i := 0; i < opCount; i++ {
		output, err := functionBuilder(ctx, a.t, a.eth, a.InstanceABI, "getOperation").
			To(a.Address.Address0xHex()).
			Input(map[string]int{"n": i}).
			CallResult()
		require.NoError(a.t, err)
		var jsonOutput map[string]any
		err = json.Unmarshal([]byte(output.JSON()), &jsonOutput)
		require.NoError(a.t, err)
		operations = append(operations, jsonOutput["0"].(map[string]any))
	}
	return operations
}
