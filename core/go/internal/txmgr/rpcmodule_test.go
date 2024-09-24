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

package txmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/httpserver"
	"github.com/kaleido-io/paladin/core/internal/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTransactionManagerWithRPC(t *testing.T, init ...func(*Config, *mockComponents)) (context.Context, string, *txManager, func()) {
	ctx, txm, txmDone := newTestTransactionManager(t, true, init...)

	rpcServer, err := rpcserver.NewRPCServer(ctx, &rpcserver.Config{
		HTTP: rpcserver.HTTPEndpointConfig{
			Config: httpserver.Config{
				Port:            confutil.P(0),
				ShutdownTimeout: confutil.P("0"),
			},
		},
		WS: rpcserver.WSEndpointConfig{Disabled: true},
	})
	require.NoError(t, err)

	rpcServer.Register(txm.rpcModule)

	err = rpcServer.Start()
	require.NoError(t, err)

	return ctx, fmt.Sprintf("http://%s", rpcServer.HTTPAddr()), txm, func() {
		txmDone()
		rpcServer.Stop()
	}

}

func TestPublicTransactionLifecycle(t *testing.T) {

	ctx, url, tmr, done := newTestTransactionManagerWithRPC(t, mockPublicSubmitTxOk(t))
	defer done()

	rpcClient, err := rpcclient.NewHTTPClient(ctx, &rpcclient.HTTPConfig{URL: url})
	require.NoError(t, err)

	sampleABI := abi.ABI{
		{Type: abi.Constructor, Inputs: abi.ParameterArray{
			{Type: "uint256"}, // unname param works with array input
		}},
		{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{
			{Type: "uint256"}, // named where we are using an object input
		}},
		{Type: abi.Error, Name: "BadValue", Inputs: abi.ParameterArray{
			{Type: "uint256"},
		}},
	}

	// Submit in a public deploy with array encoded params and bytecode
	tx0ID := uuid.New()
	var tx1ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx1ID, "ptx_sendTransaction", &ptxapi.TransactionInput{
		ABI:       sampleABI,
		Bytecode:  tktypes.MustParseHexBytes("0x11223344"),
		DependsOn: []uuid.UUID{tx0ID},
		Transaction: ptxapi.Transaction{
			IdempotencyKey: "tx1",
			Type:           ptxapi.TransactionTypePublic.Enum(),
			Data:           tktypes.RawJSON(`[12345]`),
		},
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, tx1ID)

	// Query it back
	var txns []*ptxapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactions", query.NewQueryBuilder().Limit(1).Query(), true)
	require.NoError(t, err)
	assert.Len(t, txns, 1)
	assert.Equal(t, tx1ID, txns[0].ID)
	assert.Equal(t, tx0ID, txns[0].DependsOn[0])
	assert.Equal(t, `{"0":"12345"}`, txns[0].Data.String())
	assert.Equal(t, "(uint256)", txns[0].Function)

	// Check full=false
	txns = nil
	err = rpcClient.CallRPC(ctx, &txns, "ptx_queryTransactions", query.NewQueryBuilder().Limit(1).Query(), false)
	require.NoError(t, err)
	assert.Len(t, txns, 1)

	// Get the stored ABIs to check we found it
	var abis []*ptxapi.StoredABI
	err = rpcClient.CallRPC(ctx, &abis, "ptx_queryStoredABIs", query.NewQueryBuilder().Limit(1).Query())
	require.NoError(t, err)
	assert.Len(t, abis, 1)

	// Upsert the same ABI and check we get the same hash
	var abiHash tktypes.Bytes32
	err = rpcClient.CallRPC(ctx, &abiHash, "ptx_storeABI", sampleABI)
	require.NoError(t, err)
	assert.Equal(t, abiHash, abis[0].Hash)
	assert.Equal(t, abiHash, *txns[0].ABIReference)

	// Get it directly by ID
	var abiGet *ptxapi.StoredABI
	err = rpcClient.CallRPC(ctx, &abiGet, "ptx_getStoredABI", abiHash)
	require.NoError(t, err)
	assert.Equal(t, abiHash, abiGet.Hash)

	// Null on not found is the consistent ethereum pattern
	var abiNotFound *ptxapi.StoredABI
	err = rpcClient.CallRPC(ctx, &abiNotFound, "ptx_getStoredABI", tktypes.Bytes32(tktypes.RandBytes(32)))
	require.NoError(t, err)
	assert.Nil(t, abiNotFound)

	// Submit in a public invoke using that same ABI referring to the function
	var tx2ID uuid.UUID
	err = rpcClient.CallRPC(ctx, &tx2ID, "ptx_sendTransaction", &ptxapi.TransactionInput{
		DependsOn: []uuid.UUID{tx1ID},
		Transaction: ptxapi.Transaction{
			ABIReference:   &abiHash,
			IdempotencyKey: "tx2",
			Type:           ptxapi.TransactionTypePublic.Enum(),
			Data:           tktypes.RawJSON(`{"0": 123456789012345678901234567890}`), // nice big JSON number to deal with
			Function:       "set(uint256)",
			To:             tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	assert.NoError(t, err)
	var tx2 *ptxapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &tx2, "ptx_getTransaction", tx2ID, true)
	require.NoError(t, err)
	assert.Equal(t, tx2ID, tx2.ID)
	assert.Equal(t, "set(uint256)", tx2.Function)
	err = rpcClient.CallRPC(ctx, &tx2, "ptx_getTransaction", tx2ID, false)
	require.NoError(t, err)
	assert.Equal(t, tx2ID, tx2.ID)

	// Null on not found is the consistent ethereum pattern
	var txNotFound *ptxapi.Transaction
	err = rpcClient.CallRPC(ctx, &txns, "ptx_getTransaction", uuid.New(), false)
	require.NoError(t, err)
	assert.Nil(t, txNotFound)

	// Finalize the deploy as a success
	txHash1 := tktypes.Bytes32(tktypes.RandBytes(32))
	blockNumber1 := int64(12345)
	err = tmr.FinalizeTransactions(ctx, tmr.p.DB(), []*components.ReceiptInput{
		{
			TransactionID:   tx1ID,
			ReceiptType:     components.RT_Success,
			TransactionHash: &txHash1,
			BlockNumber:     &blockNumber1,
		},
	}, false)
	require.NoError(t, err)

	// We should get that back with full
	var txWithReceipt *ptxapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &txWithReceipt, "ptx_getTransaction", tx1ID, true)
	require.NoError(t, err)
	require.True(t, txWithReceipt.Receipt.Success)
	require.Equal(t, txHash1, *txWithReceipt.Receipt.TransactionHash)
	require.Equal(t, blockNumber1, txWithReceipt.Receipt.BlockNumber)
	require.Nil(t, txWithReceipt.Receipt.RevertData)

	// Select just pending transactions
	var pendingTransactionFull []*ptxapi.TransactionFull
	err = rpcClient.CallRPC(ctx, &pendingTransactionFull, "ptx_queryPendingTransactions", query.NewQueryBuilder().Limit(100).Query(), true)
	require.NoError(t, err)
	require.Len(t, pendingTransactionFull, 1)
	require.Equal(t, tx2ID, pendingTransactionFull[0].ID)
	require.Len(t, pendingTransactionFull[0].DependsOn, 1)
	var pendingTransactions []*ptxapi.Transaction
	err = rpcClient.CallRPC(ctx, &pendingTransactions, "ptx_queryPendingTransactions", query.NewQueryBuilder().Limit(100).Query(), false)
	require.NoError(t, err)
	require.Len(t, pendingTransactions, 1)

	// Finalize the invoke as a revert with an encoded error
	txHash2 := tktypes.Bytes32(tktypes.RandBytes(32))
	blockNumber2 := int64(12345)
	revertData, err := sampleABI.Errors()["BadValue"].EncodeCallDataValuesCtx(ctx, []any{12345})
	require.NoError(t, err)
	err = tmr.FinalizeTransactions(ctx, tmr.p.DB(), []*components.ReceiptInput{
		{
			TransactionID:   tx2ID,
			ReceiptType:     components.RT_FailedOnChainWithRevertData,
			TransactionHash: &txHash2,
			BlockNumber:     &blockNumber2,
			RevertData:      revertData,
		},
	}, false)
	require.NoError(t, err)

	// Ask for the receipt directly
	var txReceipt *ptxapi.TransactionReceipt
	err = rpcClient.CallRPC(ctx, &txReceipt, "ptx_getTransactionReceipt", tx2ID)
	require.NoError(t, err)
	require.NotNil(t, txReceipt)
	require.False(t, txReceipt.Success)
	require.Equal(t, txHash2, *txReceipt.TransactionHash)
	require.Equal(t, blockNumber2, txReceipt.BlockNumber)
	require.Equal(t, tktypes.HexBytes(revertData).String(), txReceipt.RevertData.String())
	require.Equal(t, `PD012216: Transaction reverted BadValue("12345")`, txReceipt.FailureMessage)

	// Select just success receipts
	var successReceipts []*ptxapi.TransactionReceipt
	err = rpcClient.CallRPC(ctx, &successReceipts, "ptx_queryTransactionReceipts", query.NewQueryBuilder().Limit(100).Equal("success", true).Query())
	require.NoError(t, err)
	require.Len(t, successReceipts, 1)
	assert.Equal(t, successReceipts[0].ID, tx1ID)

	// Get the dependency in the middle of the chain 0, 1, 2 to see both sides
	var tx1Deps *ptxapi.TransactionDependencies
	err = rpcClient.CallRPC(ctx, &tx1Deps, "ptx_getTransactionDependencies", tx1ID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{tx0ID}, tx1Deps.DependsOn)
	assert.Equal(t, []uuid.UUID{tx2ID}, tx1Deps.PrereqOf)

}
