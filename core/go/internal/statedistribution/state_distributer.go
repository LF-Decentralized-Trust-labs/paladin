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

package statedistribution

import (
	"context"
	"time"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const RETRY_TIMEOUT = 5 * time.Second

func NewStateDistributer(ctx context.Context, nodeID string, transportManager components.TransportManager, stateManager components.StateManager, persistence persistence.Persistence, conf *pldconf.StateDistributerConfig) StateDistributer {
	sd := &stateDistributer{
		persistence:      persistence,
		stopChan:         make(chan struct{}),
		inputChan:        make(chan *StateDistribution),
		retryChan:        make(chan string),
		acknowledgedChan: make(chan string),
		pendingMap:       make(map[string]*StateDistribution),
		stateManager:     stateManager,
		transportManager: transportManager,
		nodeID:           nodeID,
	}
	sd.acknowledgementWriter = NewAcknowledgementWriter(ctx, sd.persistence, &conf.AcknowledgementWriter)
	sd.receivedStateWriter = NewReceivedStateWriter(ctx, stateManager, persistence, &conf.ReceivedStateWriter)

	return sd
}

type StateDistributionPersisted struct {
	ID              string `json:"id"`
	StateID         string `json:"stateID"`
	IdentityLocator string `json:"identityLocator"`
	DomainName      string `json:"domainName"`
	ContractAddress string `json:"contractAddress"`
}

// A StateDistribution is an intent to send private data for a given state to a remote party
type StateDistribution struct {
	ID              string
	StateID         string
	IdentityLocator string
	Domain          string
	ContractAddress string
	SchemaID        string
	StateDataJson   string
}

/*
StateDistributer is a component that is responsible for distributing state to remote parties

	it runs in its own goroutine and periodically sends states to the intended recipients
	until each recipient has acknowledged receipt of the state.

	This operates on in-memory data but will initialise from persistent storage on startup
*/
type StateDistributer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context)
	AcknowledgeState(ctx context.Context, stateID string)
	DistributeStates(ctx context.Context, stateDistributions []*StateDistribution)
}

type stateDistributer struct {
	persistence           persistence.Persistence
	stateManager          components.StateManager
	stopChan              chan struct{}
	inputChan             chan *StateDistribution
	retryChan             chan string
	acknowledgedChan      chan string
	pendingMap            map[string]*StateDistribution
	acknowledgementWriter *acknowledgementWriter
	receivedStateWriter   *receivedStateWriter
	transportManager      components.TransportManager
	nodeID                string
}

func (sd *stateDistributer) Start(ctx context.Context) error {
	log.L(ctx).Info("stateDistributer:Start")
	err := sd.transportManager.RegisterClient(ctx, sd)
	if err != nil {
		log.L(ctx).Errorf("Error registering transport client: %s", err)
		return err
	}

	var stateDistributions []StateDistributionPersisted
	err = sd.persistence.DB().Table("state_distributions").
		Select("state_distributions.*").
		Joins("LEFT JOIN state_distribution_acknowledgments ON state_distributions.id = state_distribution_acknowledgments.state_distribution").
		Where("state_distribution_acknowledgments.id IS NULL").
		Find(&stateDistributions).Error

	if err != nil {
		log.L(ctx).Errorf("Error getting state distributions: %s", err)
		return err
	}
	log.L(ctx).Infof("stateDistributer:Start loaded %d state distributions on startup", len(stateDistributions))

	for _, stateDistribution := range stateDistributions {
		state, err := sd.stateManager.GetState(ctx, sd.persistence.DB(), /* no TX for now */
			stateDistribution.DomainName, *tktypes.MustEthAddress(stateDistribution.ContractAddress), tktypes.MustParseHexBytes(stateDistribution.StateID), true, false)
		if err != nil {
			log.L(ctx).Errorf("Error getting state: %s", err)
			continue
		}

		sd.inputChan <- &StateDistribution{
			ID:              stateDistribution.ID,
			StateID:         stateDistribution.StateID,
			IdentityLocator: stateDistribution.IdentityLocator,
			Domain:          stateDistribution.DomainName,
			ContractAddress: stateDistribution.ContractAddress,
			SchemaID:        state.Schema.String(),
			StateDataJson:   string(state.Data),
		}
	}

	sd.acknowledgementWriter.Start()
	sd.receivedStateWriter.Start()

	go func() {
		log.L(ctx).Info("stateDistributer:Loop starting loop")
		for {
			log.L(ctx).Debug("stateDistributer:Loop waiting for next event")

			select {
			case <-ctx.Done():
				return
			case <-sd.stopChan:
				return
			case stateDistributionID := <-sd.acknowledgedChan:
				_, stillPending := sd.pendingMap[stateDistributionID]
				if stillPending {
					log.L(ctx).Debugf("stateDistributer:Loop processing acknowledgment %s", stateDistributionID)

					delete(sd.pendingMap, stateDistributionID)
				} else {
					log.L(ctx).Debugf("stateDistributer:Loop already received acknowledgment %s", stateDistributionID)

				}
				//if we didn't find it in the map, it was already acknowledged

			case stateDistributionID := <-sd.retryChan:

				pendingDistribution, stillPending := sd.pendingMap[stateDistributionID]
				if stillPending {
					log.L(ctx).Debugf("stateDistributer:Loop retrying %s", stateDistributionID)
					sd.sendState(ctx, pendingDistribution)
				}
				//if we didn't find it in the map, it was already acknowledged

			case stateDistribution := <-sd.inputChan:
				log.L(ctx).Debugf("stateDistributer:Loop new distribution %s", stateDistribution.ID)

				sd.pendingMap[stateDistribution.ID] = stateDistribution
				sd.sendState(ctx, stateDistribution)

			}
		}
	}()
	return nil
}

func (sd *stateDistributer) Stop(ctx context.Context) {
	close(sd.stopChan)
}
