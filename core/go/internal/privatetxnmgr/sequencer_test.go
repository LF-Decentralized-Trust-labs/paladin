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

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/ptmgrtypesmocks"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type sequencerDepencyMocks struct {
	allComponents       *componentsmocks.AllComponents
	privateTxManager    *componentsmocks.PrivateTxManager
	domainSmartContract *componentsmocks.DomainSmartContract
	domainContext       *componentsmocks.DomainContext
	domainMgr           *componentsmocks.DomainManager
	domain              *componentsmocks.Domain
	transportManager    *componentsmocks.TransportManager
	stateStore          *componentsmocks.StateManager
	keyManager          *componentsmocks.KeyManager
	endorsementGatherer *ptmgrtypesmocks.EndorsementGatherer
	publisher           *ptmgrtypesmocks.Publisher
	identityResolver    *componentsmocks.IdentityResolver
	txManager           *componentsmocks.TXManager
	pubTxManager        *componentsmocks.PublicTxManager
	transportWriter     *ptmgrtypesmocks.TransportWriter
}

func newSequencerForTesting(t *testing.T, ctx context.Context, domainAddress *pldtypes.EthAddress) (*Sequencer, *sequencerDepencyMocks, func()) {
	if domainAddress == nil {
		domainAddress = pldtypes.MustEthAddress(pldtypes.RandHex(20))
	}

	mocks := &sequencerDepencyMocks{
		allComponents:       componentsmocks.NewAllComponents(t),
		privateTxManager:    componentsmocks.NewPrivateTxManager(t),
		domainSmartContract: componentsmocks.NewDomainSmartContract(t),
		domainContext:       componentsmocks.NewDomainContext(t),
		domainMgr:           componentsmocks.NewDomainManager(t),
		domain:              componentsmocks.NewDomain(t),
		transportManager:    componentsmocks.NewTransportManager(t),
		stateStore:          componentsmocks.NewStateManager(t),
		keyManager:          componentsmocks.NewKeyManager(t),
		endorsementGatherer: ptmgrtypesmocks.NewEndorsementGatherer(t),
		publisher:           ptmgrtypesmocks.NewPublisher(t),
		identityResolver:    componentsmocks.NewIdentityResolver(t),
		txManager:           componentsmocks.NewTXManager(t),
		pubTxManager:        componentsmocks.NewPublicTxManager(t),
		transportWriter:     ptmgrtypesmocks.NewTransportWriter(t),
	}
	mocks.allComponents.On("StateManager").Return(mocks.stateStore).Maybe()
	mocks.allComponents.On("DomainManager").Return(mocks.domainMgr).Maybe()
	mocks.allComponents.On("TransportManager").Return(mocks.transportManager).Maybe()
	mocks.allComponents.On("KeyManager").Return(mocks.keyManager).Maybe()
	mocks.allComponents.On("TxManager").Return(mocks.txManager).Maybe()
	mocks.allComponents.On("PublicTxManager").Return(mocks.pubTxManager).Maybe()
	mocks.domainMgr.On("GetSmartContractByAddress", mock.Anything, mock.Anything, *domainAddress).Maybe().Return(mocks.domainSmartContract, nil)
	p, persistenceDone, err := persistence.NewUnitTestPersistence(ctx, "privatetxmgr")
	require.NoError(t, err)
	mocks.allComponents.On("Persistence").Return(p).Maybe()
	mocks.endorsementGatherer.On("DomainContext").Return(mocks.domainContext).Maybe()
	mocks.domainSmartContract.On("Domain").Return(mocks.domain).Maybe()
	mocks.domainSmartContract.On("Address").Return(*domainAddress).Maybe()
	mocks.domainSmartContract.On("ContractConfig").Return(&prototk.ContractConfig{
		CoordinatorSelection: prototk.ContractConfig_COORDINATOR_ENDORSER,
	})

	mocks.stateStore.On("NewDomainContext", mock.Anything, mocks.domain, *domainAddress, mock.Anything).Return(mocks.domainContext).Maybe()
	//mocks.domain.On("Configuration").Return(&prototk.DomainConfig{}).Maybe()

	syncPoints := syncpoints.NewSyncPoints(ctx, &pldconf.FlushWriterConfig{}, p, mocks.txManager, mocks.pubTxManager, mocks.transportManager)
	o, err := NewSequencer(ctx, mocks.privateTxManager, pldtypes.RandHex(16), *domainAddress, &pldconf.PrivateTxManagerSequencerConfig{}, mocks.allComponents, mocks.domainSmartContract, mocks.endorsementGatherer, mocks.publisher, syncPoints, mocks.identityResolver, mocks.transportWriter, 30*time.Second, 0)
	require.NoError(t, err)
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
		PreAssembly: &components.TransactionPreAssembly{
			TransactionSpecification: &prototk.TransactionSpecification{
				From: "alice",
			},
		},
	}
	dependencyMocks.txManager.On("GetResolvedTransactionByID", mock.Anything, mock.Anything).Return(&components.ResolvedTransaction{
		Transaction: &pldapi.Transaction{
			ID: &newTxID,
			TransactionBase: pldapi.TransactionBase{
				Domain: "domain1",
				To:     &testOc.contractAddress,
			},
		},
	}, nil)

	waitForFinalize := make(chan bool, 1)
	dependencyMocks.domain.On("Name").Return("domain1")
	dependencyMocks.domainSmartContract.On("AssembleTransaction", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("fail assembly"))
	dependencyMocks.publisher.On("PublishTransactionAssembleFailedEvent", mock.Anything, newTxID.String(), mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
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
