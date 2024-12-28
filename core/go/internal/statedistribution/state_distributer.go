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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	pb "github.com/kaleido-io/paladin/core/pkg/proto/engine"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const RETRY_TIMEOUT = 5 * time.Second

func NewStateDistributer(
	ctx context.Context,
	transportManager components.TransportManager,
	stateManager components.StateManager,
	keyManager components.KeyManager,
	persistence persistence.Persistence,
	conf *pldconf.DistributerConfig,
) StateDistributer {
	sd := &stateDistributer{
		persistence:      persistence,
		inputChan:        make(chan *components.StateDistributionWithData),
		retryChan:        make(chan string),
		acknowledgedChan: make(chan string),
		pendingMap:       make(map[string]*components.StateDistributionWithData),
		stateManager:     stateManager,
		keyManager:       keyManager,
		transportManager: transportManager,
		localNodeName:    transportManager.LocalNodeName(),
		retry:            retry.NewRetryIndefinite(&pldconf.RetryConfig{}, &pldconf.GenericRetryDefaults.RetryConfig),
	}
	sd.acknowledgementWriter = NewAcknowledgementWriter(ctx, sd.persistence, &conf.AcknowledgementWriter)
	sd.receivedStateWriter = NewReceivedStateWriter(ctx, stateManager, persistence, &conf.ReceivedObjectWriter)

	return sd
}

type StateDistributionPersisted struct {
	Created               tktypes.Timestamp  `json:"created" gorm:"column:created;autoCreateTime:nano"`
	ID                    string             `json:"id"`
	StateID               tktypes.HexBytes   `json:"stateID"`
	IdentityLocator       string             `json:"identityLocator"`
	DomainName            string             `json:"domainName"`
	ContractAddress       tktypes.EthAddress `json:"contractAddress"`
	NullifierAlgorithm    *string            `json:"nullifierAlgorithm,omitempty"`
	NullifierVerifierType *string            `json:"nullifierVerifierType,omitempty"`
	NullifierPayloadType  *string            `json:"nullifierPayloadType,omitempty"`
}

/*
StateDistributer is a component that is responsible for distributing state to remote parties

	it runs in its own goroutine and periodically sends states to the intended recipients
	until each recipient has acknowledged receipt of the state.

	This operates on in-memory data but will initialize from persistent storage on startup
*/
type StateDistributer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context)
	BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) ([]*components.NullifierUpsert, error)
	DistributeStates(ctx context.Context, stateDistributions []*components.StateDistributionWithData)
	HandleStateProducedEvent(ctx context.Context, stateProducedEvent *pb.StateProducedEvent, distributingNode string)
	HandleStateAcknowledgedEvent(ctx context.Context, messagePayload []byte)
}

type stateDistributer struct {
	runCtx                context.Context
	stopRunCtx            context.CancelFunc
	persistence           persistence.Persistence
	stateManager          components.StateManager
	keyManager            components.KeyManager
	inputChan             chan *components.StateDistributionWithData
	retryChan             chan string
	acknowledgedChan      chan string
	pendingMap            map[string]*components.StateDistributionWithData
	acknowledgementWriter *acknowledgementWriter
	receivedStateWriter   *receivedStateWriter
	transportManager      components.TransportManager
	localNodeName         string
	retry                 *retry.Retry
}

func (sd *stateDistributer) Start(bgCtx context.Context) error {
	sd.runCtx, sd.stopRunCtx = context.WithCancel(bgCtx)
	ctx := sd.runCtx
	log.L(ctx).Info("stateDistributer:Start")

	sd.acknowledgementWriter.Start()
	sd.receivedStateWriter.Start()

	// TODO: This needs to be a worker per-peer - probably a whole state distributor per peer that can be swapped in/out.
	// Currently it only runs on startup, and pushes all state distributions from before the startup time into the distributor.
	startTime := tktypes.TimestampNow()
	go func() {
		page := 0
		dispatched := 0
		var lastEntry *StateDistributionPersisted
		finished := false
		for !finished {
			err := sd.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
				page++
				var stateDistributions []*StateDistributionPersisted
				query := sd.persistence.DB().Table("state_distributions").
					Select("state_distributions.*").
					Joins("LEFT JOIN state_distribution_acknowledgments ON state_distributions.id = state_distribution_acknowledgments.state_distribution").
					Where("state_distribution_acknowledgments.id IS NULL").
					Where("created < ?", startTime).
					Order("created").
					Limit(100)
				if lastEntry != nil {
					query = query.Where("created > ?", lastEntry.Created)
				}
				err = query.Find(&stateDistributions).Error

				if err != nil {
					log.L(ctx).Errorf("Error getting state distributions: %s", err)
					return true, err
				}

				log.L(ctx).Infof("stateDistributer loaded %d state distributions on startup (page=%d)", len(stateDistributions), page)

				for _, stateDistribution := range stateDistributions {
					state, err := sd.stateManager.GetState(ctx, sd.persistence.DB(), /* no TX for now */
						stateDistribution.DomainName, stateDistribution.ContractAddress, stateDistribution.StateID, true, false)
					if err != nil {
						log.L(ctx).Errorf("Error getting state: %s", err)
						continue
					}

					sd.inputChan <- &components.StateDistributionWithData{
						ID:                    stateDistribution.ID,
						StateID:               stateDistribution.StateID.String(),
						IdentityLocator:       stateDistribution.IdentityLocator,
						Domain:                stateDistribution.DomainName,
						ContractAddress:       stateDistribution.ContractAddress.String(),
						SchemaID:              state.Schema.String(),
						StateDataJson:         string(state.Data),
						NullifierAlgorithm:    stateDistribution.NullifierAlgorithm,
						NullifierVerifierType: stateDistribution.NullifierVerifierType,
						NullifierPayloadType:  stateDistribution.NullifierPayloadType,
					}

					dispatched++
					lastEntry = stateDistribution
				}
				finished = (len(stateDistributions) == 0)
				return false, nil
			})
			if err != nil {
				log.L(ctx).Warnf("exiting before sending all recovered state distributions")
			}
		}
		log.L(ctx).Infof("stateDistributer finished startup recovery after dispatching %d distributions", dispatched)
	}()

	go func() {
		log.L(ctx).Info("stateDistributer:Loop starting loop")
		for {
			log.L(ctx).Debug("stateDistributer:Loop waiting for next event")

			select {
			case <-ctx.Done():
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

func (sd *stateDistributer) buildNullifier(ctx context.Context, krc components.KeyResolutionContextLazyDB, s *components.StateDistributionWithData) (*components.NullifierUpsert, error) {
	// We need to call the signing engine with the local identity to build the nullifier
	log.L(ctx).Infof("Generating nullifier for state %s on node %s (algorithm=%s,verifierType=%s,payloadType=%s)",
		s.StateID, sd.localNodeName, *s.NullifierAlgorithm, *s.NullifierVerifierType, *s.NullifierPayloadType)

	// We require a fully qualified identifier for the local node in this function
	identifier, node, err := tktypes.PrivateIdentityLocator(s.IdentityLocator).Validate(ctx, "", false)
	if err != nil || node != sd.localNodeName {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierNotLocal)
	}

	// Call the signing engine to build the nullifier
	var nulliferBytes []byte
	mapping, err := krc.KeyResolverLazyDB().ResolveKey(identifier, *s.NullifierAlgorithm, *s.NullifierVerifierType)
	if err == nil {
		nulliferBytes, err = sd.keyManager.Sign(ctx, mapping, *s.NullifierPayloadType, []byte(s.StateDataJson))
	}
	if err != nil || len(nulliferBytes) == 0 {
		return nil, i18n.WrapError(ctx, err, msgs.MsgStateDistributorNullifierFail, s.StateID)
	}
	return &components.NullifierUpsert{
		ID:    nulliferBytes,
		State: tktypes.MustParseHexBytes(s.StateID),
	}, nil
}

func (sd *stateDistributer) withKeyResolutionContext(ctx context.Context, fn func(krc components.KeyResolutionContextLazyDB) error) (err error) {

	// Unlikely we'll be resolving any new identities on this path - if we do, we'll start a new DB transaction
	// Note: This requires we're not on an existing DB TX coming into this function
	krc := sd.keyManager.NewKeyResolutionContextLazyDB(ctx)
	defer func() {
		if err == nil {
			err = krc.Commit()
		} else {
			krc.Rollback()
		}
	}()

	err = fn(krc)
	return err // note we require err to be set before return
}

func (sd *stateDistributer) BuildNullifiers(ctx context.Context, stateDistributions []*components.StateDistributionWithData) (nullifiers []*components.NullifierUpsert, err error) {

	nullifiers = []*components.NullifierUpsert{}
	err = sd.withKeyResolutionContext(ctx, func(krc components.KeyResolutionContextLazyDB) error {
		for _, s := range stateDistributions {
			if s.NullifierAlgorithm == nil || s.NullifierVerifierType == nil || s.NullifierPayloadType == nil {
				log.L(ctx).Debugf("No nullifier required for state %s on node %s", s.ID, sd.localNodeName)
				continue
			}

			nullifier, err := sd.buildNullifier(ctx, krc, s)
			if err != nil {
				return err
			}

			nullifiers = append(nullifiers, nullifier)
		}
		return nil
	})
	return nullifiers, err
}

func (sd *stateDistributer) Stop(ctx context.Context) {
	sd.stopRunCtx()
	sd.acknowledgementWriter.Stop()
	sd.receivedStateWriter.Stop()
}
