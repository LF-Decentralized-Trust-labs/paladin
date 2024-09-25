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
	"time"

	"github.com/kaleido-io/paladin/core/internal/cache"
	"github.com/kaleido-io/paladin/core/internal/flushwriter"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type ManagerConfig struct {
	MaxInFlightOrchestrators *int                  `yaml:"maxInFlightOrchestrators"`
	Interval                 *string               `yaml:"interval"`
	OrchestratorIdleTimeout  *string               `yaml:"orchestratorIdleTimeout"`  // idle orchestrators exit after this time
	OrchestratorStaleTimeout *string               `yaml:"orchestratorStaleTimeout"` // stale orchestrators exit after this time - TODO: Define stale
	OrchestratorLifetime     *string               `yaml:"orchestratorLifetime"`     // orchestrators are cycled out after this time, regardless of activity
	ActivityRecords          ActivityRecordsConfig `yaml:"activityRecords"`
	SubmissionWriter         flushwriter.Config    `yaml:"submissionWriter"`
	Retry                    retry.Config          `yaml:"retry"`
}

var DefaultManagerConfig = &ManagerConfig{
	MaxInFlightOrchestrators: confutil.P(50),
	Interval:                 confutil.P("5s"),
	OrchestratorIdleTimeout:  confutil.P("1s"),
	OrchestratorStaleTimeout: confutil.P("5m"),
	OrchestratorLifetime:     confutil.P("10m"),
	Retry: retry.Config{
		InitialDelay: confutil.P("250ms"),
		MaxDelay:     confutil.P("30s"),
		Factor:       confutil.P(2.0),
	},
	SubmissionWriter: flushwriter.Config{
		WorkerCount:  confutil.P(5),
		BatchTimeout: confutil.P("75ms"),
		BatchMaxSize: confutil.P(50),
	},
	ActivityRecords: ActivityRecordsConfig{
		Config: cache.Config{
			// Status cache can be is shared across orchestrators, allowing status to live beyond TX completion
			// while still only being in memory
			Capacity: confutil.P(1000),
		},
		RecordsPerTransaction: confutil.P(25),
	},
}

type ActivityRecordsConfig struct {
	cache.Config
	RecordsPerTransaction *int `yaml:"entriesPerTransaction"`
}

// role of transaction engine:
// 1. owner and the only manipulator of the transaction orchestrators pool
//    - decides how many transaction orchestrators can there be in total at a given time
//    - decides the size of each transaction orchestrator queue
//    - decides signing address of each transaction orchestrator
//    - controls the life cycle for transaction orchestrators based on its latest status
//       - time box transaction orchestrator lifespan
//       - creates transaction orchestrator when necessary
//       - deletes transaction orchestrator when there is no pending transaction for a signing address
//       - pauses (delete) transaction orchestrator when it's in a stuck state
//       - resumes (re-create) transaction orchestrator in a resuming mode (1 transaction to test the water before add more) based on user configuration
// 2. handle external requests of transaction status change
//       - update/delete of a transaction when its signing address not in-flight
//       - signal update/delete of a transaction in-flight to corresponding transaction orchestrator
// 3. auto fueling management - the autofueling transactions
//    - creating auto-fueling transactions when asked by transaction orchestrators
// 4. provides shared functionalities for optimization
//    - handles gas price information which is not signer specific

func (ble *pubTxManager) engineLoop() {
	defer close(ble.engineLoopDone)
	ctx := log.WithLogField(ble.ctx, "role", "engine-loop")
	log.L(ctx).Infof("Engine started polling on interval %s", ble.enginePollingInterval)

	ticker := time.NewTicker(ble.enginePollingInterval)
	for {
		// Wait to be notified, or timeout to run
		select {
		case <-ticker.C:
		case <-ble.inFlightOrchestratorStale:
		case <-ctx.Done():
			ticker.Stop()
			log.L(ctx).Infof("Engine poller exiting")
			return
		}

		polled, total := ble.poll(ctx)
		log.L(ctx).Debugf("Engine polling complete: %d transaction orchestrators were created, there are %d transaction orchestrators in flight", polled, total)
	}
}

func (pte *pubTxManager) getOrchestratorCount() int {
	pte.inFlightOrchestratorMux.Lock()
	defer pte.inFlightOrchestratorMux.Unlock()
	return len(pte.inFlightOrchestrators)
}

func (ble *pubTxManager) poll(ctx context.Context) (polled int, total int) {
	pollStart := time.Now()
	ble.inFlightOrchestratorMux.Lock()
	defer ble.inFlightOrchestratorMux.Unlock()

	oldInFlight := ble.inFlightOrchestrators
	ble.inFlightOrchestrators = make(map[tktypes.EthAddress]*orchestrator)

	inFlightSigningAddresses := make([]tktypes.EthAddress, 0, len(oldInFlight))

	stateCounts := make(map[string]int)
	for _, sName := range AllOrchestratorStates {
		// map for saving number of in flight transaction per stage
		stateCounts[sName] = 0
	}

	// Run through copying across from the old InFlight list to the new one, those that aren't ready to be deleted
	for signingAddress, oc := range oldInFlight {
		log.L(ctx).Debugf("Engine checking orchestrator for %s: state: %s, state duration: %s, number of transactions: %d", oc.signingAddress, oc.state, time.Since(oc.stateEntryTime), len(oc.InFlightTxs))
		if oc.state == OrchestratorStateIdle && time.Since(oc.stateEntryTime) > ble.orchestratorIdleTimeout ||
			oc.state == OrchestratorStateStale && time.Since(oc.stateEntryTime) > ble.orchestratorStaleTimeout {
			// tell transaction orchestrator to stop, there is a chance we later found new transaction for this address, but we got to make a call at some point
			// so it's here. The transaction orchestrator won't be removed immediately as the state update is async
			oc.Stop()
		}
		if oc.state != OrchestratorStateStopped {
			ble.inFlightOrchestrators[signingAddress] = oc
			oc.MarkInFlightTxStale()
			stateCounts[string(oc.state)] = stateCounts[string(oc.state)] + 1
			inFlightSigningAddresses = append(inFlightSigningAddresses, signingAddress)
		} else {
			log.L(ctx).Infof("Engine removed orchestrator for signing address %s", signingAddress)
		}
	}

	totalBeforePoll := len(ble.inFlightOrchestrators)
	// check and poll new signers from the persistence if there are more transaction orchestrators slots
	spaces := ble.maxInflight - totalBeforePoll
	if spaces > 0 {

		// Run through the paused orchestrators for fairness control
		for signingAddress, pausedUntil := range ble.signingAddressesPausedUntil {
			if time.Now().Before(pausedUntil) {
				log.L(ctx).Debugf("Engine excluded orchestrator for signing address %s from polling as it's paused util %s", signingAddress, pausedUntil.String())
				stateCounts[string(OrchestratorStatePaused)] = stateCounts[string(OrchestratorStatePaused)] + 1
				inFlightSigningAddresses = append(inFlightSigningAddresses, signingAddress)
			}
		}

		var additionalNonInFlightSigners []*txFromOnly
		// We retry the get from persistence indefinitely (until the context cancels)
		err := ble.retry.Do(ctx, func(attempt int) (retry bool, err error) {
			// TODO: Fairness algorithm for swapping out orchestrators when there is no space
			// (raw SQL as couldn't convince gORM to build this)

			const dbQueryBase = `SELECT DISTINCT t."from" FROM "public_txns" AS t ` +
				`LEFT JOIN "public_completions" AS c ON t."signer_nonce" = c."signer_nonce" ` +
				`WHERE c."signer_nonce" IS NULL`

			const dbQueryNothingInFlight = dbQueryBase + ` LIMIT ?`
			if len(inFlightSigningAddresses) == 0 {
				return true, ble.p.DB().Raw(dbQueryNothingInFlight, spaces).Scan(&additionalNonInFlightSigners).Error
			}

			const dbQueryInFlight = dbQueryBase + ` AND t."from" NOT IN (?) LIMIT ?`
			return true, ble.p.DB().Raw(dbQueryInFlight, inFlightSigningAddresses, spaces).Scan(&additionalNonInFlightSigners).Error
		})
		if err != nil {
			log.L(ctx).Infof("Engine polling context cancelled while retrying")
			return -1, len(ble.inFlightOrchestrators)
		}

		log.L(ctx).Debugf("Engine polled %d items to fill in %d empty slots.", len(additionalNonInFlightSigners), spaces)

		for _, r := range additionalNonInFlightSigners {
			if _, exist := ble.inFlightOrchestrators[r.From]; !exist {
				oc := NewOrchestrator(ble, r.From, ble.conf)
				ble.inFlightOrchestrators[r.From] = oc
				stateCounts[string(oc.state)] = stateCounts[string(oc.state)] + 1
				_, _ = oc.Start(ble.ctx)
				log.L(ctx).Infof("Engine added orchestrator for signing address %s", r.From)
			} else {
				log.L(ctx).Warnf("Engine fetched extra transactions from signing address %s", r.From)
			}
		}
		total = len(ble.inFlightOrchestrators)
		if total > 0 {
			polled = total - totalBeforePoll
		}
	} else {
		// the in-flight orchestrator pool is full, do the fairness control

		// TODO: don't stop more than required number of slots

		// Run through the existing running orchestrators and stop the ones that exceeded the max process timeout
		for signingAddress, oc := range ble.inFlightOrchestrators {
			if time.Since(oc.orchestratorBirthTime) > ble.orchestratorLifetime {
				log.L(ctx).Infof("Engine pause, attempt to stop orchestrator for signing address %s", signingAddress)
				oc.Stop()
				ble.signingAddressesPausedUntil[signingAddress] = time.Now().Add(ble.orchestratorLifetime)
			}
		}
	}
	ble.thMetrics.RecordInFlightOrchestratorPoolMetrics(ctx, stateCounts, ble.maxInflight-len(ble.inFlightOrchestrators))
	log.L(ctx).Debugf("Engine poll loop took %s", time.Since(pollStart))
	return polled, total
}

func (ble *pubTxManager) MarkInFlightOrchestratorsStale() {
	// try to send an item in `InFlightStale` channel, which has a buffer of 1
	// to trigger a polling event to update the in flight transaction orchestrators
	// if it already has an item in the channel, this function does nothing
	select {
	case ble.inFlightOrchestratorStale <- true:
	default:
	}
}
