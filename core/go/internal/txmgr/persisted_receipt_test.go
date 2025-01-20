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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinalizeTransactionsNoOp(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	fn, err := txm.FinalizeTransactions(ctx, txm.p.DB(), nil)
	assert.NoError(t, err)
	fn()

}

func TestFinalizeTransactionsSuccessWithFailure(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.FinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_Success,
			FailureMessage: "not empty",
		},
	})
	assert.Regexp(t, "PD012213", err)
}

func TestFinalizeTransactionsBadType(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.FinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.ReceiptType(42)}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithMessageNoMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.FinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsFailedWithRevertDataWithMessage(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	_, err := txm.FinalizeTransactions(ctx, txm.p.DB(), []*components.ReceiptInput{
		{TransactionID: txID, ReceiptType: components.RT_FailedOnChainWithRevertData,
			FailureMessage: "not empty"}})
	assert.Regexp(t, "PD012213", err)

}

func TestFinalizeTransactionsInsertFail(t *testing.T) {

	txID := uuid.New()
	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectBegin()
			mc.db.ExpectExec("INSERT.*transaction_receipts").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.p.DB().Transaction(func(tx *gorm.DB) error {
		_, err := txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{TransactionID: txID, ReceiptType: components.RT_FailedWithMessage,
				FailureMessage: "something went wrong"},
		})
		return err
	})
	assert.Regexp(t, "pop", err)

}

func mockKeyResolutionContextOk(t *testing.T) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		_ = mockKeyResolver(t, mc)
	}
}

func mockKeyResolver(t *testing.T, mc *mockComponents) *componentmocks.KeyResolver {
	krc := componentmocks.NewKeyResolutionContext(t)
	kr := componentmocks.NewKeyResolver(t)
	krc.On("KeyResolver", mock.Anything).Return(kr)
	krc.On("PreCommit").Return(nil)
	krc.On("Close", mock.Anything).Return()
	mc.keyManager.On("NewKeyResolutionContext", mock.Anything).Return(krc)
	return kr
}

func mockKeyResolutionContextFail(t *testing.T) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		_ = mockKeyResolverForFail(t, mc)
	}
}

func mockKeyResolverForFail(t *testing.T, mc *mockComponents) *componentmocks.KeyResolver {
	krc := componentmocks.NewKeyResolutionContext(t)
	kr := componentmocks.NewKeyResolver(t)
	krc.On("KeyResolver", mock.Anything).Return(kr)
	krc.On("Close", false).Return()
	mc.keyManager.On("NewKeyResolutionContext", mock.Anything).Return(krc)
	return kr
}

func mockDomainContractResolve(t *testing.T, domainName string, contractAddrs ...tktypes.EthAddress) func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
	return func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mgsc := mc.domainManager.On("GetSmartContractByAddress", mock.Anything, mock.Anything, mock.MatchedBy(func(a tktypes.EthAddress) bool {
			if len(contractAddrs) == 0 {
				return true
			}
			for _, contractAddr := range contractAddrs {
				if contractAddr == a {
					return true
				}
			}
			return false
		}))
		mgsc.Run(func(args mock.Arguments) {
			mpsc := componentmocks.NewDomainSmartContract(t)
			mdmn := componentmocks.NewDomain(t)
			mdmn.On("Name").Return(domainName)
			mpsc.On("Domain").Return(mdmn)
			mpsc.On("Address").Return(args[2].(tktypes.EthAddress)).Maybe()
			mgsc.Return(mpsc, nil)
		})
	}
}

func TestFinalizeTransactionsInsertOkOffChain(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, mockKeyResolutionContextOk(t), mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     "me",
			Type:     pldapi.TransactionTypePrivate.Enum(),
			Function: "doIt",
			To:       tktypes.MustEthAddress(tktypes.RandHex(20)),
			Data:     tktypes.JSONString(tktypes.HexBytes(callData)),
		},
		ABI: exampleABI,
	})
	require.NoError(t, err)

	var postCommit func()
	err = txm.p.DB().Transaction(func(tx *gorm.DB) (err error) {
		postCommit, err = txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				ReceiptType:   components.RT_FailedOnChainWithRevertData,
			},
		})
		return err
	})
	require.NoError(t, err)
	postCommit()

	receipt, err := txm.GetTransactionReceiptByID(ctx, *txID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"sequence":%d,
		"failureMessage":"PD012214: Unable to decode revert data (no revert data available)"
	}`, txID, receipt.Sequence), string(tktypes.JSONString(receipt)))

}

func TestFinalizeTransactionsInsertOkEvent(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, true, mockKeyResolutionContextOk(t), mockDomainContractResolve(t, "domain1"), func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
		mc.privateTxMgr.On("HandleNewTx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		mc.stateMgr.On("GetTransactionStates", mock.Anything, mock.Anything, mock.Anything).Return(
			&pldapi.TransactionStates{None: true}, nil,
		)

		md := componentmocks.NewDomain(t)
		mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(md, nil)
		md.On("BuildDomainReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not available"))
	})
	defer done()

	exampleABI := abi.ABI{{Type: abi.Function, Name: "doIt"}}
	callData, err := exampleABI[0].EncodeCallDataJSON([]byte(`[]`))
	require.NoError(t, err)

	txID, err := txm.SendTransaction(ctx, &pldapi.TransactionInput{
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

	var postCommit func()
	err = txm.p.DB().Transaction(func(tx *gorm.DB) (err error) {
		postCommit, err = txm.FinalizeTransactions(ctx, tx, []*components.ReceiptInput{
			{
				TransactionID: *txID,
				Domain:        "domain1",
				ReceiptType:   components.RT_Success,
				OnChain: tktypes.OnChainLocation{
					Type:             tktypes.OnChainEvent,
					TransactionHash:  tktypes.MustParseBytes32("d0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30"),
					BlockNumber:      12345,
					TransactionIndex: 10,
					LogIndex:         5,
					Source:           tktypes.MustEthAddress("0x3f9f796ff55589dd2358c458f185bbed357c0b6e"),
				},
			},
		})
		return err
	})
	require.NoError(t, err)
	postCommit()

	receipt, err := txm.GetTransactionReceiptByIDFull(ctx, *txID)
	require.NoError(t, err)

	require.NotNil(t, receipt)
	require.JSONEq(t, fmt.Sprintf(`{
		"id":"%s",
		"sequence":%d,
		"domain": "domain1",
		"blockNumber":12345, 
		"logIndex":5,
	 	"source":"0x3f9f796ff55589dd2358c458f185bbed357c0b6e",
	  	"success":true, 
	  	"transactionHash":"0xd0561b310b77e47bc16fb3c40d48b72255b1748efeecf7452373dfce8045af30", 
		"transactionIndex":10,
		"states": {"none": true},
		"domainReceiptError": "not available"
	}`, txID, receipt.Sequence), tktypes.JSONString(receipt).Pretty())

}

func TestCalculateRevertErrorNoData(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
	)
	defer done()

	err := txm.CalculateRevertError(ctx, nil, nil)
	assert.Regexp(t, "PD012214", err)

}

func TestCalculateRevertErrorQueryFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnError(fmt.Errorf("pop"))
		})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.DB(), []byte("any data"))
	assert.Regexp(t, "PD012221.*pop", err)

}

func TestCalculateRevertErrorDecodeFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnRows(sqlmock.NewRows([]string{"definition"}).AddRow(`{}`))
		})
	defer done()

	err := txm.CalculateRevertError(ctx, txm.p.DB(), []byte("any data"))
	assert.Regexp(t, "PD012221", err)

}

func TestGetTransactionReceiptNoResult(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByID(ctx, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, res)

}

func TestGetTransactionReceiptFullNoResult(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*transaction_receipts").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	res, err := txm.GetTransactionReceiptByIDFull(ctx, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, res)

}

func TestGetDomainReceiptFail(t *testing.T) {

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.domainManager.On("GetDomainByName", mock.Anything, "domain1").Return(nil, fmt.Errorf("not found"))
		})
	defer done()

	_, err := txm.GetDomainReceiptByID(ctx, "domain1", uuid.New())
	assert.Regexp(t, "not found", err)

}

func TestDecodeRevertErrorBadSerializer(t *testing.T) {
	revertReasonTooSmallHex := tktypes.MustParseHexBytes("0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001d5468652073746f7265642076616c756520697320746f6f20736d616c6c000000")

	ctx, txm, done := newTestTransactionManager(t, false,
		mockEmptyReceiptListeners,
		func(conf *pldconf.TxManagerConfig, mc *mockComponents) {
			mc.db.ExpectQuery("SELECT.*abi_entries").WillReturnRows(sqlmock.NewRows([]string{}))
		})
	defer done()

	_, err := txm.DecodeRevertError(ctx, txm.p.DB(), revertReasonTooSmallHex, "wrong")
	assert.Regexp(t, "PD020015", err)

}

func TestDecodeCall(t *testing.T) {

	sampleABI := abi.ABI{
		{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{
			{Type: "uint256", Name: "newValue"},
		}},
	}

	ctx, txm, done := newTestTransactionManager(t, true)
	defer done()

	postCommit, _, err := txm.storeABI(ctx, txm.p.DB(), sampleABI)
	require.NoError(t, err)
	postCommit()

	validCall, err := sampleABI.Functions()["set"].EncodeCallDataJSON([]byte(`[12345]`))
	require.NoError(t, err)

	decoded, err := txm.DecodeCall(ctx, txm.p.DB(), validCall, "")
	assert.NoError(t, err)
	require.JSONEq(t, `{"newValue": "12345"}`, string(decoded.Data))
	require.Equal(t, `set(uint256)`, string(decoded.Signature))

	invalidCall := append(sampleABI.Functions()["set"].FunctionSelectorBytes(), []byte{0x00}...)
	_, err = txm.DecodeCall(ctx, txm.p.DB(), tktypes.HexBytes(invalidCall), "")
	assert.Regexp(t, "PD012227.*1 matched function selector", err)

	short := []byte{0xfe, 0xed}
	_, err = txm.DecodeCall(ctx, txm.p.DB(), tktypes.HexBytes(short), "")
	assert.Regexp(t, "PD012226", err)

	_, err = txm.DecodeCall(ctx, txm.p.DB(), validCall, "wrong")
	assert.Regexp(t, "PD020015", err)

}

func TestDecodeEvent(t *testing.T) {

	sampleABI := abi.ABI{
		{Type: abi.Event, Name: "Updated", Inputs: abi.ParameterArray{
			{Type: "uint256", Name: "newValue", Indexed: true},
		}},
	}

	ctx, txm, done := newTestTransactionManager(t, true)
	defer done()

	postCommit, _, err := txm.storeABI(ctx, txm.p.DB(), sampleABI)
	require.NoError(t, err)
	postCommit()

	validTopic0 := tktypes.Bytes32(sampleABI.Events()["Updated"].SignatureHashBytes())
	validTopic1, err := (&abi.ParameterArray{{Type: "uint256"}}).EncodeABIDataJSON([]byte(`["12345"]`))
	require.NoError(t, err)

	decoded, err := txm.DecodeEvent(ctx, txm.p.DB(), []tktypes.Bytes32{validTopic0, tktypes.Bytes32(validTopic1)}, []byte{}, "")
	assert.NoError(t, err)
	require.JSONEq(t, `{"newValue": "12345"}`, string(decoded.Data))
	require.Equal(t, `Updated(uint256)`, string(decoded.Signature))

	_, err = txm.DecodeEvent(ctx, txm.p.DB(), []tktypes.Bytes32{validTopic0 /* missing 2nd topic*/}, []byte{}, "")
	assert.Regexp(t, "PD012229.*1 matched signature", err)

	_, err = txm.DecodeEvent(ctx, txm.p.DB(), []tktypes.Bytes32{tktypes.Bytes32(tktypes.RandBytes(32)) /* unknown topic */}, []byte{}, "")
	assert.Regexp(t, "PD012229", err)

	_, err = txm.DecodeEvent(ctx, txm.p.DB(), []tktypes.Bytes32{ /* no topics */ }, []byte{}, "")
	assert.Regexp(t, "PD012226", err)

	_, err = txm.DecodeEvent(ctx, txm.p.DB(), []tktypes.Bytes32{validTopic0, tktypes.Bytes32(validTopic1)}, []byte{}, "wrong")
	assert.Regexp(t, "PD020015", err)

}
