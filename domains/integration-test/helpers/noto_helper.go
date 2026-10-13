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

	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/rpcclient"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger-firefly/signer/pkg/abi"
	"github.com/stretchr/testify/require"
)

//go:embed abis/NotoFactory.json
var NotoFactoryJSON []byte

//go:embed abis/Noto.json
var NotoJSON []byte

//go:embed abis/Noto_V0.json
var NotoV0JSON []byte

//go:embed abis/SmtLib.json
var SmtLibJSON []byte

//go:embed abis/NotoNullifiers.json
var NotoNullifiersJSON []byte

//go:embed abis/ERC1967Proxy.json
var ERC1967ProxyJSON []byte

//go:embed abis/INoto.json
var NotoInterfaceJSON []byte

//go:embed abis/INoto_V0.json
var NotoV0InterfaceJSON []byte

var NotoFactoryABI = solutils.MustParseBuildABI(NotoFactoryJSON)

type NotoHelper struct {
	t       *testing.T
	rpc     rpcclient.Client
	Address *pldtypes.EthAddress
	ABI     abi.ABI
}

func DeployNoto(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, variant, notary string, hooks *pldtypes.EthAddress) *NotoHelper {
	return DeployNotoImplementation(ctx, t, rpc, domainName, variant, notary, hooks)
}

func DeployNotoImplementation(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, implementation, notary string, hooks *pldtypes.EthAddress) *NotoHelper {
	notaryMode := types.NotaryModeBasic
	if hooks != nil {
		notaryMode = types.NotaryModeHooks
	}

	var addr pldtypes.EthAddress
	constructorParams := &types.ConstructorParams{
		Notary:     notary + "@node1",
		NotaryMode: notaryMode,
		Options: types.NotoOptions{
			Hooks: &types.NotoHooksOptions{
				PublicAddress:     hooks,
				DevUsePublicHooks: true,
			},
		},
	}
	if implementation != "" {
		constructorParams.Implementation = implementation
		constructorParams.Name = implementation + "-" + pldtypes.RandHex(4)
	}
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, "notary", constructorParams)
	if rpcerr != nil {
		require.NoError(t, rpcerr)
	}
	return &NotoHelper{
		t:       t,
		rpc:     rpc,
		Address: &addr,
		ABI:     solutils.MustLoadBuild(NotoInterfaceJSON).ABI,
	}
}

func (n *NotoHelper) Mint(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.MintParams{
		To:     to,
		Amount: pldtypes.Uint64ToUint256(amount),
	}))
}

func (n *NotoHelper) Transfer(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.TransferParams{
		To:     to,
		Amount: pldtypes.Uint64ToUint256(amount),
	}))
}

func (n *NotoHelper) TransferFrom(ctx context.Context, from, to string, amount uint64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["transferFrom"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.TransferFromParams{
		From:   from,
		To:     to,
		Amount: pldtypes.Uint64ToUint256(amount),
	}))
}

func (n *NotoHelper) Burn(ctx context.Context, amount uint64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["burn"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.BurnParams{
		Amount: pldtypes.Uint64ToUint256(amount),
	}))
}

func (n *NotoHelper) BurnFrom(ctx context.Context, from string, amount uint64) *DomainTransactionHelper {
	fn := types.NotoABI.Functions()["burnFrom"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.BurnFromParams{
		From:   from,
		Amount: pldtypes.Uint64ToUint256(amount),
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

func (n *NotoHelper) PrepareUnlock(ctx context.Context, params *types.PrepareUnlockParams) *DomainTransactionHelper {
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
