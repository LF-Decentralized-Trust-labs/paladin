/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicaptm law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package publictxmgr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestOrchestrator(t *testing.T, cbs ...func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig)) (context.Context, *orchestrator, *mocksAndTestControl, func()) {
	ctx, ptm, m, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		mocks.disableManagerStart = true // we don't want the manager running - this gives us a fake nonce manager too
		for _, cb := range cbs {
			cb(mocks, conf)
		}
	})

	signingAddress := pldtypes.EthAddress(pldtypes.RandBytes(20))
	o := NewOrchestrator(ptm, signingAddress, ptm.conf)

	return ctx, o, m, done

}

func newInflightTransaction(o *orchestrator, nonce uint64, txMods ...func(tx *DBPublicTxn)) (*inFlightTransactionStageController, *inFlightTransactionState) {
	tx := &DBPublicTxn{
		From:    o.signingAddress,
		Nonce:   &nonce,
		Gas:     2000,
		Created: pldtypes.TimestampNow(),
		To:      pldtypes.EthAddressBytes(pldtypes.RandBytes(20)),
	}
	for _, txMod := range txMods {
		txMod(tx)
	}
	mockIT := NewInFlightTransactionStageController(o.pubTxManager, o, tx, uuid.New())
	return mockIT, mockIT.stateManager.(*inFlightTransactionState)
}

func TestNewOrchestratorLoadsSecondTxAndQueuesBalanceCheck(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(2) // only poll once then we're full
		maxFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		conf.GasPrice.FixedGasPrice = &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		}
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)

	// Fill first slot with a stage controller
	o.inFlightTxs = []*inFlightTransactionStageController{mockIT}

	// Return a single transaction - note there's a highest nonce query on startup before the first poll, so we query twice
	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{"pub_txn_id", "from", "nonce", "Binding__pub_txn_id", "Binding__transaction"}).AddRow(
			0, o.signingAddress, 2, 0, uuid.New().String(),
		))
	}

	// Do not return any submissions for it
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	addressBalanceChecked := make(chan bool)
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(pldtypes.Uint64ToUint256(100), nil).Run(func(args mock.Arguments) {
		select {
		case <-addressBalanceChecked:
			// the channel only needs to be closed the first time
		default:
			close(addressBalanceChecked)
		}
	})
	oDone, _ := o.Start(ctx)
	<-addressBalanceChecked
	o.Stop()
	<-oDone

}

func TestNewOrchestratorPollingLoopContextCancelled(t *testing.T) {

	_, o, _, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(10)
	})
	done()

	o.orchestratorLoopDone = make(chan struct{})
	o.orchestratorLoop()
}

func TestNewOrchestratorPollingContextCancelledWhileRetrying(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(10)
	})
	defer done()

	o.retry.UTSetMaxAttempts(1) // simulate exit after error
	m.db.ExpectQuery("SELECT.*public_txn").WillReturnError(fmt.Errorf("pop"))

	o.ctxCancel()
	polled, _ := o.pollAndProcess(ctx)
	assert.Equal(t, -1, polled)
}

func TestNewOrchestratorPollingRemoveCompleted(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(1) // just one inflight, which will trigger poll only after it is done
	})
	defer done()

	// Fill first slot with a stage controller
	mockIT, _ := newInflightTransaction(o, 1)
	mockIT.hasZeroGasPrice = true
	confirmed := InFlightStatusConfirmReceived
	mockIT.newStatus = &confirmed
	o.inFlightTxs = []*inFlightTransactionStageController{mockIT}
	o.setState(OrchestratorStateRunning)

	for range 2 {
		m.db.ExpectQuery("SELECT.*public_txns").WillReturnRows(sqlmock.NewRows([]string{}))
		m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))
	}
	ocDone, _ := o.Start(ctx)

	// It should go idle
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for o.getState() != OrchestratorStateIdle && !t.Failed() {
		<-ticker.C
	}

	// and that means it should be empty
	assert.Empty(t, o.inFlightTxs)

	// Then we stop it (the manager does this - orchestrators do not stop themselves)
	o.Stop()
	<-ocDone
}

func TestOrchestratorWaitingForBalance(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(m *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(1) // just one inflight - which we inject in
		maxFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		conf.GasPrice.FixedGasPrice = &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		}
	})
	defer done()

	mockIT, txState := newInflightTransaction(o, 1, func(tx *DBPublicTxn) {
		tx.Gas = 100
	})
	txState.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		NewValues: BaseTXUpdateNewValues{
			GasPricing: &pldapi.PublicTxGasPricing{
				MaxFeePerGas:         pldtypes.Uint64ToUint256(1000),
				MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(100),
			},
		},
	})

	// Fill first slot with a stage controller
	o.inFlightTxs = []*inFlightTransactionStageController{mockIT}
	o.setState(OrchestratorStateRunning)
	o.lastQueueUpdate = time.Now()

	m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{}))
	m.db.ExpectQuery("SELECT.*public_txn_bindings").WillReturnRows(sqlmock.NewRows([]string{"transaction"}).AddRow(uuid.New().String()))

	// Mock the insufficient balance on the account that's submitting
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(pldtypes.Uint64ToUint256(0), nil)

	oDone, err := o.Start(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return o.getState() == OrchestratorStateWaiting
	}, time.Second, 10*time.Millisecond)

	o.Stop()
	<-oDone
}

func TestAllocateNoncesGetTransactionCountError(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()

	// nextNonce is nil so allocateNonces will call GetTransactionCount
	txn := &DBPublicTxn{PublicTxnID: 1, From: o.signingAddress}
	m.ethClient.On("GetTransactionCount", mock.Anything, o.signingAddress).
		Return(nil, fmt.Errorf("rpc error")).Once()

	err := o.allocateNonces(ctx, []*DBPublicTxn{txn})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rpc error")
}

func TestAllocateNoncesNonceCacheAheadOfMempool(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()

	// Set nextNonce ahead of what the mempool reports
	ahead := uint64(5)
	o.nextNonce = &ahead
	// lastNonceAlloc is zero time so cache is always expired and GetTransactionCount is called

	// Mempool reports nonce 4 (lower than our cached 5) - so we keep our cached nonce
	m.ethClient.On("GetTransactionCount", mock.Anything, o.signingAddress).
		Return(confutil.P(pldtypes.HexUint64(4)), nil).Once()

	// DB transaction to record the nonce assignment must succeed
	m.db.ExpectBegin()
	m.db.ExpectExec("WITH nonce_updates").WillReturnResult(sqlmock.NewResult(1, 1))
	m.db.ExpectCommit()

	txn := &DBPublicTxn{PublicTxnID: 1, From: o.signingAddress}
	err := o.allocateNonces(ctx, []*DBPublicTxn{txn})
	assert.NoError(t, err)
	// nextNonce should have advanced by 1 (we allocated nonce 5)
	assert.Equal(t, uint64(6), *o.nextNonce)
}

func TestAllocateNoncesDBTransactionError(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t)
	defer done()

	// nextNonce is nil so GetTransactionCount is called
	m.ethClient.On("GetTransactionCount", mock.Anything, o.signingAddress).
		Return(confutil.P(pldtypes.HexUint64(10)), nil).Once()

	// DB transaction fails
	m.db.ExpectBegin()
	m.db.ExpectExec("WITH nonce_updates").WillReturnError(fmt.Errorf("db transaction error"))
	m.db.ExpectRollback()

	txn := &DBPublicTxn{PublicTxnID: 1, From: o.signingAddress}
	err := o.allocateNonces(ctx, []*DBPublicTxn{txn})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db transaction error")
}

func TestPollAndProcessHandleTransactionCollectedAndNonceAssignedErrors(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(5)
	})
	defer done()
	o.testOnlyNoActionMode = true

	txID := uuid.New()
	nonce := uint64(7)
	contractAddr := "0x1234567890123456789012345678901234567890"

	// Poll returns a tx with nonce already set and a ContractAddress in the binding
	m.db.ExpectQuery("SELECT.*public_txns").WillReturnRows(
		sqlmock.NewRows([]string{"pub_txn_id", "from", "nonce", "Binding__pub_txn_id", "Binding__transaction", "Binding__contract_address"}).
			AddRow(1, o.signingAddress, nonce, 1, txID.String(), contractAddr),
	)
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	// HandleTransactionCollected returns an error (logged as warning, processing continues)
	m.sequencerManager.On("HandleTransactionCollected", mock.Anything, o.signingAddress.String(), contractAddr, txID).
		Return(fmt.Errorf("collected error")).Once()

	// HandleNonceAssigned is called after allocateNonces (nonce already set so it's a no-op alloc)
	m.sequencerManager.On("HandleNonceAssigned", mock.Anything, nonce, contractAddr, txID).
		Return(fmt.Errorf("nonce assigned error")).Once()

	polled, _ := o.pollAndProcess(ctx)
	assert.Equal(t, 1, polled)
}

func TestPollAndProcessAllocateNoncesError(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(5)
	})
	defer done()

	o.retry.UTSetMaxAttempts(1)

	// Poll returns a tx with Nonce=nil (allocateNonces will be needed)
	m.db.ExpectQuery("SELECT.*public_txns").WillReturnRows(
		sqlmock.NewRows([]string{"pub_txn_id", "from", "Binding__pub_txn_id", "Binding__transaction"}).
			AddRow(1, o.signingAddress, 1, uuid.New().String()),
	)
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	// allocateNonces fails: GetTransactionCount returns error
	m.ethClient.On("GetTransactionCount", mock.Anything, o.signingAddress).
		Return(nil, fmt.Errorf("nonce rpc error")).Once()

	polled, _ := o.pollAndProcess(ctx)
	// Polling succeeded but allocateNonces failed -> early return with polled=0
	assert.Equal(t, 0, polled)
	assert.Empty(t, o.inFlightTxs)
}

func TestPollAndProcessNilBinding(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(5)
	})
	defer done()

	nonce := uint64(3)

	// Poll returns a tx WITHOUT binding columns - GORM leaves Binding as nil
	m.db.ExpectQuery("SELECT.*public_txns").WillReturnRows(
		sqlmock.NewRows([]string{"pub_txn_id", "from", "nonce"}).
			AddRow(1, o.signingAddress, nonce),
	)
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	polled, _ := o.pollAndProcess(ctx)
	// Tx with nil binding is skipped (not added to inFlightTxs)
	assert.Equal(t, 0, polled)
	assert.Empty(t, o.inFlightTxs)
}

func TestProcessInFlightTransactionsBalanceUnavailableWait(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(1)
		// Non-zero gas price so balance check is NOT skipped
		maxFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		conf.GasPrice.FixedGasPrice = &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		}
		s := string(OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyWait)
		conf.Orchestrator.UnavailableBalanceHandler = &s
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)
	// GetBalance returns error so GetAddressBalance fails
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(nil, fmt.Errorf("balance error")).Once()

	waitingForBalance, err := o.ProcessInFlightTransactions(ctx, []*inFlightTransactionStageController{mockIT})
	require.NoError(t, err)
	assert.True(t, waitingForBalance)
}

func TestProcessInFlightTransactionsBalanceUnavailableStop(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(1)
		maxFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		conf.GasPrice.FixedGasPrice = &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		}
		s := string(OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyStop)
		conf.Orchestrator.UnavailableBalanceHandler = &s
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(nil, fmt.Errorf("balance error")).Once()

	waitingForBalance, err := o.ProcessInFlightTransactions(ctx, []*inFlightTransactionStageController{mockIT})
	require.NoError(t, err)
	assert.True(t, waitingForBalance)
}

func TestProcessInFlightTransactionsBalanceUnavailableContinue(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(1)
		maxFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		maxPriorityFeePerGasStr := pldtypes.Uint64ToUint256(1).HexString0xPrefix()
		conf.GasPrice.FixedGasPrice = &pldconf.FixedGasPricing{
			MaxFeePerGas:         &maxFeePerGasStr,
			MaxPriorityFeePerGas: &maxPriorityFeePerGasStr,
		}
		// Any other strategy (like "continue") triggers the default case
		s := string(OrchestratorBalanceCheckUnavailableBalanceHandlingStrategyContinue)
		conf.Orchestrator.UnavailableBalanceHandler = &s
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)
	// Suppress async stage actions: with "continue" the function falls through to ProduceLatestInFlightStageContext,
	// which would spawn a goroutine via executeAsync without this flag.
	mockIT.testOnlyNoActionMode = true
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(nil, fmt.Errorf("balance error")).Once()

	waitingForBalance, err := o.ProcessInFlightTransactions(ctx, []*inFlightTransactionStageController{mockIT})
	require.NoError(t, err)
	// With "continue" strategy, processing continues without balance check - waitingForBalance stays false
	assert.False(t, waitingForBalance)
}
