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
	"strconv"
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
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
	pld         pldclient.PaladinClient
	Address     *tktypes.EthAddress
	FactoryABI  abi.ABI
	InstanceABI abi.ABI
}

type AtomHelper struct {
	t           *testing.T
	tb          testbed.Testbed
	pld         pldclient.PaladinClient
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
	pld pldclient.PaladinClient,
	address string,
) *AtomFactoryHelper {
	a := &AtomFactoryHelper{
		t:           t,
		tb:          tb,
		pld:         pld,
		Address:     tktypes.MustEthAddress(address),
		FactoryABI:  domain.LoadBuild(AtomFactoryJSON).ABI,
		InstanceABI: domain.LoadBuild(AtomJSON).ABI,
	}

	return a
}

func (a *AtomFactoryHelper) Create(ctx context.Context, signer string, operations []*AtomOperation) *AtomHelper {
	builder := functionBuilder(ctx, a.t, a.pld, a.FactoryABI, "create").
		To(a.Address).
		Input(toJSON(a.t, map[string]any{"operations": operations}))
	th := NewTransactionHelper(ctx, a.t, a.tb, builder)
	tx := th.SignAndSend(signer)
	r, err := tx.Wait(ctx)
	require.NoError(a.t, err)
	require.NoError(a.t, r.Error())

	var atomDeployed AtomDeployed
	th.FindEvent(r.Receipt().TransactionHash, a.FactoryABI, "AtomDeployed", &atomDeployed)
	assert.NotEmpty(a.t, atomDeployed.Address)
	return &AtomHelper{
		t:           a.t,
		tb:          a.tb,
		pld:         a.pld,
		Address:     atomDeployed.Address,
		InstanceABI: a.InstanceABI,
	}
}

func (a *AtomHelper) Execute(ctx context.Context) *TransactionHelper {
	builder := functionBuilder(ctx, a.t, a.pld, a.InstanceABI, "execute").To(a.Address)
	return NewTransactionHelper(ctx, a.t, a.tb, builder)
}

func (a *AtomHelper) GetOperationCount(ctx context.Context) int {
	tx, err := functionBuilder(ctx, a.t, a.pld, a.InstanceABI, "getOperationCount").
		Public().
		To(a.Address).
		BuildTX()
	require.NoError(a.t, err)
	var jsonOutput map[string]any
	err = a.tb.ExecBaseLedgerCall(ctx, &jsonOutput, tx)
	require.NoError(a.t, err)
	opCount, err := strconv.Atoi(jsonOutput["0"].(string))
	require.NoError(a.t, err)
	return opCount
}

func (a *AtomHelper) GetOperations(ctx context.Context) []map[string]any {
	opCount := a.GetOperationCount(ctx)
	var operations []map[string]any
	for i := 0; i < opCount; i++ {
		tx, err := functionBuilder(ctx, a.t, a.pld, a.InstanceABI, "getOperation").
			Public().
			To(a.Address).
			Input(map[string]int{"n": i}).
			BuildTX()
		require.NoError(a.t, err)
		var jsonOutput map[string]any
		err = a.tb.ExecBaseLedgerCall(ctx, &jsonOutput, tx)
		require.NoError(a.t, err)
		operations = append(operations, jsonOutput["0"].(map[string]any))
	}
	return operations
}
