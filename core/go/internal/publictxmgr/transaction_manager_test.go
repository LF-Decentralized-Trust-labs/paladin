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

package publictxmgr

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/keymanager"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/metrics"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/blockindexermocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ethclientmocks"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/blockindexer"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type mocksAndTestControl struct {
	disableManagerStart bool
	allComponents       *componentsmocks.AllComponents
	db                  sqlmock.Sqlmock // unless realDB
	keyManager          components.KeyManager
	ethClientFactory    *ethclientmocks.EthClientFactory
	ethClient           *ethclientmocks.EthClient
	blockIndexer        *blockindexermocks.BlockIndexer
	txManager           *componentsmocks.TXManager
}

// const testDestAddress = "0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39"

// const testMainSigningAddress = testDestAddress

func baseMocks(t *testing.T) *mocksAndTestControl {
	mm := metrics.NewMetricsManager(context.Background())
	mocks := &mocksAndTestControl{
		allComponents:    componentsmocks.NewAllComponents(t),
		ethClientFactory: ethclientmocks.NewEthClientFactory(t),
		ethClient:        ethclientmocks.NewEthClient(t),
		blockIndexer:     blockindexermocks.NewBlockIndexer(t),
		txManager:        componentsmocks.NewTXManager(t),
	}
	mocks.allComponents.On("EthClientFactory").Return(mocks.ethClientFactory).Maybe()
	mocks.ethClientFactory.On("SharedWS").Return(mocks.ethClient).Maybe()
	mocks.ethClientFactory.On("HTTPClient").Return(mocks.ethClient).Maybe()
	mocks.allComponents.On("BlockIndexer").Return(mocks.blockIndexer).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.allComponents.On("MetricsManager").Return(mm).Maybe()
	return mocks
}

func newTestPublicTxManager(t *testing.T, realDBAndSigner bool, extraSetup ...func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig)) (context.Context, *pubTxManager, *mocksAndTestControl, func()) {
	// log.SetLevel("debug")
	ctx := context.Background()
	maxFeePerGasStr := pldtypes.Uint64ToUint256(0).HexString0xPrefix()
	maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(0).HexString0xPrefix()
	conf := &pldconf.PublicTxManagerConfig{
		Manager: pldconf.PublicTxManagerManagerConfig{
			Interval:                 confutil.P("1h"),
			MaxInFlightOrchestrators: confutil.P(1),
			SubmissionWriter: pldconf.FlushWriterConfig{
				WorkerCount: confutil.P(1),
			},
		},
		Orchestrator: pldconf.PublicTxManagerOrchestratorConfig{
			Interval: confutil.P("1h"),
			SubmissionRetry: pldconf.RetryConfigWithMax{
				MaxAttempts: confutil.P(1),
			},
			TimeLineLoggingMaxEntries: 10,
		},
		GasPrice: pldconf.GasPriceConfig{
			FixedGasPrice: &pldconf.FixedGasPricing{
				MaxFeePerGas:         &maxFeePerGasStr,
				MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
			},
		},
	}

	mocks := baseMocks(t)

	var dbClose func()
	var p persistence.Persistence
	if realDBAndSigner {
		var err error
		p, dbClose, err = persistence.NewUnitTestPersistence(ctx, "publictxmgr")
		require.NoError(t, err)

		mocks.keyManager = keymanager.NewKeyManager(ctx, &pldconf.KeyManagerInlineConfig{
			Wallets: []*pldconf.WalletConfig{
				{
					Name: "wallet1",
					Signer: &pldconf.SignerConfig{
						KeyStore: pldconf.KeyStoreConfig{
							Type: pldconf.KeyStoreTypeStatic,
							Static: pldconf.StaticKeyStoreConfig{
								Keys: map[string]pldconf.StaticKeyEntryConfig{
									"seed": {
										Encoding: "hex",
										Inline:   pldtypes.RandBytes32().String(),
									},
								},
							},
						},
						KeyDerivation: pldconf.KeyDerivationConfig{
							Type: pldconf.KeyDerivationTypeBIP32,
						},
					},
				},
			},
		})
		mocks.allComponents.On("Persistence").Return(p)
		_, err = mocks.keyManager.PreInit(mocks.allComponents)
		require.NoError(t, err)
		err = mocks.keyManager.PostInit(mocks.allComponents)
		require.NoError(t, err)
		err = mocks.keyManager.Start()
		require.NoError(t, err)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mocks.db = mp.Mock
		dbClose = func() {}
		mocks.keyManager = componentsmocks.NewKeyManager(t)
		mocks.allComponents.On("Persistence").Return(p).Maybe()
	}
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()

	// Run any extra functions before we create the manager
	for _, setup := range extraSetup {
		setup(mocks, conf)
	}

	pmgr := NewPublicTransactionManager(ctx, conf).(*pubTxManager)
	ir, err := pmgr.PreInit(mocks.allComponents)
	require.NoError(t, err)
	assert.NotNil(t, ir)

	err = pmgr.PostInit(mocks.allComponents)
	require.NoError(t, err)

	if mocks.disableManagerStart {
		pmgr.ethClient = pmgr.ethClientFactory.SharedWS()
		pmgr.gasPriceClient.Start(ctx, pmgr.ethClient)
	} else {
		err = pmgr.Start()
		require.NoError(t, err)
	}

	return pmgr.ctx, pmgr, mocks, func() {
		pmgr.Stop()
		dbClose()
	}
}

func TestInit(t *testing.T) {
	_, _, _, done := newTestPublicTxManager(t, false)
	defer done()
}

func TestTransactionLifecycleRealKeyMgrAndDB(t *testing.T) {
	ctx, ptm, m, done := newTestPublicTxManager(t, true, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Manager.Interval = confutil.P("50ms")
		conf.Orchestrator.Interval = confutil.P("50ms")
		conf.Manager.OrchestratorIdleTimeout = confutil.P("1ms")
		conf.GasLimit.GasEstimateFactor = confutil.P(1.0)
		conf.GasPrice.FixedGasPrice = nil

		// eth_gasPrice- is called on start up to detect that this is not a zero gas price chain
		mocks.ethClient.On("GasPrice", mock.Anything).Return(pldtypes.MustParseHexUint256("100"), nil)
	})
	defer done()

	chainID, _ := rand.Int(rand.Reader, big.NewInt(100000000000000))
	m.ethClient.On("ChainID").Return(chainID.Int64())

	// Resolve the key ourselves for comparison
	keyMapping, err := m.keyManager.ResolveKeyNewDatabaseTX(ctx, "signer1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	resolvedKey := pldtypes.MustEthAddress(keyMapping.Verifier.Verifier)

	// Mock gas price and estimation - we need to set the result of three calls where the values relate to each other
	const transactionCount = 10
	baseFee := pldtypes.MustParseHexUint256("100")
	reward := pldtypes.MustParseHexUint256("50")
	gasLimit := pldtypes.HexUint64(10)

	// (baseFee + reward) * gasLimit * transactionCount = balance
	// (100 + 50) * 10 * 10 = 15000
	balance := pldtypes.MustParseHexUint256("15000")

	// 1. eth_feeHistory - for dynamic gas pricing
	m.ethClient.On("FeeHistory", mock.Anything, 20, "latest", []float64{85}).Return(&ethclient.FeeHistoryResult{
		BaseFeePerGas: []pldtypes.HexUint256{*baseFee},
		Reward:        [][]pldtypes.HexUint256{{*reward}},
	}, nil)
	// 2. eth_estimateGas
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{GasLimit: gasLimit}, nil)
	// 3. eth_getBalance - for ensuring that the account has enough gas to pay for the transactions
	m.ethClient.On("GetBalance", mock.Anything, *resolvedKey, "latest").Return(balance, nil)

	// create some transactions that are successfully added
	txIDs := make([]uuid.UUID, transactionCount)
	txs := make([]*components.PublicTxSubmission, transactionCount)
	for i := range txIDs {
		txIDs[i] = uuid.New()

		// We do the public TX manager's job for it in this test
		fakeTxManagerInsert(t, ptm.p.DB(), txIDs[i], "signer2")

		txs[i] = &components.PublicTxSubmission{
			Bindings: []*components.PaladinTXReference{
				{TransactionID: txIDs[i], TransactionType: pldapi.TransactionTypePrivate.Enum()},
			},
			PublicTxInput: pldapi.PublicTxInput{
				From: resolvedKey,
				Data: []byte(fmt.Sprintf("data %d", i)),
			},
		}

	}

	// nonce should be cached - so is once'd
	baseNonce := uint64(11223000)
	m.ethClient.On("GetTransactionCount", mock.Anything, mock.Anything).
		Return(confutil.P(pldtypes.HexUint64(baseNonce)), nil).Once()

	// For the first one we do a one-off
	singleTx, err := ptm.SingleTransactionSubmit(ctx, txs[0])
	require.NoError(t, err)

	// The rest we submit as as batch
	for _, tx := range txs[1:] {
		err := ptm.ValidateTransaction(ctx, ptm.p.NOTX(), tx)
		require.NoError(t, err)
	}
	var batch []*pldapi.PublicTx
	err = ptm.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		batch, err = ptm.WriteNewTransactions(ctx, dbTX, txs[1:])
		return err
	})
	require.NoError(t, err)
	require.Len(t, batch, len(txs[1:]))
	for _, tx := range batch {
		require.Greater(t, *tx.LocalID, uint64(0))
	}

	// Get one back again by ID
	txRead, err := ptm.QueryPublicTxWithBindings(ctx, ptm.p.NOTX(), query.NewQueryBuilder().Equal("localId", *batch[1].LocalID).Limit(1).Query())
	require.NoError(t, err)
	require.Len(t, txRead, 1)
	require.Equal(t, batch[1].Data, txRead[0].Data)

	// Record activity on one TX
	for i, tx := range append([]*pldapi.PublicTx{singleTx}, batch...) {
		ptm.addActivityRecord(*tx.LocalID, fmt.Sprintf("activity %d", i))
	}

	// Query to check we now have all of these
	queryTxs, err := ptm.QueryPublicTxWithBindings(ctx, ptm.p.NOTX(),
		query.NewQueryBuilder().Sort("localId").Query())
	require.NoError(t, err)
	assert.Len(t, queryTxs, len(txs))
	for i, qTX := range queryTxs {
		// We don't include the bindings on these queries
		assert.Equal(t, *resolvedKey, qTX.From)
		assert.Equal(t, txs[i].Data, qTX.Data)
		require.Greater(t, len(qTX.Activity), 0)
	}

	// Query scoped to one TX
	byTxn, err := ptm.QueryPublicTxForTransactions(ctx, ptm.p.NOTX(), txIDs, nil)
	require.NoError(t, err)
	for _, tx := range txs {
		queryTxs := byTxn[tx.Bindings[0].TransactionID]
		require.Len(t, queryTxs, 1)
	}

	// Check we can select to just see confirmed (which this isn't yet)
	byTxn, err = ptm.QueryPublicTxForTransactions(ctx, ptm.p.NOTX(), txIDs,
		query.NewQueryBuilder().NotNull("transactionHash").Query())
	require.NoError(t, err)
	for _, tx := range txs {
		queryTxs := byTxn[tx.Bindings[0].TransactionID]
		require.Empty(t, queryTxs, 1)
	}

	// Wait for a submission to happen
	calculatedConfirmations := make(chan *blockindexer.IndexedTransactionNotify, len(txIDs))
	srtx := m.ethClient.On("SendRawTransaction", mock.Anything, mock.Anything)
	srtx.Run(func(args mock.Arguments) {
		signedMessage := args[1].(pldtypes.HexBytes)

		// We need to decode the TX to find the nonce
		signer, ethTx, err := ethsigner.RecoverRawTransaction(ctx, ethtypes.HexBytes0xPrefix(signedMessage), m.ethClient.ChainID())
		require.NoError(t, err)
		assert.Equal(t, *resolvedKey, pldtypes.EthAddress(*signer))

		assert.Equal(t, uint64(150), ethTx.MaxFeePerGas.Uint64())
		assert.Equal(t, uint64(50), ethTx.MaxPriorityFeePerGas.Uint64())

		txHash := calculateTransactionHash(signedMessage)
		confirmation := &blockindexer.IndexedTransactionNotify{
			IndexedTransaction: pldapi.IndexedTransaction{
				Hash:             *txHash,
				BlockNumber:      11223344,
				TransactionIndex: 10,
				From:             resolvedKey,
				To:               (*pldtypes.EthAddress)(ethTx.To),
				Nonce:            ethTx.Nonce.Uint64(),
				Result:           pldapi.TXResult_SUCCESS.Enum(),
			},
		}
		calculatedConfirmations <- confirmation

		srtx.Return(&confirmation.Hash, nil)
	})

	// Wait for all the confirmations to be calculated
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	gatheredConfirmations := []*blockindexer.IndexedTransactionNotify{}
	for len(gatheredConfirmations) < len(txs) {
		select {
		case confirmation := <-calculatedConfirmations:
			gatheredConfirmations = append(gatheredConfirmations, confirmation)

			// Check we can query the public txn by this submission (even before the confirm)
			ptxQuery, err := ptm.GetPublicTransactionForHash(ctx, ptm.p.NOTX(), confirmation.Hash)
			require.NoError(t, err)
			require.NotNil(t, ptxQuery)
			require.Len(t, ptxQuery.Submissions, 1)
			require.Equal(t, ptxQuery.Nonce.Uint64(), confirmation.Nonce)
		case <-ticker.C:
			if t.Failed() {
				return
			}
		}
	}

	// Simulate detection of the receipt by the blockexplorer - phase 1 in the DB Transaction
	var allMatches []*components.PublicTxMatch
	confirmationsMatched := make(map[uuid.UUID]*components.PublicTxMatch)
	for _, confirmation := range gatheredConfirmations {
		matches, err := ptm.MatchUpdateConfirmedTransactions(ctx, ptm.p.NOTX(), []*blockindexer.IndexedTransactionNotify{confirmation})
		require.NoError(t, err)
		// NOTE: This is a good test that we definitely persist _before_ we submit as
		// otherwise we could miss notifying users of their transactions completing.
		// Either because we crashed without finishing our DB commit, or because (like this
		// test simulates) the blockchain confirms to us before we submit.
		allMatches = append(allMatches, matches...)
		assert.Len(t, matches, 1)
		confirmationsMatched[matches[0].TransactionID] = matches[0]
	}
	for _, tx := range txs {
		assert.NotNil(t, confirmationsMatched[tx.Bindings[0].TransactionID])
	}

	// Check we can select to just see just unconfirmed
	byTxn, err = ptm.QueryPublicTxForTransactions(ctx, ptm.p.NOTX(), txIDs,
		query.NewQueryBuilder().Null("transactionHash").Query())
	require.NoError(t, err)
	for _, tx := range txs {
		queryTxs := byTxn[tx.Bindings[0].TransactionID]
		require.Empty(t, queryTxs, 1)
	}

	// phase 2 of the update, happens after the DB TX commits, so we can wake up the
	// orchestrators to remove the in-flight TXns
	ptm.NotifyConfirmPersisted(ctx, allMatches)

	// Now the inflights should all exit, so we wait for the orchestrator to exit
	for ptm.getOrchestratorCount() > 0 {
		<-ticker.C
		if t.Failed() {
			return
		}
	}
	ticker.Stop()

}

func fakeTxManagerInsert(t *testing.T, db *gorm.DB, txID uuid.UUID, fromStr string) {
	// Yes, there is a slight smell of un-partitioned DB responsibilities between components
	// here. But the saving is critical path avoidance of one extra DB query for every block
	// that is mined. So it's currently considered worth this limited quirk.
	fakeABI := pldtypes.RandBytes32()
	err := db.Exec(`INSERT INTO "abis" ("hash","abi","created") VALUES (?, ?, ?)`,
		fakeABI, `[]`, pldtypes.TimestampNow()).
		Error
	require.NoError(t, err)
	err = db.Exec(`INSERT INTO "transactions" ("id", "submit_mode", "created", "type", "abi_ref", "from") VALUES (?, ?, ?, ?, ?, ?)`,
		txID, pldapi.SubmitModeAuto, pldtypes.TimestampNow(), pldapi.TransactionTypePrivate.Enum(), fakeABI, fromStr).
		Error
	require.NoError(t, err)
}

func TestSubmitFailures(t *testing.T) {
	ctx, ptm, m, done := newTestPublicTxManager(t, false)
	defer done()

	// estimation failure - for non-revert
	m.db.ExpectBegin()
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{}, fmt.Errorf("GasEstimate error")).Once()
	_, err := ptm.SingleTransactionSubmit(ctx, &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: pldtypes.RandAddress(),
		},
	})
	assert.Regexp(t, "GasEstimate error", err)

	// estimation failure - for revert
	m.db.ExpectBegin()
	sampleRevertData := pldtypes.HexBytes("some data")
	m.txManager.On("CalculateRevertError", mock.Anything, mock.Anything, sampleRevertData).Return(fmt.Errorf("mapped revert error"))
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{
			RevertData: sampleRevertData,
		}, fmt.Errorf("execution reverted")).Once()
	_, err = ptm.SingleTransactionSubmit(ctx, &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: pldtypes.RandAddress(),
		},
	})
	assert.Regexp(t, "mapped revert error", err)

}

func TestAddActivityDisabled(t *testing.T) {
	_, ptm, _, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Manager.ActivityRecords.RecordsPerTransaction = confutil.P(0)
	})
	defer done()

	ptm.addActivityRecord(12345, "message")

	assert.Empty(t, ptm.getActivityRecords(12345))
}

func TestAddActivityWrap(t *testing.T) {
	_, ptm, _, done := newTestPublicTxManager(t, false)
	defer done()

	for i := 0; i < 100; i++ {
		ptm.addActivityRecord(12345, fmt.Sprintf("message %.2d", i))
	}

	activityRecords := ptm.getActivityRecords(12345)
	assert.Equal(t, "message 99", activityRecords[0].Message)
	assert.Equal(t, "message 98", activityRecords[1].Message)
	assert.Len(t, activityRecords, ptm.maxActivityRecordsPerTx)

}

func TestHandleNewTransactionTransferOnlyWithProvideGas(t *testing.T) {
	ctx := context.Background()
	_, ptm, _, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		mocks.db.MatchExpectationsInOrder(false)
		mocks.db.ExpectBegin()
		mocks.db.ExpectQuery("SELECT.*public_txns").WillReturnRows(sqlmock.NewRows([]string{}))
		mocks.db.ExpectQuery("INSERT.*public_txns").WillReturnRows(mocks.db.NewRows([]string{"pub_txn_id"}).AddRow(12345))
		mocks.db.ExpectCommit()
	})
	defer done()

	// create transaction succeeded
	tx, err := ptm.SingleTransactionSubmit(ctx, &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: pldtypes.RandAddress(),
			To:   pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas:   confutil.P(pldtypes.HexUint64(1223451)),
				Value: pldtypes.Uint64ToUint256(100),
			},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, tx.From)
	assert.Equal(t, uint64(1223451), tx.Gas.Uint64())

}

func TestEngineSuspendResumeRealDB(t *testing.T) {
	ctx, ptm, m, done := newTestPublicTxManager(t, true, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Manager.Interval = confutil.P("50ms")
		conf.Orchestrator.Interval = confutil.P("50ms")
		conf.Manager.OrchestratorIdleTimeout = confutil.P("1ms")
		conf.Orchestrator.StageRetryTime = confutil.P("0ms") // without this we stick in the stage for 10s before we look to suspend
	})
	defer done()

	keyMapping, err := m.keyManager.ResolveKeyNewDatabaseTX(ctx, "signer1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	resolvedKey := *pldtypes.MustEthAddress(keyMapping.Verifier.Verifier)

	chainID, _ := rand.Int(rand.Reader, big.NewInt(100000000000000))
	m.ethClient.On("ChainID").Return(chainID.Int64()).Maybe()

	pubTx := &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: &resolvedKey,
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(1223451)),
			},
		},
	}

	// We can get the nonce
	m.ethClient.On("GetTransactionCount", mock.Anything, mock.Anything).Return(confutil.P(pldtypes.HexUint64(1122334455)), nil)
	// ... but attempting to get it onto the chain is going to block failing
	m.ethClient.On("SendRawTransaction", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop")).Maybe()

	_, err = ptm.SingleTransactionSubmit(ctx, pubTx)
	require.NoError(t, err)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Wait for the orchestrator to kick off and pick this TX up
	getIFT := func() *inFlightTransactionStageController {
		var o *orchestrator
		var ift *inFlightTransactionStageController
		for ift == nil {
			<-ticker.C
			if t.Failed() {
				panic("test failed")
			}
			o = ptm.getOrchestratorForAddress(resolvedKey)
			if o != nil {
				ift = o.getFirstInFlight()
			}
		}
		return ift
	}
	txNonce := getIFT().stateManager.GetNonce()

	// suspend the TX
	err = ptm.SuspendTransaction(ctx, resolvedKey, txNonce)
	require.NoError(t, err)

	// wait to flush out the whole orchestrator as this is the only thing in flight
	for ptm.getOrchestratorCount() > 0 {
		<-ticker.C
		if t.Failed() {
			return
		}
	}

	// resume the txn
	err = ptm.ResumeTransaction(ctx, resolvedKey, txNonce)
	require.NoError(t, err)

	// check the orchestrator comes back
	newNonce := getIFT().stateManager.GetNonce()
	assert.Equal(t, txNonce, newNonce)

}

func TestUpdateTransactionRealDB(t *testing.T) {
	ctx, ptm, m, done := newTestPublicTxManager(t, true, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Manager.Interval = confutil.P("50ms")
		conf.Orchestrator.Interval = confutil.P("50ms")
		conf.Manager.OrchestratorIdleTimeout = confutil.P("1ms")
	})
	defer done()

	keyMapping, err := m.keyManager.ResolveKeyNewDatabaseTX(ctx, "signer1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	require.NoError(t, err)
	resolvedKey := pldtypes.MustEthAddress(keyMapping.Verifier.Verifier)

	chainID, _ := rand.Int(rand.Reader, big.NewInt(100000000000000))
	m.ethClient.On("ChainID").Return(chainID.Int64())

	txID := uuid.New()
	pubTxSub := &components.PublicTxSubmission{
		Bindings: []*components.PaladinTXReference{
			{TransactionID: txID, TransactionType: pldapi.TransactionTypePublic.Enum()},
		},
		PublicTxInput: pldapi.PublicTxInput{
			From: resolvedKey,
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(1223451)),
			},
		},
	}

	m.ethClient.On("GetTransactionCount", mock.Anything, mock.Anything).Return(confutil.P(pldtypes.HexUint64(1122334455)), nil)

	confirmations := make(chan *blockindexer.IndexedTransactionNotify, 1)
	srtx := m.ethClient.On("SendRawTransaction", mock.Anything, mock.Anything)
	srtx.Run(func(args mock.Arguments) {
		signedMessage := args[1].(pldtypes.HexBytes)

		signer, ethTx, err := ethsigner.RecoverRawTransaction(ctx, ethtypes.HexBytes0xPrefix(signedMessage), m.ethClient.ChainID())
		require.NoError(t, err)
		assert.Equal(t, *resolvedKey, pldtypes.EthAddress(*signer))

		if ethTx.GasLimit.Int64() == int64(2223451) {
			// We need to decode the TX to find the nonce
			txHash := calculateTransactionHash(signedMessage)
			confirmation := &blockindexer.IndexedTransactionNotify{
				IndexedTransaction: pldapi.IndexedTransaction{
					Hash:             *txHash,
					BlockNumber:      11223344,
					TransactionIndex: 10,
					From:             resolvedKey,
					To:               (*pldtypes.EthAddress)(ethTx.To),
					Nonce:            ethTx.Nonce.Uint64(),
					Result:           pldapi.TXResult_SUCCESS.Enum(),
				},
			}
			confirmations <- confirmation

			srtx.Return(&confirmation.Hash, nil)
		} else {
			srtx.Return(nil, fmt.Errorf("pop"))
		}
	})

	pubTx, err := ptm.SingleTransactionSubmit(ctx, pubTxSub)
	require.NoError(t, err)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Wait for the orchestrator to kick off and pick this TX up
	var ift *inFlightTransactionStageController
	for ift == nil {
		<-ticker.C
		if t.Failed() {
			panic("test failed")
		}
		o := ptm.getOrchestratorForAddress(*resolvedKey)
		if o != nil {
			ift = o.getFirstInFlight()
		}
	}

	// pub_txn_id not found
	err = ptm.UpdateTransaction(ctx, txID, uint64(2), resolvedKey, &pldapi.TransactionInput{}, nil, func(dbTX persistence.DBTX) error { return nil })

	require.Error(t, err)

	// gas estimate failure with revert data
	sampleRevertData := pldtypes.HexBytes("some data")
	m.txManager.On("CalculateRevertError", mock.Anything, mock.Anything, sampleRevertData).Return(fmt.Errorf("mapped revert error"))
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{
			RevertData: sampleRevertData,
		}, fmt.Errorf("execution reverted")).Once()

	err = ptm.UpdateTransaction(ctx, txID, *pubTx.LocalID, resolvedKey, &pldapi.TransactionInput{}, nil, func(dbTX persistence.DBTX) error { return errors.New("db write failed") })
	require.EqualError(t, err, "mapped revert error")

	// gas estimate failure without revert data
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{}, fmt.Errorf("GasEstimate error")).Once()
	err = ptm.UpdateTransaction(ctx, txID, *pubTx.LocalID, resolvedKey, &pldapi.TransactionInput{}, nil, func(dbTX persistence.DBTX) error { return errors.New("db write failed") })
	require.EqualError(t, err, "GasEstimate error")

	// txmgr db write fails
	err = ptm.UpdateTransaction(ctx, txID, *pubTx.LocalID, resolvedKey, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			PublicTxOptions: pldapi.PublicTxOptions{
				Gas: confutil.P(pldtypes.HexUint64(2223451)),
			},
		},
	}, nil, func(dbTX persistence.DBTX) error { return errors.New("db write failed") })

	require.Error(t, err)

	// update the transaction
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{
			GasLimit: pldtypes.HexUint64(2223451),
		}, nil).Once()
	err = ptm.UpdateTransaction(ctx, txID, *pubTx.LocalID, resolvedKey, &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     resolvedKey.String(),
			To:       pldtypes.MustEthAddress(pldtypes.RandHex(20)),
			Function: "set",
			Data:     pldtypes.RawJSON(`{"value": 46}`),
		},
		ABI: abi.ABI{{Type: abi.Function, Name: "set", Inputs: abi.ParameterArray{{Type: "uint256", Name: "value"}}}},
	}, nil, func(dbTX persistence.DBTX) error { return nil })

	require.NoError(t, err)

	// simulate the confirmation so we can check that the inflight transaction is able to complete and be removed
	// we don't want any previous state to block this
	waitingForConfirmation := true
	for waitingForConfirmation {
		select {
		case confirmation := <-confirmations:
			match, err := ptm.MatchUpdateConfirmedTransactions(ctx, ptm.p.NOTX(), []*blockindexer.IndexedTransactionNotify{confirmation})
			require.NoError(t, err)
			ptm.NotifyConfirmPersisted(ctx, match)
			waitingForConfirmation = false

		case <-ticker.C:
			if t.Failed() {
				return
			}
		}
	}

	// wait to flush out the whole orchestrator as this is the only thing in flight
	for ptm.getOrchestratorCount() > 0 {
		<-ticker.C
		if t.Failed() {
			return
		}
	}

	txs, err := ptm.QueryPublicTxForTransactions(ctx, ptm.p.NOTX(), []uuid.UUID{txID}, nil)
	require.NoError(t, err)
	require.Len(t, txs[txID], 1)
	tx := txs[txID][0]
	require.Len(t, tx.Submissions, 2)
}

func TestGasEstimateFactor(t *testing.T) {
	ctx := context.Background()
	_, ptm, m, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.GasLimit.GasEstimateFactor = confutil.P(2.0)
	})
	defer done()

	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{GasLimit: pldtypes.MustParseHexUint64("0x62f8")}, nil)

	tx := &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: pldtypes.MustEthAddress("0x14655a513c68280d16f72304ebfd1ae1a2262d2d"),
			Data: []byte("[2]"),
		},
	}

	require.NoError(t, ptm.ValidateTransaction(ctx, ptm.p.NOTX(), tx))
	assert.Equal(t, pldtypes.MustParseHexUint64("0xc5f0"), *tx.Gas)
}

func TestSuspendTransactionNoOrchestrator(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, true) // Use real DB
	defer done()

	// Create a test address and nonce
	testAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	testNonce := uint64(12345)

	// First, insert a test transaction into the database (omit PublicTxnID - it's auto-generated)
	dbTX := ptm.p.DB().WithContext(ctx)
	err := dbTX.Table("public_txns").Create(&DBPublicTxn{
		From:      *testAddress,
		Nonce:     &testNonce,
		Suspended: false,
	}).Error
	require.NoError(t, err)

	// Call SuspendTransaction - this should trigger persistSuspendedFlag since no orchestrator is in flight
	err = ptm.SuspendTransaction(ctx, *testAddress, testNonce)
	assert.NoError(t, err)

	// Verify the transaction was actually suspended in the database
	var updatedTx DBPublicTxn
	err = dbTX.Table("public_txns").
		Where(`"from" = ? AND nonce = ?`, *testAddress, testNonce).
		First(&updatedTx).Error
	require.NoError(t, err)
	assert.True(t, updatedTx.Suspended)
}

func TestResumeTransactionNoOrchestrator(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, true) // Use real DB
	defer done()

	// Create a test address and nonce
	testAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	testNonce := uint64(12346)

	// First, insert a test transaction into the database (suspended) (omit PublicTxnID - it's auto-generated)
	dbTX := ptm.p.DB().WithContext(ctx)
	err := dbTX.Table("public_txns").Create(&DBPublicTxn{
		From:      *testAddress,
		Nonce:     &testNonce,
		Suspended: true,
	}).Error
	require.NoError(t, err)

	// Call ResumeTransaction - this should trigger persistSuspendedFlag since no orchestrator is in flight
	err = ptm.ResumeTransaction(ctx, *testAddress, testNonce)
	assert.NoError(t, err)

	// Verify the transaction was actually resumed in the database
	var updatedTx DBPublicTxn
	err = dbTX.Table("public_txns").
		Where(`"from" = ? AND nonce = ?`, *testAddress, testNonce).
		First(&updatedTx).Error
	require.NoError(t, err)
	assert.False(t, updatedTx.Suspended)
}

func TestDispatchActionInvalidAction(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, false) // Use mock DB
	defer done()

	// Create a test address and nonce
	testAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	testNonce := uint64(12345)

	// Call dispatchAction with an invalid action type (beyond the defined constants)
	// This should trigger the default return case (line 71)
	invalidAction := AsyncRequestType(999) // Invalid action type
	err := ptm.dispatchAction(ctx, *testAddress, testNonce, invalidAction)

	// The default case should return nil (no error)
	assert.NoError(t, err)
}

func TestCheckTransactionCompletedNotFound(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, true) // Use real DB
	defer done()

	// Test with a non-existent transaction ID
	completed, err := ptm.CheckTransactionCompleted(ctx, 99999)
	assert.NoError(t, err)
	assert.False(t, completed)
}

func TestCheckTransactionCompletedNotCompleted(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, true) // Use real DB
	defer done()

	// Create a test transaction that is not completed
	testAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	testNonce := uint64(12347)

	// Insert a transaction without completion (omit PublicTxnID - it's auto-generated)
	dbTX := ptm.p.DB().WithContext(ctx)
	err := dbTX.Table("public_txns").Create(&DBPublicTxn{
		From:      *testAddress,
		Nonce:     &testNonce,
		Suspended: false,
		Completed: nil, // No completion record
	}).Error
	require.NoError(t, err)

	// Retrieve the transaction to get the actual PublicTxnID
	var insertedTx DBPublicTxn
	err = dbTX.Table("public_txns").
		Where(`"from" = ? AND nonce = ?`, *testAddress, testNonce).
		First(&insertedTx).Error
	require.NoError(t, err)

	// Check if it's completed - should return false
	completed, err := ptm.CheckTransactionCompleted(ctx, insertedTx.PublicTxnID)
	assert.NoError(t, err)
	assert.False(t, completed)
}

func TestCheckTransactionCompletedWithCompletion(t *testing.T) {
	ctx, ptm, _, done := newTestPublicTxManager(t, true) // Use real DB
	defer done()

	// Create a test transaction that is completed
	testAddress := pldtypes.MustEthAddress("0x1234567890123456789012345678901234567890")
	testNonce := uint64(12348)
	testTxHash := pldtypes.MustParseBytes32("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	// Insert a transaction with completion (omit PublicTxnID - it's auto-generated)
	dbTX := ptm.p.DB().WithContext(ctx)
	err := dbTX.Table("public_txns").Create(&DBPublicTxn{
		From:      *testAddress,
		Nonce:     &testNonce,
		Suspended: false,
		Completed: &DBPublicTxnCompletion{
			TransactionHash: testTxHash,
		},
	}).Error
	require.NoError(t, err)

	// Retrieve the transaction to get the actual PublicTxnID
	var insertedTx DBPublicTxn
	err = dbTX.Table("public_txns").
		Where(`"from" = ? AND nonce = ?`, *testAddress, testNonce).
		First(&insertedTx).Error
	require.NoError(t, err)

	// Check if it's completed - should return true
	completed, err := ptm.CheckTransactionCompleted(ctx, insertedTx.PublicTxnID)
	assert.NoError(t, err)
	assert.True(t, completed)
}
