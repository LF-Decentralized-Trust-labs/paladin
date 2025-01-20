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
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func mockBeginRollback(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectBegin()
	mc.db.ExpectRollback()
}

func TestResolveFunctionABIAndDef(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback)
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:         pldapi.TransactionTypePublic.Enum(),
			ABIReference: confutil.P(tktypes.Bytes32(tktypes.RandBytes(32))),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012202", err)
}

func TestResolveFunctionNoABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback)
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012218", err)
}

func TestResolveFunctionBadABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback)
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "wrong"}}}},
	})
	assert.Regexp(t, "PD012203.*FF22025", err)
}

func mockInsertABI(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectBegin()
	mockInsertABINoBegin(conf, mc)
}

func mockInsertABINoBegin(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
}

func TestResolveFunctionNamedWithNoTarget(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "doIt",
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "doIt"}},
	})
	assert.Regexp(t, "PD012204", err)
}

func mockInsertABIAndTransactionOK(commit bool) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
		mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows)
		if commit {
			mc.db.ExpectCommit()
		}
	}
}

func mockQueryPublicTxForTransactions(cb func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*pldapi.PublicTx, error)) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].([]uuid.UUID), args[3].(*query.QueryJSON))
			mqb.Return(result, err)
		})
	}
}

func mockQueryPublicTxWithBindings(cb func(jq *query.QueryJSON) ([]*pldapi.PublicTxWithBinding, error)) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("QueryPublicTxWithBindings", mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].(*query.QueryJSON))
			mqb.Return(result, err)
		})
	}
}

func mockGetPublicTransactionForHash(cb func(hash tktypes.Bytes32) (*pldapi.PublicTxWithBinding, error)) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("GetPublicTransactionForHash", mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].(tktypes.Bytes32))
			mqb.Return(result, err)
		})
	}
}

func TestSubmitBadFromAddr(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			kr := mockKeyResolverForFail(t, mc)
			kr.On("ResolveKey", "sender1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(nil, fmt.Errorf("bad address"))
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "bad address", err)
}

func TestResolveFunctionHexInputOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		mockSubmitPublicTxOk(t, tktypes.RandAddress()),
	)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionHexInputFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`"0x"`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestResolveFunctionUnsupportedInput(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`false`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012212", err)
}

func TestResolveFunctionPlainNameOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockSubmitPublicTxOk(t, tktypes.RandAddress()))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     "sender1",
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateDeploy(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockKeyResolutionContextOk(t),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Constructor}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:   "me",
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: "domain1",
			Data:   tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateInvoke(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockDomainContractResolve(t, "domain1"), mockKeyResolutionContextOk(t),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateInvokeFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(false), mockDomainContractResolve(t, "domain1"), mockKeyResolutionContextFail(t),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveFunctionOnlyOneToMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockSubmitPublicTxOk(t, tktypes.RandAddress()))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionOnlyDuplicateMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Name: "polymorphic", Type: "string"}}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012205", err)
}

func TestResolveFunctionNoMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "nope",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012206", err)
}

func TestParseInputsBadTxType(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012211", err)
}

func TestParseInputsPrivateLookupFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePrivate.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestParseInputsPrivateDomainMismatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback, mockDomainContractResolve(t, "domain1"))
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Domain: "domain2",
			Type:   pldapi.TransactionTypePrivate.Enum(),
			To:     tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
	})
	assert.Regexp(t, "PD012231", err)
}

func TestParseInputsPrivateDeployNoDomain(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockKeyResolutionContextFail(t), mockBeginRollback)
	defer done()

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePrivate.Enum(),
		},
	})
	assert.Regexp(t, "PD012232", err)
}

func TestParseInputsBadFromRemoteNode(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "me@node2",
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012230", err)
}

func TestParseInputsBytecodeNonConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI:      exampleABI,
		Bytecode: tktypes.HexBytes(tktypes.RandBytes(1)),
	})
	assert.Regexp(t, "PD012207", err)
}

func TestParseInputsBytecodeMissingConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Constructor, Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012210", err)
}

func TestParseInputsBadDataJSON(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`{!!! bad json`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestParseInputsBadDataForFunction(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`["not a number"]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "FF22030", err)
}

func TestParseInputsBadByteString(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, mockKeyResolutionContextFail(t))
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`"not hex"`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestInsertTransactionFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnError(fmt.Errorf("pop"))
			mc.db.ExpectRollback()
			mockResolveKeyOKThenFail(t, mc, "sender1", tktypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mockResolveKeyOKThenFail(t, mc, "sender1", tktypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareReject(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mockResolveKeyOKThenFail(t, mc, "sender1", tktypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows)
			mc.publicTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
		})
	defer done()

	// Default public constructor invoke - no ABI or data
	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
		},
		Bytecode: tktypes.HexBytes(tktypes.RandBytes(1)),
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionOkDefaultConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		mockSubmitPublicTxOk(t, tktypes.RandAddress()))
	defer done()

	// Default public constructor invoke
	_, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
		},
		ABI:      abi.ABI{{Name: "notConstructor", Type: abi.Function, Inputs: abi.ParameterArray{}}},
		Bytecode: tktypes.HexBytes(tktypes.RandBytes(1)),
	})
	assert.NoError(t, err)
}

func TestCheckIdempotencyKeyNoOverrideErrIfFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("crackle"))
		})
	defer done()

	// Default public constructor invoke
	err := txm.checkIdempotencyKeys(ctx, fmt.Errorf("pop"), []*pldapi.TransactionInput{{TransactionBase: pldapi.TransactionBase{
		IdempotencyKey: "idem1",
	}}})
	assert.Regexp(t, "pop", err)
}

func TestGetPublicTxDataErrors(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.getPublicTxData(ctx, &abi.Entry{Type: abi.Event}, nil, nil)
	assert.Regexp(t, "PD011929", err)

	_, err = txm.getPublicTxData(ctx, &abi.Entry{Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Type: "wrong"},
	}}, nil, nil)
	assert.Regexp(t, "FF22041", err)

}

func TestCallTransactionNoFrom(t *testing.T) {
	ec := ethclient.NewUnconnectedRPCClient(context.Background(), &pldconf.EthClientConfig{}, 0)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.ethClientFactory.On("HTTPClient").Return(ec)
		})
	defer done()

	tx := pldclient.New().ForABI(ctx,
		abi.ABI{
			{
				Name: "getSpins",
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "wheel", Type: "string"},
				},
				Outputs: abi.ParameterArray{
					{Name: "times", Type: "uint256"},
				},
			},
		}).
		Function("getSpins").
		Public().
		To(tktypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		BuildTX()
	require.NoError(t, tx.Error())

	var result any
	err := txm.CallTransaction(ctx, &result, tx.CallTX())
	require.Regexp(t, "PD011517", err) // means we successfully submitted it to the client

}

func TestCallTransactionWithFrom(t *testing.T) {
	ec := ethclient.NewUnconnectedRPCClient(context.Background(), &pldconf.EthClientConfig{}, 0)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.keyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "red.one").
				Return(tktypes.RandAddress(), nil)
			mc.ethClientFactory.On("HTTPClient").Return(ec)
		})
	defer done()

	tx := pldclient.New().ForABI(ctx,
		abi.ABI{
			{
				Name: "getSpins",
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "wheel", Type: "string"},
				},
				Outputs: abi.ParameterArray{
					{Name: "times", Type: "uint256"},
				},
			},
		}).
		Function("getSpins").
		Public().
		From("red.one").
		To(tktypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		BuildTX()
	require.NoError(t, tx.Error())

	var result any
	err := txm.CallTransaction(ctx, &result, tx.CallTX())
	require.Regexp(t, "PD011517", err) // means we successfully submitted it to the client

}

func TestCallTransactionBadTX(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	var result any
	err := txm.CallTransaction(ctx, &result, &pldapi.TransactionCall{})
	require.Regexp(t, "PD012211", err)

}

func TestCallTransactionPrivOk(t *testing.T) {
	fnDef := &abi.Entry{Name: "getSpins", Type: abi.Function,
		Inputs: abi.ParameterArray{
			{Name: "wheel", Type: "string"},
		},
		Outputs: abi.ParameterArray{
			{Name: "spins", Type: "uint256"},
		},
	}

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin,
		mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			res, err := fnDef.Outputs.ParseJSON([]byte(`{"spins": 42}`))
			require.NoError(t, err)

			mc.privateTxMgr.On("CallPrivateSmartContract", mock.Anything, mock.Anything).
				Return(res, nil)
		})
	defer done()

	tx := pldclient.New().ForABI(ctx, abi.ABI{fnDef}).
		Function("getSpins").
		Private().
		Domain("domain1").
		To(tktypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		DataFormat("mode=array&number=json-number").
		BuildTX()
	require.NoError(t, tx.Error())

	var result tktypes.RawJSON
	err := txm.CallTransaction(ctx, &result, tx.CallTX())
	require.NoError(t, err)
	require.JSONEq(t, `[42]`, result.Pretty())

}

func TestCallTransactionPrivFail(t *testing.T) {
	fnDef := &abi.Entry{Name: "ohSnap", Type: abi.Function}

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin,
		mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("CallPrivateSmartContract", mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("snap"))
		})
	defer done()

	tx := pldclient.New().ForABI(ctx, abi.ABI{fnDef}).
		Function("ohSnap").
		Private().
		Domain("domain1").
		To(tktypes.RandAddress()).
		BuildTX()
	require.NoError(t, tx.Error())

	var result tktypes.RawJSON
	err := txm.CallTransaction(ctx, &result, tx.CallTX())
	assert.Regexp(t, "snap", err)

}

func TestCallTransactionPrivMissingTo(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin)
	defer done()

	err := txm.CallTransaction(ctx, nil, &pldapi.TransactionCall{
		TransactionInput: pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:   pldapi.TransactionTypePrivate.Enum(),
				Domain: "domain1",
			},
		},
	})
	assert.Regexp(t, "PD012222", err)

}

func TestCallTransactionBadSerializer(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABINoBegin)
	defer done()

	err := txm.CallTransaction(ctx, nil, &pldapi.TransactionCall{
		TransactionInput: pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				Type:   pldapi.TransactionTypePrivate.Enum(),
				Domain: "domain1",
			},
		},
		DataFormat: "wrong",
	})
	assert.Regexp(t, "PD020015", err)

}

var testInternalTransactionFn = &abi.Entry{Type: abi.Function, Name: "doStuff"}

func newTestInternalTransaction(idempotencyKey string) *pldapi.TransactionInput {
	return &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:           "me",
			Type:           pldapi.TransactionTypePrivate.Enum(),
			Domain:         "domain1",
			Function:       "doStuff",
			To:             tktypes.RandAddress(),
			IdempotencyKey: idempotencyKey,
		},
		ABI: abi.ABI{testInternalTransactionFn},
	}
}

func TestInternalPrivateTXInsertWithIdempotencyKeys(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"))
	defer done()

	fifteenPostCommits := make([]func(), 15)
	fifteenTxns := make([]*components.ValidatedTransaction, 15)
	err := txm.p.DB().Transaction(func(dbTX *gorm.DB) (err error) {
		for i := range fifteenTxns {
			tx := newTestInternalTransaction(fmt.Sprintf("tx_%.3d", i))
			// We do a dep chain
			if i > 0 {
				tx.DependsOn = []uuid.UUID{*fifteenTxns[i-1].Transaction.ID}
			}
			fifteenPostCommits[i], fifteenTxns[i], err = txm.PrepareInternalPrivateTransaction(ctx, dbTX, tx, pldapi.SubmitModeAuto)
			require.NoError(t, err)
		}
		return nil
	})
	require.NoError(t, err)
	for _, pc := range fifteenPostCommits {
		pc()
	}

	// Insert first 10 in a Txn
	var postCommit func()
	err = txm.p.DB().Transaction(func(dbTX *gorm.DB) (err error) {
		postCommit, err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, fifteenTxns[0:10])
		return err
	})
	require.NoError(t, err)
	postCommit()

	// Insert 5-15 in the second txn so with an overlap
	err = txm.p.DB().Transaction(func(dbTX *gorm.DB) (err error) {
		postCommit, err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, fifteenTxns[5:15])
		return err
	})
	require.NoError(t, err)
	postCommit()

	// Check we can get each back
	idemQueryKeys := make([]any, len(fifteenTxns))
	for _, expected := range fifteenTxns {
		tx, err := txm.GetTransactionByID(ctx, *expected.Transaction.ID)
		require.NoError(t, err)
		require.Equal(t, expected.Transaction.IdempotencyKey, tx.IdempotencyKey)

		rtx, err := txm.GetResolvedTransactionByID(ctx, *expected.Transaction.ID)
		require.NoError(t, err)
		require.Equal(t, tx, rtx.Transaction)
		require.NotNil(t, rtx.Function)

		idemQueryKeys = append(idemQueryKeys, expected.Transaction.IdempotencyKey)
	}

	// Check we can query them in bulk (as we would to poll the DB to perform TX management)
	rtxs, err := txm.QueryTransactionsResolved(ctx, query.NewQueryBuilder().Limit(15).In("idempotencyKey", idemQueryKeys).Sort("created").Query(), txm.p.DB(), true)
	require.NoError(t, err)
	require.Len(t, rtxs, 15)
	for i, rtx := range rtxs {
		if i > 0 {
			require.Equal(t, []uuid.UUID{*rtxs[i-1].Transaction.ID}, rtx.DependsOn)
		}
		require.NotNil(t, rtx.Function)
	}

}

func TestPrepareInternalPrivateTransactionNoIdempotencyKey(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, _, err := txm.PrepareInternalPrivateTransaction(ctx, txm.p.DB(), &pldapi.TransactionInput{}, pldapi.SubmitModeAuto)
	assert.Regexp(t, "PD012223", err)

}

func TestUpsertInternalPrivateTxsFinalizeIDsInsertFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	postCommit, tx, err := txm.PrepareInternalPrivateTransaction(ctx, txm.p.DB(), newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
	require.NoError(t, err)
	postCommit()

	_, err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, txm.p.DB(), []*components.ValidatedTransaction{tx})
	assert.Regexp(t, "pop", err)

}

func TestUpsertInternalPrivateTxsIdempotencyKeyFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows) // empty result when we expect one
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	postCommit, tx, err := txm.PrepareInternalPrivateTransaction(ctx, txm.p.DB(), newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
	require.NoError(t, err)
	postCommit()

	_, err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, txm.p.DB(), []*components.ValidatedTransaction{tx})
	assert.Regexp(t, "pop", err)

}

func TestUpsertInternalPrivateTxsIdempotencyMisMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows)      // empty result when we expect one
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(mc.db.NewRows([]string{})) // definitely should get one
		})
	defer done()

	postCommit, tx, err := txm.PrepareInternalPrivateTransaction(ctx, txm.p.DB(), newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
	require.NoError(t, err)
	postCommit()

	_, err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, txm.p.DB(), []*components.ValidatedTransaction{tx})
	assert.Regexp(t, "PD012224", err)

}
