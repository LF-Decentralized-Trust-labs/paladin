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

package preparedtxdistribution

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const RETRY_TIMEOUT = 5 * time.Second

func NewPreparedTransactionDistributer(ctx context.Context, nodeID string, transportManager components.TransportManager, txMgr components.TXManager, persistence persistence.Persistence, conf *pldconf.DistributerConfig) PreparedTransactionDistributer {
	sd := &preparedTransactionDistributer{
		persistence:      persistence,
		inputChan:        make(chan *PreparedTxnDistribution),
		retryChan:        make(chan string),
		acknowledgedChan: make(chan string),
		pendingMap:       make(map[string]*PreparedTxnDistribution),
		txMgr:            txMgr,
		transportManager: transportManager,
		nodeID:           nodeID,
		retry:            retry.NewRetryIndefinite(&pldconf.RetryConfig{}, &pldconf.GenericRetryDefaults.RetryConfig),
	}
	sd.acknowledgementWriter = NewAcknowledgementWriter(ctx, sd.persistence, &conf.AcknowledgementWriter)
	sd.receivedPreparedTransactionWriter = NewReceivedPreparedTransactionWriter(ctx, txMgr, persistence, &conf.ReceivedObjectWriter)

	return sd
}

type PreparedTxnDistributionPersisted struct {
	Created         tktypes.Timestamp `json:"created" gorm:"column:created;autoCreateTime:nano"`
	ID              string            `json:"id"`
	PreparedTxnID   string            `json:"preparedTxnID"`
	IdentityLocator string            `json:"identityLocator"`
	DomainName      string            `json:"domainName"`
	ContractAddress string            `json:"contractAddress"`
}

// A PreparedTxnDistribution is an intent to send a prepared transaction to a remote party
type PreparedTxnDistribution struct {
	ID                      string
	PreparedTxnID           string
	IdentityLocator         string
	Domain                  string
	ContractAddress         string
	PreparedTransactionJSON []byte
}

/*
PreparedTransactionDistributer is a component that is responsible for distributing prepared transactions to remote parties

	it runs in its own goroutine and periodically sends prepared transactions to the intended recipients
	until each recipient has acknowledged receipt of the prepared transaction.

	This operates on in-memory data but will initialize from persistent storage on startup
*/
type PreparedTransactionDistributer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context)
	DistributePreparedTransactions(ctx context.Context, preparedTxnDistributions []*PreparedTxnDistribution)
}

type preparedTransactionDistributer struct {
	runCtx                            context.Context
	stopRunCtx                        context.CancelFunc
	persistence                       persistence.Persistence
	txMgr                             components.TXManager
	inputChan                         chan *PreparedTxnDistribution
	retryChan                         chan string
	acknowledgedChan                  chan string
	pendingMap                        map[string]*PreparedTxnDistribution
	acknowledgementWriter             *acknowledgementWriter
	receivedPreparedTransactionWriter *receivedPreparedTransactionWriter
	transportManager                  components.TransportManager
	nodeID                            string
	retry                             *retry.Retry
}

func (sd *preparedTransactionDistributer) Start(bgCtx context.Context) error {
	sd.runCtx, sd.stopRunCtx = context.WithCancel(bgCtx)
	ctx := sd.runCtx
	log.L(ctx).Info("preparedTransactionDistributer:Start")

	sd.acknowledgementWriter.Start()
	sd.receivedPreparedTransactionWriter.Start()

	// TODO: This needs to be a worker per-peer - probably a whole distributor per peer that can be swapped in/out.
	// Currently it only runs on startup, and pushes all prepared transaction distributions from before the startup time into the distributor.
	startTime := tktypes.TimestampNow()
	go func() {
		page := 0
		dispatched := 0
		var lastEntry *PreparedTxnDistributionPersisted
		finished := false
		for !finished {
			err := sd.retry.Do(ctx, func(attempt int) (retryable bool, err error) {
				page++
				var preparedTxnDistributions []*PreparedTxnDistributionPersisted
				query := sd.persistence.DB().Table("prepared_txn_distributions").
					Select("prepared_txn_distributions.*").
					Joins("LEFT JOIN prepared_txn_distribution_acknowledgments ON prepared_txn_distributions.id = prepared_txn_distribution_acknowledgments.prepared_txn_distribution").
					Where("prepared_txn_distribution_acknowledgments.id IS NULL").
					Where("created < ?", startTime).
					Order("created").
					Limit(100)
				if lastEntry != nil {
					query = query.Where("created > ?", lastEntry.Created)
				}
				err = query.Find(&preparedTxnDistributions).Error

				if err != nil {
					log.L(ctx).Errorf("Error getting prepared transaction distributions: %s", err)
					return true, err
				}

				log.L(ctx).Infof("preparedTransactionDistributer loaded %d prepared transaction distributions on startup (page=%d)", len(preparedTxnDistributions), page)

				for _, preparedTxnDistribution := range preparedTxnDistributions {
					preparedTxnID, err := uuid.Parse(preparedTxnDistribution.PreparedTxnID)
					if err != nil {
						log.L(ctx).Errorf("Error parsing prepared transaction ID: %s", err)
						continue
					}
					preparedTransaction, err := sd.txMgr.GetPreparedTransactionByID(ctx, sd.persistence.DB() /* no TX for now */, preparedTxnID)
					if err != nil {
						log.L(ctx).Errorf("Error getting prepared transaction: %s", err)
						continue
					}

					preparedTransactionJSON, err := json.Marshal(preparedTransaction)
					if err != nil {
						log.L(ctx).Errorf("Error marshalling prepared transaction: %s", err)
						continue
					}

					sd.inputChan <- &PreparedTxnDistribution{
						ID:                      preparedTxnDistribution.ID,
						PreparedTxnID:           preparedTxnDistribution.PreparedTxnID,
						IdentityLocator:         preparedTxnDistribution.IdentityLocator,
						Domain:                  preparedTxnDistribution.DomainName,
						ContractAddress:         preparedTxnDistribution.ContractAddress,
						PreparedTransactionJSON: preparedTransactionJSON,
					}

					dispatched++
					lastEntry = preparedTxnDistribution
				}
				finished = (len(preparedTxnDistributions) == 0)
				return false, nil
			})
			if err != nil {
				log.L(ctx).Warnf("exiting before sending all recovered prepared transaction distributions")
			}
		}
		log.L(ctx).Infof("preparedTransactionDistributer finished startup recovery after dispatching %d distributions", dispatched)
	}()

	go func() {
		log.L(ctx).Info("preparedTransactionDistributer:Loop starting loop")
		for {
			log.L(ctx).Debug("preparedTransactionDistributer:Loop waiting for next event")

			select {
			case <-ctx.Done():
				return
			case preparedTxnDistributionID := <-sd.acknowledgedChan:
				_, stillPending := sd.pendingMap[preparedTxnDistributionID]
				if stillPending {
					log.L(ctx).Debugf("preparedTransactionDistributer:Loop processing acknowledgment %s", preparedTxnDistributionID)

					delete(sd.pendingMap, preparedTxnDistributionID)
				} else {
					log.L(ctx).Debugf("preparedTransactionDistributer:Loop already received acknowledgment %s", preparedTxnDistributionID)

				}
				//if we didn't find it in the map, it was already acknowledged

			case preparedTxnDistributionID := <-sd.retryChan:

				pendingDistribution, stillPending := sd.pendingMap[preparedTxnDistributionID]
				if stillPending {
					log.L(ctx).Debugf("preparedTransactionDistributer:Loop retrying %s", preparedTxnDistributionID)
					sd.sendPreparedTransaction(ctx, pendingDistribution)
				}
				//if we didn't find it in the map, it was already acknowledged

			case preparedTxnDistribution := <-sd.inputChan:
				log.L(ctx).Debugf("preparedTransactionDistributer:Loop new distribution %s", preparedTxnDistribution.ID)

				sd.pendingMap[preparedTxnDistribution.ID] = preparedTxnDistribution
				sd.sendPreparedTransaction(ctx, preparedTxnDistribution)

			}
		}
	}()
	return nil
}

func (sd *preparedTransactionDistributer) Stop(ctx context.Context) {
	sd.stopRunCtx()
	sd.acknowledgementWriter.Stop()
	sd.receivedPreparedTransactionWriter.Stop()
}
