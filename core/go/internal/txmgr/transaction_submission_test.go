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
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
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
		mockBeginRollback)
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:         pldapi.TransactionTypePublic.Enum(),
			ABIReference: confutil.P(pldtypes.RandBytes32()),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012202", err)
}

func TestResolveFunctionNoABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback)
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
		},
		ABI: abi.ABI{},
	})
	assert.Regexp(t, "PD012218", err)
}

func TestResolveFunctionBadABI(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback)
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "wrong"}}}},
	})
	assert.Regexp(t, "PD012203.*FF22025", err)
}

func mockInsertABI(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectBegin()
	mockInsertABINoBegin(mc)
}

func mockInsertABINoBegin(mc *mockComponents) {
	mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
	mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
}

func mockInsertABIBeginCommit(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	mc.db.ExpectBegin()
	mockInsertABINoBegin(mc)
	mc.db.ExpectCommit()
}

func TestResolveFunctionNamedWithNoTarget(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
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
		mc.db.ExpectExec("INSERT.*transaction_history").WillReturnResult(driver.ResultNoRows)

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

func mockGetPublicTransactionForHash(cb func(hash pldtypes.Bytes32) (*pldapi.PublicTxWithBinding, error)) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mqb := mc.publicTxMgr.On("GetPublicTransactionForHash", mock.Anything, mock.Anything, mock.Anything)
		mqb.Run(func(args mock.Arguments) {
			result, err := cb(args[2].(pldtypes.Bytes32))
			mqb.Return(result, err)
		})
	}
}

func TestSubmitBadFromAddr(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			kr := mockKeyResolver(t, mc)
			kr.On("ResolveKey", mock.Anything, "sender1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS).Return(nil, fmt.Errorf("bad address"))
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "bad address", err)
}

func TestResolveFunctionHexInputOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		mockSubmitPublicTxOk(t, pldtypes.RandAddress()),
	)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionHexInputFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`"0x"`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestResolveFunctionUnsupportedInput(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}}}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`false`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012212", err)
}

func TestResolveFunctionPlainNameOK(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockSubmitPublicTxOk(t, pldtypes.RandAddress()))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     "sender1",
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSubmitEthAddrOK(t *testing.T) {
	addr := pldtypes.RandAddress()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		mockSubmitPublicTxOk(t, addr),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mapping := &pldapi.KeyMappingAndVerifier{
				KeyMappingWithPath: &pldapi.KeyMappingWithPath{
					KeyMapping: &pldapi.KeyMapping{
						Identifier: "sender1",
					},
				},
			}
			mc.keyManager.On("ReverseKeyLookup", mock.Anything, mock.Anything, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, addr.String()).Return(mapping, nil)
		},
	)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     fmt.Sprintf("eth_address:%s", addr.String()),
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSubmitVerifierNotEthAddr(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI,
	)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     "eth_address:banana",
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.ErrorContains(t, err, "PD012253")
}

func TestSubmitVerifierNotFound(t *testing.T) {
	addr := pldtypes.RandAddress()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.keyManager.On("ReverseKeyLookup", mock.Anything, mock.Anything, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, addr.String()).Return(nil, errors.New("not found"))
		},
	)
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     fmt.Sprintf("eth_address:%s", addr.String()),
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.ErrorContains(t, err, "not found")
}

func TestSendTransactionPrivateDeploy(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Constructor}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:   "me",
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: "domain1",
			Data:   pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateInvoke(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestSendTransactionPrivateInvokeFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(false), mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Domain:   "domain1",
			Function: "doIt",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestResolveFunctionOnlyOneToMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true), mockSubmitPublicTxOk(t, pldtypes.RandAddress()))
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.NoError(t, err)
}

func TestResolveFunctionOnlyDuplicateMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Name: "polymorphic", Type: "string"}}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012205", err)
}

func TestResolveFunctionNoMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: "nope",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012206", err)
}

func TestParseInputsBadTxType(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012211", err)
}

func TestParseInputsPrivateLookupFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePrivate.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestParseInputsPrivateDomainMismatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback, mockDomainContractResolve(t, "domain1"))
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Domain: "domain2",
			Type:   pldapi.TransactionTypePrivate.Enum(),
			To:     pldtypes.MustEthAddress(pldtypes.RandHex(20)),
		},
	})
	assert.Regexp(t, "PD012231", err)
}

func TestParseInputsPrivateDeployNoDomain(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockBeginRollback)
	defer done()

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePrivate.Enum(),
		},
	})
	assert.Regexp(t, "PD012232", err)
}

func TestParseInputsBadFromRemoteNode(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "me@node2",
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012230", err)
}

func TestParseInputsBytecodeNonConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI:      exampleABI,
		Bytecode: pldtypes.HexBytes(pldtypes.RandBytes(1)),
	})
	assert.Regexp(t, "PD012207", err)
}

func TestParseInputsBytecodeMissingConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Constructor, Inputs: abi.ParameterArray{}},
	}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	_, err = txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			Data: pldtypes.JSONString(pldtypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012210", err)
}

func TestParseInputsBadDataJSON(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.RawJSON(`{!!! bad json`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "PD012208", err)
}

func TestParseInputsBadDataForFunction(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.RawJSON(`["not a number"]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "FF22030", err)
}

func TestParseInputsBadByteString(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI)
	defer done()

	exampleABI := abi.ABI{
		{Type: abi.Function, Name: "doIt", Inputs: abi.ParameterArray{{Type: "uint256"}}},
	}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data: pldtypes.RawJSON(`"not hex"`),
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
			mockResolveKeyOKThenFail(t, mc, "sender1", pldtypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mockResolveKeyOKThenFail(t, mc, "sender1", pldtypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
		})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}

	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type:     pldapi.TransactionTypePublic.Enum(),
			Function: exampleABI[0].FunctionSelectorBytes().String(),
			From:     "sender1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`[]`),
		},
		ABI: exampleABI,
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionPublicTxPrepareReject(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABI, func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mockResolveKeyOKThenFail(t, mc, "sender1", pldtypes.RandAddress())
			mc.publicTxMgr.On("ValidateTransaction", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transaction_history").WillReturnResult(driver.ResultNoRows)
			mc.publicTxMgr.On("WriteNewTransactions", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
		})
	defer done()

	// Default public constructor invoke - no ABI or data
	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
		},
		Bytecode: pldtypes.HexBytes(pldtypes.RandBytes(1)),
	})
	assert.Regexp(t, "pop", err)
}

func TestInsertTransactionOkDefaultConstructor(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIAndTransactionOK(true),
		mockSubmitPublicTxOk(t, pldtypes.RandAddress()))
	defer done()

	// Default public constructor invoke
	_, err := txm.sendTransactionNewDBTX(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Type: pldapi.TransactionTypePublic.Enum(),
			From: "sender1",
		},
		ABI:      abi.ABI{{Name: "notConstructor", Type: abi.Function, Inputs: abi.ParameterArray{}}},
		Bytecode: pldtypes.HexBytes(pldtypes.RandBytes(1)),
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
		mockEmptyReceiptListeners)
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
		mockInsertABIBeginCommit,
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
		To(pldtypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		BuildTX()
	require.NoError(t, tx.Error())

	var result any
	err := txm.CallTransaction(ctx, txm.p.NOTX(), &result, tx.CallTX())
	require.Regexp(t, "PD011517", err) // means we successfully submitted it to the client

}

func TestCallTransactionWithFrom(t *testing.T) {
	ec := ethclient.NewUnconnectedRPCClient(context.Background(), &pldconf.EthClientConfig{}, 0)

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIBeginCommit,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.keyManager.On("ResolveEthAddressNewDatabaseTX", mock.Anything, "red.one").
				Return(pldtypes.RandAddress(), nil)
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
		To(pldtypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		BuildTX()
	require.NoError(t, tx.Error())

	var result any
	err := txm.CallTransaction(ctx, txm.p.NOTX(), &result, tx.CallTX())
	require.Regexp(t, "PD011517", err) // means we successfully submitted it to the client

}

func TestCallTransactionBadTX(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	var result any
	err := txm.CallTransaction(ctx, txm.p.NOTX(), &result, &pldapi.TransactionCall{})
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
		mockInsertABIBeginCommit,
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
		To(pldtypes.RandAddress()).
		Inputs(map[string]any{"wheel": "of fortune"}).
		DataFormat("mode=array&number=json-number").
		BuildTX()
	require.NoError(t, tx.Error())

	var result pldtypes.RawJSON
	err := txm.CallTransaction(ctx, txm.p.NOTX(), &result, tx.CallTX())
	require.NoError(t, err)
	require.JSONEq(t, `[42]`, result.Pretty())

}

func TestCallTransactionPrivFail(t *testing.T) {
	fnDef := &abi.Entry{Name: "ohSnap", Type: abi.Function}

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIBeginCommit,
		mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.privateTxMgr.On("CallPrivateSmartContract", mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("snap"))
		})
	defer done()

	tx := pldclient.New().ForABI(ctx, abi.ABI{fnDef}).
		Function("ohSnap").
		Private().
		Domain("domain1").
		To(pldtypes.RandAddress()).
		BuildTX()
	require.NoError(t, tx.Error())

	var result pldtypes.RawJSON
	err := txm.CallTransaction(ctx, txm.p.NOTX(), &result, tx.CallTX())
	assert.Regexp(t, "snap", err)

}

func TestCallTransactionPrivMissingTo(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockInsertABIBeginCommit)
	defer done()

	err := txm.CallTransaction(ctx, txm.p.NOTX(), nil, &pldapi.TransactionCall{
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
		mockInsertABIBeginCommit)
	defer done()

	err := txm.CallTransaction(ctx, txm.p.NOTX(), nil, &pldapi.TransactionCall{
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
			To:             pldtypes.RandAddress(),
			IdempotencyKey: idempotencyKey,
		},
		ABI: abi.ABI{testInternalTransactionFn},
	}
}

func TestInternalPrivateTXInsertWithIdempotencyKeys(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, true, mockDomainContractResolve(t, "domain1"))
	defer done()

	fifteenTxns := make([]*components.ValidatedTransaction, 15)
	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		for i := range fifteenTxns {
			tx := newTestInternalTransaction(fmt.Sprintf("tx_%.3d", i))
			// We do a dep chain
			if i > 0 {
				tx.DependsOn = []uuid.UUID{*fifteenTxns[i-1].Transaction.ID}
			}
			fifteenTxns[i], err = txm.PrepareInternalPrivateTransaction(ctx, dbTX, tx, pldapi.SubmitModeAuto)
			require.NoError(t, err)
		}
		return nil
	})
	require.NoError(t, err)

	// Insert first 10 in a Txn
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, fifteenTxns[0:10])
		return err
	})
	require.NoError(t, err)

	// Insert 5-15 in the second txn so with an overlap
	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		err = txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, fifteenTxns[5:15])
		return err
	})
	require.NoError(t, err)

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
	rtxs, err := txm.QueryTransactionsResolved(ctx, query.NewQueryBuilder().Limit(15).In("idempotencyKey", idemQueryKeys).Sort("created").Query(), txm.p.NOTX(), true)
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
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
		},
	)
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		_, err = txm.PrepareInternalPrivateTransaction(ctx, dbTX, &pldapi.TransactionInput{}, pldapi.SubmitModeAuto)
		return err
	})
	assert.Regexp(t, "PD012223", err)

}

func TestUpsertInternalPrivateTxsFinalizeIDsInsertFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		tx, err := txm.PrepareInternalPrivateTransaction(ctx, dbTX, newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
		require.NoError(t, err)

		return txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, []*components.ValidatedTransaction{tx})
	})
	assert.Regexp(t, "pop", err)

}

func TestUpsertInternalPrivateTxsIdempotencyKeyFail(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows) // empty result when we expect one
			mc.db.ExpectExec("INSERT.*transaction_history").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		tx, err := txm.PrepareInternalPrivateTransaction(ctx, dbTX, newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
		require.NoError(t, err)

		return txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, []*components.ValidatedTransaction{tx})
	})
	assert.Regexp(t, "pop", err)

}

func TestUpsertInternalPrivateTxsIdempotencyMisMatch(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		mockDomainContractResolve(t, "domain1"),
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transactions").WillReturnResult(driver.ResultNoRows) // empty result when we expect one
			mc.db.ExpectExec("INSERT.*transaction_history").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectQuery("SELECT.*transactions").WillReturnRows(mc.db.NewRows([]string{})) // definitely should get one
		})
	defer done()

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) (err error) {
		tx, err := txm.PrepareInternalPrivateTransaction(ctx, dbTX, newTestInternalTransaction("tx1"), pldapi.SubmitModeAuto)
		require.NoError(t, err)

		return txm.UpsertInternalPrivateTxsFinalizeIDs(ctx, dbTX, []*components.ValidatedTransaction{tx})
	})
	assert.Regexp(t, "PD012224", err)

}

func TestUpdateTransactionDBReadError(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnError(fmt.Errorf("pop"))
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, uuid.New(), &pldapi.TransactionInput{})
	assert.ErrorContains(t, err, "pop")
}

func TestUpdateTransactionNotFound(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(mc.db.NewRows(nil))
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, uuid.New(), &pldapi.TransactionInput{})
	assert.ErrorContains(t, err, "PD012244")
}

func TestUpdateTransactionPrivate(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"type"}
			rows := sqlmock.NewRows(columns).AddRow("private")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, uuid.New(), &pldapi.TransactionInput{})
	assert.ErrorContains(t, err, "PD012245")
}

func TestUpdateTransactionErrorQueryingPublicTX(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"type"}
			rows := sqlmock.NewRows(columns).AddRow("public")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, fmt.Errorf("pop"))
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, uuid.New(), &pldapi.TransactionInput{})
	assert.ErrorContains(t, err, "pop")
}

func TestUpdateTransactionPublicTXNotFound(t *testing.T) {
	id := uuid.New()

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"id", "type"}
			rows := sqlmock.NewRows(columns).AddRow(id.String(), "public")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[uuid.UUID][]*pldapi.PublicTx{
					id: {},
				}, nil)
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, id, &pldapi.TransactionInput{})
	assert.ErrorContains(t, err, "PD011911")
	assert.ErrorContains(t, err, id.String())
}

func TestUpdateTransactionFailResolve(t *testing.T) {
	id := uuid.New()

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"id", "type"}
			rows := sqlmock.NewRows(columns).AddRow(id.String(), "public")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[uuid.UUID][]*pldapi.PublicTx{
					id: {{LocalID: confutil.P(uint64(1))}},
				}, nil)
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, id, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Function: "set",
		},
	})
	assert.ErrorContains(t, err, "PD012203")
}

func TestUpdateTransactionKeyResolutionError(t *testing.T) {
	id := uuid.New()

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"id", "type", "from"}
			rows := sqlmock.NewRows(columns).AddRow(id.String(), "public", "identity@node1")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[uuid.UUID][]*pldapi.PublicTx{
					id: {{LocalID: confutil.P(uint64(1))}},
				}, nil)
			kr := componentmocks.NewKeyResolver(t)
			kr.On("ResolveKey", mock.Anything, "identity", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
			mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(kr)
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, id, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Function: "set",
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     "identity@node1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`{"value": 46}`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}},
	})
	assert.ErrorContains(t, err, "pop")
}

func TestUpdateTransactionKeyParseError(t *testing.T) {
	id := uuid.New()

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"id", "type", "from"}
			rows := sqlmock.NewRows(columns).AddRow(id.String(), "public", "identity@node1")
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.publicTxMgr.On("QueryPublicTxForTransactions", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(map[uuid.UUID][]*pldapi.PublicTx{
					id: {{LocalID: confutil.P(uint64(1))}},
				}, nil)
			kr := componentmocks.NewKeyResolver(t)
			kr.On("ResolveKey", mock.Anything, "identity", mock.Anything, mock.Anything).Return(&pldapi.KeyMappingAndVerifier{
				Verifier: &pldapi.KeyVerifier{
					Verifier: "not an eth address",
				},
			}, nil)
			mc.keyManager.On("KeyResolverForDBTX", mock.Anything).Return(kr)

		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, id, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Function: "set",
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     "identity@node1",
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Data:     pldtypes.RawJSON(`{"value": 46}`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}},
	})
	assert.ErrorContains(t, err, "bad address")
}

func TestUpdateTransactionCallPublicTXUpdate(t *testing.T) {
	id := uuid.New()
	from := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	to := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	abiRef := pldtypes.MustParseBytes32("0x76458e36bbb1e4f5e5742aa62b3122eb2e4622e19489dd2eb4c7370858085511")
	function := "set(uint256)"
	data := pldtypes.RawJSON(`{"value":"46"}`)
	var dbUpdateFn func(dbTX persistence.DBTX) error

	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			Function: "set",
			Type:     pldapi.TransactionTypePublic.Enum(),
			From:     from.HexString(),
			To:       to,
			Data:     pldtypes.RawJSON(`{"value": 46}`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}},
	}

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			columns := []string{"id", "type", "from"}
			rows := sqlmock.NewRows(columns).AddRow(id.String(), "public", from.HexString())
			mc.db.ExpectQuery("SELECT.*transactions.*").WillReturnRows(rows)
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectCommit()
			mc.db.ExpectBegin()
			mc.db.ExpectExec("UPDATE \"transactions\" SET \"abi_ref\"=\\$1,\"function\"=\\$2,\"to\"=\\$3,\"data\"=\\$4 WHERE id = \\$5").
				WithArgs(abiRef, function, to, data, id).
				WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*transaction_history").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectCommit()

		},
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			localID := uint64(1)
			mc.publicTxMgr.On("QueryPublicTxForTransactions",
				mock.Anything,
				mock.Anything,
				mock.Anything,
				mock.Anything,
			).
				Return(map[uuid.UUID][]*pldapi.PublicTx{
					id: {{
						LocalID: &localID,
					}},
				}, nil)
			pubTxData, _ := hex.DecodeString("60fe47b1000000000000000000000000000000000000000000000000000000000000002e")
			mockUpdateTX := mc.publicTxMgr.On("UpdateTransaction",
				mock.Anything,
				id,
				uint64(1),
				from,
				tx,
				pubTxData,
				mock.Anything,
			).Return(nil)
			mockUpdateTX.Run(func(args mock.Arguments) {
				dbUpdateFn = args.Get(6).(func(dbTX persistence.DBTX) error)
			})
		},
	)
	defer done()

	_, err := txm.UpdateTransaction(ctx, id, tx)
	require.NoError(t, err)

	err = txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		return dbUpdateFn(dbTX)
	})
	require.NoError(t, err)
}

func TestResolveUpdatedTransactionResolveFunctionError(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
	)
	defer done()

	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To: pldtypes.MustEthAddress(pldtypes.RandHex(20)),
		},
		ABI: abi.ABI{},
	}

	_, err := txm.resolveUpdatedTransaction(ctx, nil, uuid.New(), tx, nil)
	assert.ErrorContains(t, err, "PD012218")
}

func TestResolveUpdatedTransactionParseInputError(t *testing.T) {
	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
		},
	)
	defer done()

	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Function: "set",
			Data:     pldtypes.RawJSON(`{`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set"}},
	}

	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		_, err := txm.resolveUpdatedTransaction(ctx, dbTX, uuid.New(), tx, nil)
		return err
	})
	assert.ErrorContains(t, err, "PD012208")
}

func TestResolveUpdatedTransactionSuccess(t *testing.T) {
	to := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	from := pldtypes.MustEthAddress(pldtypes.RandHex(20))
	tx := &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			To:       to,
			Function: "set",
			Data:     pldtypes.RawJSON(`{"value": 46}`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}},
	}

	ctx, txm, done := newTestTransactionManager(t,
		false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*abis").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectExec("INSERT.*abi_entries").WillReturnResult(driver.ResultNoRows)
			mc.db.ExpectCommit()
		},
	)
	defer done()

	var validatedTransaction *components.ValidatedTransaction
	err := txm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		var err error
		validatedTransaction, err = txm.resolveUpdatedTransaction(ctx, dbTX, uuid.New(), tx, &pldapi.Transaction{
			TransactionBase: pldapi.TransactionBase{
				Type:           pldapi.TransactionTypePublic.Enum(),
				From:           from.String(),
				IdempotencyKey: "idempotencyKey",
			},
		})
		return err
	})

	require.NoError(t, err)
	require.NotNil(t, validatedTransaction)
	assert.Equal(t, to, validatedTransaction.Transaction.To)
	assert.Equal(t, from.String(), validatedTransaction.Transaction.From)
	assert.Equal(t, pldapi.TransactionTypePublic.Enum(), validatedTransaction.Transaction.Type)
	assert.Equal(t, "idempotencyKey", validatedTransaction.Transaction.IdempotencyKey)
	assert.Equal(t, "set(uint256)", validatedTransaction.ResolvedTransaction.Function.Signature)
	assert.Equal(t, "0x76458e36bbb1e4f5e5742aa62b3122eb2e4622e19489dd2eb4c7370858085511", validatedTransaction.ResolvedTransaction.Function.ABIReference.HexString0xPrefix())
	assert.Equal(t, `{"value":"46"}`, validatedTransaction.Transaction.Data.String())
	assert.Equal(t, "60fe47b1000000000000000000000000000000000000000000000000000000000000002e", hex.EncodeToString(validatedTransaction.PublicTxData))
}
