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
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"

	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
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
	}
	for _, txMod := range txMods {
		txMod(tx)
	}
	mockIT := NewInFlightTransactionStageController(o.pubTxManager, o, tx)
	return mockIT, mockIT.stateManager.(*inFlightTransactionState)
}

func TestNewOrchestratorLoadsSecondTxAndQueuesBalanceCheck(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.MaxInFlight = confutil.P(2) // only poll once then we're full
		conf.GasPrice.FixedGasPrice = 1
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)

	// Fill first slot with a stage controller
	o.inFlightTxs = []*inFlightTransactionStageController{mockIT}

	// Return a single transaction - note there's a highest nonce query on startup before the first poll, so we query twice
	for i := 0; i < 2; i++ {
		m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{"from", "nonce"}).AddRow(
			o.signingAddress, 2,
		))
	}
	// Do not return any submissions for it
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	addressBalanceChecked := make(chan bool)
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(pldtypes.Uint64ToUint256(100), nil).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
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
	o.state = OrchestratorStateRunning

	for i := 0; i < 2; i++ {
		// return empty rows - once for max nonce calculation, and then again for the actual query
		m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{}))
	}

	ocDone, _ := o.Start(ctx)

	// It should go idle
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for o.state != OrchestratorStateIdle && !t.Failed() {
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
		conf.GasPrice.FixedGasPrice = 1
	})
	defer done()

	mockIT, txState := newInflightTransaction(o, 1, func(tx *DBPublicTxn) {
		tx.Gas = 100
	})
	txState.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Int64ToInt256(1000),
		},
	})

	// Fill first slot with a stage controller
	o.inFlightTxs = []*inFlightTransactionStageController{mockIT}
	o.state = OrchestratorStateRunning
	o.lastQueueUpdate = time.Now()

	for range 2 {
		// return empty rows - once for max nonce calculation, and then again for the actual query
		m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{}))
	}

	// Mock the insufficient balance on the account that's submitting
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(pldtypes.Uint64ToUint256(0), nil)

	oDone, err := o.Start(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return o.state == OrchestratorStateWaiting
	}, time.Second, 10*time.Millisecond)

	o.Stop()
	<-oDone
}
