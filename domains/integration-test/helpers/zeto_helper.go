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
	"fmt"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/stretchr/testify/assert"
)

//go:embed abis/SampleERC20.json
var erc20ABI []byte

//go:embed abis/Zeto_Anon.json
var ZetoAnonABIJSON []byte

//go:embed abis/Zeto_AnonNullifierKyc.json
var ZetoAnonNullifierKycABIJSON []byte

type ZetoHelper struct {
	t       *testing.T
	rpc     rpcclient.Client
	Address *pldtypes.EthAddress
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
	var addr pldtypes.EthAddress
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deploy", domainName, controllerName, &types.InitializerParams{
		TokenName: tokenName,
		Name:      "Test Zeto",
		Symbol:    "ZETO",
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

func DeployERC20(ctx context.Context, rpc rpcclient.Client, deployer, initialOwnerAddr string) (*pldtypes.EthAddress, error) {
	build := solutils.MustLoadBuild(erc20ABI)
	params := fmt.Sprintf(`{"initialOwner":"%s"}`, initialOwnerAddr)
	var addr string
	rpcerr := rpc.CallRPC(ctx, &addr, "testbed_deployBytecode", deployer, build.ABI, build.Bytecode.String(), pldtypes.RawJSON(params))
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return pldtypes.MustEthAddress(addr), nil
}

func (z *ZetoHelperFungible) SetERC20(ctx context.Context, tb testbed.Testbed, sender string, erc20Address *pldtypes.EthAddress) {
	paramsJson, _ := json.Marshal(&map[string]string{"erc20": erc20Address.String()})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "setERC20",
			Data:     paramsJson,
		},
		ABI: solutils.MustLoadBuild(ZetoAnonABIJSON).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelperFungible) Mint(ctx context.Context, to string, amounts []uint64) *DomainTransactionHelper {
	entries := make([]*types.FungibleTransferParamEntry, len(amounts))
	for i, amount := range amounts {
		entries[i] = &types.FungibleTransferParamEntry{
			To:     to,
			Amount: pldtypes.Uint64ToUint256(amount),
		}
	}
	fn := types.ZetoFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleMintParams{
		Mints: entries,
	}))
}

func (z *ZetoHelperFungible) Transfer(ctx context.Context, to []string, amounts []uint64) *DomainTransactionHelper {
	entries := make([]*types.FungibleTransferParamEntry, len(amounts))
	for i, amount := range amounts {
		entries[i] = &types.FungibleTransferParamEntry{
			To:     to[i],
			Amount: pldtypes.Uint64ToUint256(amount),
		}
	}
	fn := types.ZetoFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleTransferParams{
		Transfers: entries,
	}))
}

func (z *ZetoHelperFungible) BalanceOf(ctx context.Context, account string) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["balanceOf"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleBalanceOfParam{
		Account: account,
	}))
}

func (z *ZetoHelper) TransferLocked(ctx context.Context, lockedUtxo *pldtypes.HexUint256, delegate, to string, amount uint64) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["transferLocked"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.FungibleTransferLockedParams{
		LockedInputs: []*pldtypes.HexUint256{lockedUtxo},
		Delegate:     delegate,
		Transfers: []*types.FungibleTransferParamEntry{
			{
				To:     to,
				Amount: pldtypes.Uint64ToUint256(amount),
			},
		},
	}))
}

func (z *ZetoHelper) SendTransferLocked(ctx context.Context, tb testbed.Testbed, sender string, result *testbed.TransactionResult) {
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "transferLocked",
			Data:     result.PreparedTransaction.Data,
		},
		ABI: solutils.MustLoadBuild(ZetoAnonABIJSON).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) Lock(ctx context.Context, delegate *pldtypes.EthAddress, amount int) *DomainTransactionHelper {
	fn := types.ZetoFungibleABI.Functions()["lock"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &types.LockParams{
		Delegate: delegate,
		Amount:   pldtypes.Uint64ToUint256(uint64(amount)),
	}))
}

func (z *ZetoHelper) DelegateLock(ctx context.Context, tb testbed.Testbed, lockedUtxo *pldtypes.HexUint256, delegate *pldtypes.EthAddress, sender string) {
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

func (z *ZetoHelper) MintERC20(ctx context.Context, tb testbed.Testbed, erc20Address pldtypes.EthAddress, amount int64, from, to string) {
	paramsJson, _ := json.Marshal(&map[string]any{"amount": amount, "to": to})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "mint",
			Data:     paramsJson,
		},
		ABI: solutils.MustLoadBuild(erc20ABI).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) ApproveERC20(ctx context.Context, tb testbed.Testbed, erc20Address pldtypes.EthAddress, amount int64, from string) {
	paramsJson, _ := json.Marshal(&map[string]any{"spender": z.Address.String(), "value": amount})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from,
			To:       &erc20Address,
			Function: "approve",
			Data:     paramsJson,
		},
		ABI: solutils.MustLoadBuild(erc20ABI).ABI,
	})
	assert.NoError(z.t, err)
}

func (z *ZetoHelper) Deposit(ctx context.Context, amount int64) *DomainTransactionHelper {
	params := &types.DepositParams{
		Amount: pldtypes.Int64ToInt256(amount),
	}
	fn := types.ZetoFungibleABI.Functions()["deposit"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &params))
}

func (z *ZetoHelper) Withdraw(ctx context.Context, amount int64) *DomainTransactionHelper {
	params := &types.WithdrawParams{
		Amount: pldtypes.Int64ToInt256(amount),
	}
	fn := types.ZetoFungibleABI.Functions()["withdraw"]
	return NewDomainTransactionHelper(ctx, z.t, z.rpc, z.Address, fn, toJSON(z.t, &params))
}

func (z *ZetoHelper) Register(ctx context.Context, tb testbed.Testbed, sender string, publicKey []*big.Int) {
	abi := abi.ABI{
		&abi.Entry{
			Type: abi.Function,
			Name: "register",
			Inputs: abi.ParameterArray{
				{Name: "publicKey", Type: "uint256[2]"},
				{Name: "data", Type: "bytes"},
			},
		},
	}
	paramsJson, _ := json.Marshal(&map[string]any{"publicKey": publicKey, "data": "0x"})
	_, err := tb.ExecTransactionSync(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     sender,
			To:       z.Address,
			Function: "register",
			Data:     paramsJson,
		},
		ABI: abi,
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
	var addr pldtypes.EthAddress
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

func (n *ZetoHelperNonFungible) Mint(ctx context.Context, to, uri []string) *DomainTransactionHelper {
	entries := make([]*types.NonFungibleTransferParamEntry, len(uri))
	for i, u := range uri {
		entries[i] = &types.NonFungibleTransferParamEntry{
			To:  to[i],
			URI: u,
		}
	}
	fn := types.ZetoNonFungibleABI.Functions()["mint"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleMintParams{
		Mints: entries,
	}))
}

func (n *ZetoHelperNonFungible) Transfer(ctx context.Context, to string, tokenID *pldtypes.HexUint256) *DomainTransactionHelper {
	fn := types.ZetoNonFungibleABI.Functions()["transfer"]
	return NewDomainTransactionHelper(ctx, n.t, n.rpc, n.Address, fn, toJSON(n.t, &types.NonFungibleTransferParams{
		Transfers: []*types.NonFungibleTransferParamEntry{
			{
				To:      to,
				TokenID: tokenID,
			},
		},
	}))
}
