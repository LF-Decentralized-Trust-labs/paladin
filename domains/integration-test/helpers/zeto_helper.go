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

	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

//go:embed abis/ZetoFactory.json
var ZetoFactoryJSON []byte

type ZetoHelper struct {
	t       *testing.T
	rpc     rpcbackend.Backend
	Address *tktypes.EthAddress
}

func DeployZeto(ctx context.Context, t *testing.T, rpc rpcbackend.Backend, domainName, controllerName, tokenName string) *ZetoHelper {
	var addr tktypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	return &ZetoHelper{
		t:       t,
		rpc:     rpc,
		Address: &addr,
	}
}

func (n *ZetoHelper) Mint(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.MintParams{
		Mints: []*types.TransferParamEntry{
			{
				To:     to,
				Amount: tktypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (n *ZetoHelper) Transfer(ctx context.Context, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.TransferParams{
		Transfers: []*types.TransferParamEntry{
			{
				To:     to,
				Amount: tktypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (z *ZetoHelper) Lock(ctx context.Context, delegate *tktypes.EthAddress, call []byte) *DomainTransactionHelper {
	fn := types.ZetoABI.Functions()["lock"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.LockParams{
		Delegate: delegate,
		Call:     call,
	}))
}
