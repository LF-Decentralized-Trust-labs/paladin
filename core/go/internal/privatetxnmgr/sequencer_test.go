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

package privatetxnmgr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/mocks/privatetxnmgrmocks"
	"github.com/kaleido-io/paladin/core/mocks/statedistributionmocks"

	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type sequencerDepencyMocks struct {
	allComponents       *componentmocks.AllComponents
	domainSmartContract *componentmocks.DomainSmartContract
	domainContext       *componentmocks.DomainContext
	domainMgr           *componentmocks.DomainManager
	transportManager    *componentmocks.TransportManager
	stateStore          *componentmocks.StateManager
	keyManager          *componentmocks.KeyManager
	endorsementGatherer *privatetxnmgrmocks.EndorsementGatherer
	publisher           *privatetxnmgrmocks.Publisher
	identityResolver    *componentmocks.IdentityResolver
	stateDistributer    *statedistributionmocks.StateDistributer
	txManager           *componentmocks.TXManager
	transportWriter     *privatetxnmgrmocks.TransportWriter
}

func newSequencerForTesting(t *testing.T, ctx context.Context, domainAddress *tktypes.EthAddress) (*Sequencer, *sequencerDepencyMocks, func()) {
	if domainAddress == nil {
		domainAddress = tktypes.MustEthAddress(tktypes.RandHex(20))
	}

	mocks := &sequencerDepencyMocks{
		allComponents:       componentmocks.NewAllComponents(t),
		domainSmartContract: componentmocks.NewDomainSmartContract(t),
		domainContext:       componentmocks.NewDomainContext(t),
		domainMgr:           componentmocks.NewDomainManager(t),
		transportManager:    componentmocks.NewTransportManager(t),
		stateStore:          componentmocks.NewStateManager(t),
		keyManager:          componentmocks.NewKeyManager(t),
		endorsementGatherer: privatetxnmgrmocks.NewEndorsementGatherer(t),
		publisher:           privatetxnmgrmocks.NewPublisher(t),
		identityResolver:    componentmocks.NewIdentityResolver(t),
		stateDistributer:    statedistributionmocks.NewStateDistributer(t),
		txManager:           componentmocks.NewTXManager(t),
		transportWriter:     privatetxnmgrmocks.NewTransportWriter(t),
	}
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, *domainAddress).Maybe().Return(mocks.domainSmartContract, nil)
	p, persistenceDone, err := persistence.NewUnitTestPersistence(ctx)
	require.NoError(t, err)
	mocks.allComponents.On("Persistence").Return(p).Maybe()
	mocks.endorsementGatherer.On("DomainContext").Return(mocks.domainContext).Maybe()
	mocks.domainSmartContract.On("Address").Return(*domainAddress).Maybe()

	syncPoints := syncpoints.NewSyncPoints(ctx, &pldconf.FlushWriterConfig{}, p, mocks.txManager)
	o := NewSequencer(ctx, tktypes.RandHex(16), *domainAddress, &pldconf.PrivateTxManagerSequencerConfig{}, mocks.allComponents, mocks.domainSmartContract, mocks.endorsementGatherer, mocks.publisher, syncPoints, mocks.identityResolver, mocks.stateDistributer, mocks.transportWriter, 30*time.Second)
	ocDone, err := o.Start(ctx)
	require.NoError(t, err)

	return o, mocks, func() {
		<-ocDone
		persistenceDone()
	}

}

func waitForChannel[T any](t *testing.T, ch chan T) T {
	ticker := time.NewTicker(100 * time.Millisecond)
	if _, ok := t.Deadline(); !ok {
		//no deadline set - assuming we are in a debug session
		ticker = time.NewTicker(1 * time.Hour)
	}
	defer ticker.Stop()
	for {
		select {
		case ret := <-ch:
			return ret
		case <-ticker.C:
			require.Fail(t, "timeout waiting for channel")
		}
	}
}

func TestNewSequencerProcessNewTransactionAssemblyFailed(t *testing.T) {
	// Test that the sequencer can receive a new transaction and begin processing it.
	// In this test, it doesn't need to get any further than assembly

	ctx := context.Background()

	testOc, dependencyMocks, _ := newSequencerForTesting(t, ctx, nil)

	newTxID := uuid.New()
	testTx := &components.PrivateTransaction{
		ID: newTxID,
		Inputs: &components.TransactionInputs{
			From: "alice",
		},
		PreAssembly: &components.TransactionPreAssembly{},
	}

	waitForFinalize := make(chan bool, 1)
	dependencyMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("fail assembly. Just happy that we got this far"))
	dependencyMocks.publisher.On("PublishTransactionAssembleFailedEvent", mock.Anything, newTxID.String(), "mock.Anything").Return(nil).Run(func(args mock.Arguments) {
		waitForFinalize <- true
	})
	//As we are using a mock publisher, the assemble failed event never gets back onto the event loop to trigger the next step ( finalization )
	// but that's ok, we have proven what we set out to, the sequencer can handle a new transaction and begin processing it
	// we could then emulate the publisher and trigger the next iteration of the loop but that would be better done with a less isolated test

	assert.Empty(t, testOc.incompleteTxSProcessMap)

	// when incomplete tx is more than max concurrent
	testOc.maxConcurrentProcess = 0
	assert.True(t, testOc.ProcessNewTransaction(ctx, testTx))

	// gets add when the queue is not full
	testOc.maxConcurrentProcess = 10
	assert.Empty(t, testOc.incompleteTxSProcessMap)
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	assert.Equal(t, 1, len(testOc.incompleteTxSProcessMap))

	_ = waitForChannel(t, waitForFinalize)

	// add again doesn't cause a repeat process of the current stage context
	assert.False(t, testOc.ProcessNewTransaction(ctx, testTx))
	assert.Equal(t, 1, len(testOc.incompleteTxSProcessMap))

}

func TestSequencerPollingLoopStop(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testOc, _, ocDone := newSequencerForTesting(t, ctx, nil)
	defer ocDone()
	testOc.TriggerSequencerEvaluation()
	testOc.Stop()

}

func TestSequencerPollingLoopCancelContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	_, _, ocDone := newSequencerForTesting(t, ctx, nil)
	defer ocDone()

	cancel()
}
