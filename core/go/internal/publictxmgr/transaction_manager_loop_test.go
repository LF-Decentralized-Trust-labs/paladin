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
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
)

func TestNewEnginePollingCancelledContext(t *testing.T) {
	ctx, ble, _, done := newTestPublicTxManager(t, false)
	done()

	polled, _ := ble.poll(ctx)
	assert.Equal(t, -1, polled)
}

func TestNewEnginePollingStoppingAnOrchestratorForFairnessControl(t *testing.T) {
	testSigningAddr1 := pldtypes.RandAddress()
	testSigningAddr2 := pldtypes.RandAddress()

	ctx, ble, m, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		mocks.disableManagerStart = true                         // we don't want the manager running... yet
		conf.Manager.MaxInFlightOrchestrators = confutil.P(1)    // we only have one slot
		conf.Manager.OrchestratorSwapTimeout = confutil.P("1ms") // we're very aggressive on swapping out
	})
	defer done()

	// Fake an inflight orchestrator for signing address 1
	existingOrchestrator := &orchestrator{
		signingAddress:              *testSigningAddr1,
		orchestratorBirthTime:       time.Now().Add(-1 * time.Hour),
		pubTxManager:                ble,
		orchestratorPollingInterval: ble.enginePollingInterval,
		state:                       OrchestratorStateRunning,
		stateEntryTime:              time.Now().Add(1 * time.Hour).Add(-1 * time.Minute),
		InFlightTxsStale:            make(chan bool, 1),
		stopProcess:                 make(chan bool, 1),
	}
	ble.inFlightOrchestrators = map[pldtypes.EthAddress]*orchestrator{
		*testSigningAddr1: existingOrchestrator, // already has an orchestrator for 0x1
	}

	// we should stop the first, and swap in the second
	m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{"from", "nonce"}).AddRow(testSigningAddr2, 12345))

	ble.poll(ctx)
	existingOrchestrator.orchestratorLoopDone = make(chan struct{})
	existingOrchestrator.orchestratorLoop()
	<-existingOrchestrator.orchestratorLoopDone
	assert.Equal(t, OrchestratorStateStopped, existingOrchestrator.state)
}

func TestNewEnginePollingExcludePausedOrchestrator(t *testing.T) {

	testSigningAddr1 := *pldtypes.RandAddress()

	ctx, ble, m, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		mocks.disableManagerStart = true                         // we don't want the manager running... yet
		conf.Manager.MaxInFlightOrchestrators = confutil.P(1)    // we only have one slot
		conf.Manager.OrchestratorSwapTimeout = confutil.P("1ms") // we're very aggressive on swapping out
	})
	defer done()

	m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{"from"}))

	// already has a running orchestrator for the address so no new orchestrator should be started
	ble.inFlightOrchestrators = map[pldtypes.EthAddress]*orchestrator{}
	ble.signingAddressesPausedUntil = map[pldtypes.EthAddress]time.Time{testSigningAddr1: time.Now().Add(1 * time.Hour)}

	ble.poll(ctx)

}
