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

	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

//go:embed abis/Zeto_Anon.json
var ZetoAnonABIJSON []byte

type ZetoHelper struct {
	t       *testing.T
	rpc     rpcclient.Client
	Address *tktypes.EthAddress
}

// =============================================================================
//
//	Fungible
//
// =============================================================================
type ZetoHelperFungible struct {
	ZetoHelper
}

func DeployZetoFungible(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, controllerName, tokenName string) *ZetoHelperFungible {
	var addr tktypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &ZetoHelperFungible{
		ZetoHelper: ZetoHelper{
			t:       t,
			rpc:     rpc,
			Address: &addr,
		},
	}
}

func (n *ZetoHelperFungible) Mint(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.FungibleMintParams{
		Mints: []*types.FungibleTransferParamEntry{
			{
				To:     to,
				Amount: tktypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (n *ZetoHelperFungible) Transfer(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.FungibleTransferParams{
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     to,
				Amount: tktypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (n *ZetoHelper) TransferLocked(ctx context.Context, lockedUtxo *tktypes.HexUint256, delegate string, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["transferLocked"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.FungibleTransferLockedParams{
		LockedInputs: []*tktypes.HexUint256{lockedUtxo},
		Delegate:     delegate,
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     to,
				Amount: tktypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (z *ZetoHelper) Lock(ctx context.Context, delegate *tktypes.EthAddress, amount int) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["lock"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.LockParams{
		Delegate: delegate,
		Amount:   tktypes.Uint64ToUint256(uint64(amount)),
	}))
}

func (z *ZetoHelper) DelegateLock(ctx context.Context, tb testbed.Testbed, lockedUtxo *tktypes.HexUint256, delegate *tktypes.EthAddress, sender string) {
	txInput := map[string]any{
		"utxos":    []string{lockedUtxo.String()},
		"delegate": delegate.String(),
		"data":     "0x",
	}
	txInputJson, _ := json.Marshal(txInput)
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "delegateLock",
			Data:     txInputJson,
		},
		ABI: solutils.MustLoadBuild(ZetoAnonABIJSON).ABI,
	})
	assert.NoError(z.t, err)
}

// =============================================================================
//
//	NonFungible
//
// =============================================================================
type ZetoHelperNonFungible struct {
	ZetoHelper
}

func DeployZetoNonFungible(ctx context.Context, t *testing.T, rpc rpcclient.Client, domainName, controllerName, tokenName string) *ZetoHelperNonFungible {
	var addr tktypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr)
	}
	return &ZetoHelperNonFungible{
		ZetoHelper: ZetoHelper{
			t:       t,
			rpc:     rpc,
			Address: &addr,
		},
	}
}

func (n *ZetoHelperNonFungible) Mint(ctx context.Context, to, uri string) *DomainTransactionHelper {
	fn := types.ZetoNonFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleMintParams{
		Mints: []*types.NonFungibleTransferParamEntry{
			{
				To:  to,
				URI: uri,
			},
		},
	}))
}

func (n *ZetoHelperNonFungible) Transfer(ctx context.Context, to string, tokenID string) *DomainTransactionHelper {
	fn := types.ZetoNonFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleTransferParams{
		Transfers: []*types.NonFungibleTransferParamEntry{
			{
				To:      to,
				TokenID: tktypes.MustParseHexUint256(tokenID),
			},
		},
	}))
}
