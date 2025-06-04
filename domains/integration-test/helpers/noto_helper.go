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
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/rpcclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/solutils"
	"github.com/stretchr/testify/assert"
)

//go:embed abis/NotoFactory.json
var NotoFactoryJSON []byte

//go:embed abis/INoto.json
var NotoInterfaceJSON []byte

type NotoHelper struct {
	t       *testing.T
	rpc     rpcclient.Client
	Address *pldtypes.EthAddress
	ABI     abi.ABI
}

func DeployNoto(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, notary string, hooks *pldtypes.EthAddress) *NotoHelper {
	notaryMode := types.NotaryModeBasic
	if hooks != nil {
		notaryMode = types.NotaryModeHooks
	}

	var addr pldtypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, "notary", &types.ConstructorParams{
		Notary:     notary + "@node1",
		NotaryMode: notaryMode,
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     hooks,
				DevUsePublicHooks: true,
			},
		},
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &NotoHelper{
		t:       t,
		rpc:     rpc,
		Address: &addr,
		ABI:     solutils.MustLoadBuild(NotoInterfaceJSON).ABI,
	}
}

func (n *NotoHelper) Mint(ctx context.Context, to string, amount int64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.MintParams{
		To:     to,
		Amount: pldtypes.Int64ToInt256(amount),
	}))
}

func (n *NotoHelper) Transfer(ctx context.Context, to string, amount int64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.TransferParams{
		To:     to,
		Amount: pldtypes.Int64ToInt256(amount),
	}))
}

func (n *NotoHelper) ApproveTransfer(ctx context.Context, params *types.ApproveParams) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["approveTransfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}

func (n *NotoHelper) Lock(ctx context.Context, params *types.LockParams) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["lock"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}

func (n *NotoHelper) Unlock(ctx context.Context, params *types.UnlockParams) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["unlock"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}

func (n *NotoHelper) PrepareUnlock(ctx context.Context, params *types.UnlockParams) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["prepareUnlock"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}

func (n *NotoHelper) DelegateLock(ctx context.Context, params *types.DelegateLockParams) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["delegateLock"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}

func (n *NotoHelper) BalanceOf(ctx context.Context, params *types.BalanceOfParam) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["balanceOf"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, params))
}
