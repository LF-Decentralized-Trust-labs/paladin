/*
 * Copyright © 2025 Kaleido, Inc.
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

package sequencer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/coordinator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/syncpoints"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/core/mocks/componentsmocks"
	"github.com/LFDT-Paladin/paladin/core/mocks/persistencemocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test utilities and mocks for sequencer lifecycle testing
type sequencerLifecycleTestMocks struct {
	components       *componentsmocks.AllComponents
	domainManager    *componentsmocks.DomainManager
	stateManager     *componentsmocks.StateManager
	transportManager *componentsmocks.TransportManager
	persistence      *persistencemocks.Persistence
	txManager        *componentsmocks.TXManager
	publicTxManager  *componentsmocks.PublicTxManager
	keyManager       *componentsmocks.KeyManager
	domainAPI        *componentsmocks.DomainSmartContract
	transportWriter  *transport.MockTransportWriter
	originator       *originator.MockSeqOriginator
	coordinator      *coordinator.MockSeqCoordinator
	syncPoints       *syncpoints.MockSyncPoints
	metrics          *metrics.MockDistributedSequencerMetrics
}

func newSequencerLifecycleTestMocks(t *testing.T) *sequencerLifecycleTestMocks {

	return &sequencerLifecycleTestMocks{
		components:       componentsmocks.NewAllComponents(t),
		domainManager:    componentsmocks.NewDomainManager(t),
		stateManager:     componentsmocks.NewStateManager(t),
		transportManager: componentsmocks.NewTransportManager(t),
		persistence:      persistencemocks.NewPersistence(t),
		txManager:        componentsmocks.NewTXManager(t),
		publicTxManager:  componentsmocks.NewPublicTxManager(t),
		keyManager:       componentsmocks.NewKeyManager(t),
		domainAPI:        componentsmocks.NewDomainSmartContract(t),
		transportWriter:  transport.NewMockTransportWriter(t),
		originator:       originator.NewMockSeqOriginator(t),
		coordinator:      coordinator.NewMockSeqCoordinator(t),
		syncPoints:       syncpoints.NewMockSyncPoints(t),
		metrics:          metrics.NewMockDistributedSequencerMetrics(t),
	}
}

func (m *sequencerLifecycleTestMocks) setupDefaultExpectations(ctx context.Context, contractAddr *pldtypes.EthAddress) {
	// Setup default component expectations
	m.components.EXPECT().DomainManager().Return(m.domainManager).Maybe()
	m.components.EXPECT().StateManager().Return(m.stateManager).Maybe()
	m.components.EXPECT().TransportManager().Return(m.transportManager).Maybe()
	m.components.EXPECT().Persistence().Return(m.persistence).Maybe()
	m.components.EXPECT().TxManager().Return(m.txManager).Maybe()
	m.components.EXPECT().PublicTxManager().Return(m.publicTxManager).Maybe()
	m.components.EXPECT().KeyManager().Return(m.keyManager).Maybe()
	// m.components.EXPECT().MetricsManager().Return(m.metricsManager).Maybe()

	// Setup persistence expectations
	m.persistence.EXPECT().NOTX().Return(nil).Maybe()

	// Setup transport manager expectations
	m.transportManager.EXPECT().LocalNodeName().Return("test-node").Maybe()

	// Setup domain manager expectations
	m.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, contractAddr).Return(m.domainAPI, nil).Maybe()

	// // Setup domain API expectations
	// m.domainAPI.EXPECT().Domain().Return("test-domain").Maybe()
	// m.domainAPI.EXPECT().ContractConfig().Return(&mockContractConfig{}).Maybe()
}

type mockContractConfig struct{}

func (m *mockContractConfig) GetCoordinatorSelection() interface{} {
	return nil
}

func (m *mockContractConfig) GetSubmitterSelection() interface{} {
	return nil
}

func newSequencerManagerForTesting(t *testing.T, mocks *sequencerLifecycleTestMocks) *sequencerManager {
	ctx := context.Background()
	config := &pldconf.SequencerConfig{}

	sm := &sequencerManager{
		ctx:                           ctx,
		config:                        config,
		components:                    mocks.components,
		nodeName:                      "test-node",
		sequencersLock:                sync.RWMutex{},
		sequencers:                    make(map[string]*sequencer),
		metrics:                       mocks.metrics,
		targetActiveCoordinatorsLimit: 2,
		targetActiveSequencersLimit:   2,
	}

	return sm
}

func newSequencerForTesting(contractAddr *pldtypes.EthAddress, mocks *sequencerLifecycleTestMocks) *sequencer {
	return &sequencer{
		ctx:             context.Background(),
		contractAddress: contractAddr.String(),
		originator:      mocks.originator,
		coordinator:     mocks.coordinator,
		transportWriter: mocks.transportWriter,
		lastTXTime:      time.Now(),
	}
}

// Test sequencer interface methods
func TestSequencer_GetCoordinator(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	coordinator := seq.GetCoordinator()
	assert.Equal(t, mocks.coordinator, coordinator)
}

func TestSequencer_GetOriginator(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	originator := seq.GetOriginator()
	assert.Equal(t, mocks.originator, originator)
}

func TestSequencer_GetTransportWriter(t *testing.T) {
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	seq := newSequencerForTesting(contractAddr, mocks)

	transportWriter := seq.GetTransportWriter()
	assert.Equal(t, mocks.transportWriter, transportWriter)
}

func TestSequencerManager_LoadSequencer_NewSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for new sequencer creation
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mockDomainSmartContract := componentsmocks.NewDomainSmartContract(t)
	mockDomain := componentsmocks.NewDomain(t)
	mockDomainSmartContract.EXPECT().Domain().Return(mockDomain).Twice()
	mockDomainSmartContract.EXPECT().ContractConfig().Return(&prototk.ContractConfig{StaticCoordinator: proto.String("test-coordinator")}).Maybe()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(mockDomainSmartContract, nil)
	mocks.stateManager.EXPECT().NewDomainContext(ctx, mockDomain, *contractAddr).Return(componentsmocks.NewDomainContext(t)).Twice()

	// Setup transport writer creation
	mocks.transportWriter.EXPECT().SendDispatched(ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Setup originator creation expectations
	mocks.originator.EXPECT().SetActiveCoordinator(ctx, mock.Anything).Maybe()

	// Setup coordinator creation expectations
	mocks.coordinator.EXPECT().UpdateOriginatorNodePool(ctx, mock.Anything).Maybe()
	mocks.coordinator.EXPECT().GetActiveCoordinatorNode(ctx).Return("test-coordinator").Maybe()

	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	// Create a mock private transaction
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, mockDomainSmartContract, tx)

	// Verify results
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.GetCoordinator())
	assert.NotNil(t, result.GetOriginator())
	assert.NotNil(t, result.GetTransportWriter())

	// Verify sequencer was stored
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_LoadSequencer_ExistingSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create and store an existing sequencer
	existingSeq := newSequencerForTesting(contractAddr, mocks)
	sm.sequencers[contractAddr.String()] = existingSeq

	// Setup expectations for existing sequencer
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil).Once()

	// Setup coordinator expectations for existing sequencer
	mocks.originator.EXPECT().GetCurrentCoordinator().Return("test-coordinator").Once()

	// Create a mock private transaction
	tx := &components.PrivateTransaction{
		ID: uuid.New(),
		PreAssembly: &components.TransactionPreAssembly{
			RequiredVerifiers: []*prototk.ResolveVerifierRequest{
				{Lookup: "verifier1@node1"},
			},
		},
	}

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, tx)

	// Verify results
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, existingSeq, result)

	// Verify lastTXTime was updated
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	seq := sm.sequencers[contractAddr.String()]
	assert.True(t, seq.lastTXTime.After(time.Now().Add(-time.Second)))
}

func TestSequencerManager_LoadSequencer_NoDomainAPI(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager returning nil
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("contract not found")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_DomainManagerError(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations for domain manager error
	mocks.components.EXPECT().DomainManager().Return(mocks.domainManager).Once()
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, errors.New("database error")).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.NoError(t, err) // This should not error, just return nil
	assert.Nil(t, result)
}

func TestSequencerManager_LoadSequencer_NoDomainProvided(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Setup expectations
	mocks.setupDefaultExpectations(ctx, contractAddr)
	mocks.domainManager.EXPECT().GetSmartContractByAddress(ctx, mock.Anything, *contractAddr).Return(nil, nil)
	mocks.metrics.EXPECT().SetActiveSequencers(0).Once()

	// Call LoadSequencer
	result, err := sm.LoadSequencer(ctx, nil, *contractAddr, nil, nil)

	// Verify results
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "No domain provided to create sequencer")
	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_stopLowestPrioritySequencer_NoSequencers(t *testing.T) {
	ctx := context.Background()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// No sequencers in the map
	sm.sequencersLock.Lock()
	defer sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Should not panic or error
	assert.Empty(t, sm.sequencers)
}

func TestSequencerManager_stopLowestPrioritySequencer_SequencerAlreadyClosing(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer that's already closing
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Flush)

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify sequencer is still in the map (not stopped)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.Contains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_stopLowestPrioritySequencer_IdleSequencer(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create an idle sequencer
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.originator.EXPECT().Stop().Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify sequencer was removed
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr.String())
}

func TestSequencerManager_stopLowestPrioritySequencer_LowestPriority(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	// Create two sequencers with different lastTXTime
	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-2 * time.Hour) // Older

	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-1 * time.Hour) // Newer

	// Setup expectations - both are active, seq1 should be stopped
	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks1.coordinator.EXPECT().Stop().Once()
	mocks1.originator.EXPECT().Stop().Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencersLock.Unlock()

	// Call stopLowestPrioritySequencer
	sm.stopLowestPrioritySequencer(ctx)

	// Verify only seq1 was removed (lowest priority)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr1.String())
	assert.Contains(t, sm.sequencers, contractAddr2.String())
}

func TestSequencerManager_updateActiveCoordinators_NoActiveCoordinators(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer with inactive coordinator
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Idle)
	mocks.metrics.EXPECT().SetActiveCoordinators(0).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_updateActiveCoordinators_ActiveCoordinators(t *testing.T) {
	ctx := context.Background()
	contractAddr := pldtypes.RandAddress()
	mocks := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks)

	// Create a sequencer with active coordinator
	seq := newSequencerForTesting(contractAddr, mocks)
	mocks.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks.metrics.EXPECT().SetActiveCoordinators(1).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr.String()] = seq
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	mocks.metrics.AssertExpectations(t)
}

func TestSequencerManager_updateActiveCoordinators_ExceedsLimit(t *testing.T) {
	ctx := context.Background()
	contractAddr1 := pldtypes.RandAddress()
	contractAddr2 := pldtypes.RandAddress()
	contractAddr3 := pldtypes.RandAddress()
	mocks1 := newSequencerLifecycleTestMocks(t)
	mocks2 := newSequencerLifecycleTestMocks(t)
	mocks3 := newSequencerLifecycleTestMocks(t)
	sm := newSequencerManagerForTesting(t, mocks1)

	// Set limit to 2
	sm.targetActiveCoordinatorsLimit = 2

	// Create three sequencers with active coordinators
	seq1 := newSequencerForTesting(contractAddr1, mocks1)
	seq1.lastTXTime = time.Now().Add(-3 * time.Hour) // Oldest

	seq2 := newSequencerForTesting(contractAddr2, mocks2)
	seq2.lastTXTime = time.Now().Add(-2 * time.Hour) // Middle

	seq3 := newSequencerForTesting(contractAddr3, mocks3)
	seq3.lastTXTime = time.Now().Add(-1 * time.Hour) // Newest

	// Setup expectations - all are active, seq1 should be stopped
	mocks1.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks2.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks3.coordinator.EXPECT().GetCurrentState().Return(coordinator.State_Active)
	mocks1.coordinator.EXPECT().Stop().Once()
	mocks1.metrics.EXPECT().SetActiveCoordinators(3).Once()

	sm.sequencersLock.Lock()
	sm.sequencers[contractAddr1.String()] = seq1
	sm.sequencers[contractAddr2.String()] = seq2
	sm.sequencers[contractAddr3.String()] = seq3
	sm.sequencersLock.Unlock()

	// Call updateActiveCoordinators
	sm.updateActiveCoordinators(ctx)

	// Verify only seq1 was removed (lowest priority)
	sm.sequencersLock.RLock()
	defer sm.sequencersLock.RUnlock()
	assert.NotContains(t, sm.sequencers, contractAddr1.String())
	assert.Contains(t, sm.sequencers, contractAddr2.String())
	assert.Contains(t, sm.sequencers, contractAddr3.String())

	mocks1.metrics.AssertExpectations(t)
}
