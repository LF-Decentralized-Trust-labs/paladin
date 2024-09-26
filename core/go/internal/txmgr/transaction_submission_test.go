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
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResolveFunctionABIAndDef(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:         ptxapi.TransactionTypePublic.Enum(),
			ABIReference: confutil.P(tktypes.Bytes32(tktypes.RandBytes(32))),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012202", err)
}

func TestResolveFunctionNoABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012218", err)
}

func TestResolveFunctionBadABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "wrong"}}}},
	})
	assert.Regexp(t, "PD012203.*FF22025", err)
}

func mockInsertABI(conf *Config, mc *mockComponents) {
	mc.db.ExpectBegin()
	mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectExec("INSERT.*abi_errors").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectCommit()
}

func TestResolveFunctionNamedWithNoTarget(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: "doIt",
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "doIt"}},
	})
	assert.Regexp(t, "PD012204", err)
}

func mockInsertABIAndTransactionOK(conf *Config, mc *mockComponents) {
	mc.db.ExpectBegin()
	mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectExec("INSERT.*abi_errors").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectCommit()
	mc.db.ExpectBegin()
	mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectCommit()
}

func mockPublicSubmitTxOk(t *testing.T) func(conf *Config, mc *mockComponents) {
	return func(conf *Config, mc *mockComponents) {
		mockSubmissionBatch := componentmocks.NewPublicTxBatch(t)
		mockSubmissionBatch.On("Rejected").Return([]components.PublicTxRejected{})
		mockSubmissionBatch.On("Submit", mock.Anything, mock.Anything).Return(nil)
		mockSubmissionBatch.On("Completed", mock.Anything, true).Return(nil)
		mc.publicTxMgr.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockSubmissionBatch, nil)
	}
}

func mockQueryPublicTxForTransactions(cb func(ids []uuid.UUID, jq *query.QueryJSON) (map[uuid.UUID][]*ptxapi.PublicTx, error)) func(conf *Config, mc *mockComponents) {
	return func(conf *Config, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].([]uuid.UUID), args[3].(*query.QueryJSON))
			mqb.Return(result, err)
		})
	}
}

func mockQueryPublicTxWithBindings(cb func(jq *query.QueryJSON) ([]*ptxapi.PublicTxWithBinding, error)) func(conf *Config, mc *mockComponents) {
	return func(conf *Config, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("QueryPublicTxWithBindings", mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].(*query.QueryJSON))
			mqb.Return(result, err)
		})
	}
}

func mockGetPublicTransactionForHash(cb func(hash tktypes.Bytes32) (*ptxapi.PublicTxWithBinding, error)) func(conf *Config, mc *mockComponents) {
	return func(conf *Config, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("GetPublicTransactionForHash", mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].(tktypes.Bytes32))
			mqb.Return(result, err)
		})
	}
}

func TestResolveFunctionHexInputOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, mockPublicSubmitTxOk(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionHexInputFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`"0x"`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestResolveFunctionUnsupportedInput(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`false`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012212", err)
}

func TestResolveFunctionPlainNameOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, mockPublicSubmitTxOk(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateDeploy(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, func(conf *Config, mc *mockComponents) {
		mc.privateTxMgr.On("HandleDeployTx", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Constructor}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:   ptxapi.TransactionTypePrivate.Enum(),
			Domain: "domain1",
			Data:   tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateInvoke(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, func(conf *Config, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePrivate.Enum(),
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
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, func(conf *Config, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePrivate.Enum(),
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
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABIAndTransactionOK, mockPublicSubmitTxOk(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionOnlyDuplicateMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Name: "polymorphic", Type: "string"}}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012205", err)
}

func TestResolveFunctionNoMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: "nope",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012206", err)
}

func TestParseInputsBadTxType(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012211", err)
}

func TestParseInputsBytecodeNonConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI:      exampleABI,
		Bytecode: tktypes.HexBytes(tktypes.RandBytes(1)),
	})
	assert.Regexp(t, "PD012207", err)
}

func TestParseInputsBytecodeMissingConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Constructor, Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			Data: tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012210", err)
}

func TestParseInputsBadDataJSON(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`{!!! bad json`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestParseInputsBadDataForFunction(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`["not a number"]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "FF22030", err)
}

func TestParseInputsBadByteString(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type: ptxapi.TransactionTypePublic.Enum(),
			To:   tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data: tktypes.RawJSON(`"not hex"`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func mockPublicSubmitTxRollback(t *testing.T) func(conf *Config, mc *mockComponents) {
	return func(conf *Config, mc *mockComponents) {
		mockSubmissionBatch := componentmocks.NewPublicTxBatch(t)
		mockSubmissionBatch.On("Rejected").Return([]components.PublicTxRejected{})
		mockSubmissionBatch.On("Completed", mock.Anything, false).Return(nil)
		mc.publicTxMgr.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockSubmissionBatch, nil)
	}
}
func TestInsertTransactionFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, func(conf *Config, mc *mockComponents) {
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
		mc.db.ExpectExec("INSERT.*abi_errors").WillReturnResult(driver.ResultNoRows)
		mc.db.ExpectCommit()
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT.*transactions").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	}, mockPublicSubmitTxRollback(t))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI, func(conf *Config, mc *mockComponents) {
		mockSubmissionBatch := componentmocks.NewPublicTxBatch(t)
		rejectedSubmission := componentmocks.NewPublicTxRejected(t)
		rejectedSubmission.On("RejectedError").Return(fmt.Errorf("pop"))
		mockSubmissionBatch.On("Rejected").Return([]components.PublicTxRejected{
			rejectedSubmission,
		})
		mockSubmissionBatch.On("Completed", mock.Anything, false).Return(nil)
		mc.publicTxMgr.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(mockSubmissionBatch, nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareReject(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false, mockInsertABI, func(conf *Config, mc *mockComponents) {
		mc.publicTxMgr.On("PrepareSubmissionBatch", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.sendTransaction(ctx, &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			Type:     ptxapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}
