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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

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
// 3. provides shared functionalities for optimization
//    - handles gas price information which is not signer specific

func (ptm *pubTxManager) engineLoop() {
	defer close(ptm.engineLoopDone)
	ctx := log.WithLogField(ptm.ctx, "role", "engine-loop")
	log.L(ctx).Infof("Engine started polling on interval %s", ptm.enginePollingInterval)

	ticker := time.NewTicker(ptm.enginePollingInterval)
	for {
		// Wait to be notified, or timeout to run
		select {
		case <-ticker.C:
		case <-ptm.inFlightOrchestratorStale:
		case <-ctx.Done():
			ticker.Stop()
			log.L(ctx).Infof("Engine poller exiting")
			return
		}

		ptm.handleUpdates()
		polled, total := ptm.poll(ctx)
		log.L(ctx).Debugf("Engine polling complete: %d transaction orchestrators were created, there are %d transaction orchestrators in flight", polled, total)
	}
}

//nolint:unused // May be used in future
func (ptm *pubTxManager) getOrchestratorCount() int {
	ptm.inFlightOrchestratorMux.Lock()
	defer ptm.inFlightOrchestratorMux.Unlock()
	return len(ptm.inFlightOrchestrators)
}

func (ptm *pubTxManager) getOrchestratorForAddress(signer pldtypes.EthAddress) *orchestrator {
	ptm.inFlightOrchestratorMux.Lock()
	defer ptm.inFlightOrchestratorMux.Unlock()
	return ptm.inFlightOrchestrators[signer]
}

func (ptm *pubTxManager) flushStaleOrchestratorsGetCount(ctx context.Context) (inFlightSigningAddresses []pldtypes.EthAddress, stateCounts map[string]int, totalAfterFlush int) {
	ptm.inFlightOrchestratorMux.Lock()
	defer ptm.inFlightOrchestratorMux.Unlock()

	oldInFlight := ptm.inFlightOrchestrators
	ptm.inFlightOrchestrators = make(map[pldtypes.EthAddress]*orchestrator)
	inFlightSigningAddresses = make([]pldtypes.EthAddress, 0, len(oldInFlight))

	stateCounts = make(map[string]int)
	for _, sName := range AllOrchestratorStates {
		// map for saving number of in flight transaction per stage
		stateCounts[sName] = 0
	}

	// Run through copying across from the old InFlight list to the new one, those that aren't ready to be deleted
	for signingAddress, oc := range oldInFlight {
		log.L(ctx).Debugf("Engine checking orchestrator for %s: state: %s, state duration: %s, number of transactions: %d", oc.signingAddress, oc.state, time.Since(oc.stateEntryTime), len(oc.inFlightTxs))
		if oc.state == OrchestratorStateIdle && time.Since(oc.stateEntryTime) > ptm.orchestratorIdleTimeout ||
			oc.state == OrchestratorStateStale && time.Since(oc.stateEntryTime) > ptm.orchestratorStaleTimeout {
			// tell transaction orchestrator to stop, there is a chance we later found new transaction for this address, but we got to make a call at some point
			// so it's here. The transaction orchestrator won't be removed immediately as the state update is async
			oc.Stop()
		}
		if oc.state != OrchestratorStateStopped {
			ptm.inFlightOrchestrators[signingAddress] = oc
			oc.MarkInFlightTxStale()
			stateCounts[string(oc.state)] = stateCounts[string(oc.state)] + 1
			inFlightSigningAddresses = append(inFlightSigningAddresses, signingAddress)
		} else {
			log.L(ctx).Infof("Engine removed orchestrator for signing address %s", signingAddress)
		}
	}

	totalAfterFlush = len(ptm.inFlightOrchestrators)
	return inFlightSigningAddresses, stateCounts, totalAfterFlush
}

func (ptm *pubTxManager) poll(ctx context.Context) (polled int, total int) {
	pollStart := time.Now()

	// Perform locked processing to determine if there are spaces to fill
	inFlightSigningAddresses, stateCounts, totalBeforePoll := ptm.flushStaleOrchestratorsGetCount(ctx)

	// check and poll new signers from the persistence if there are more transaction orchestrators slots
	spaces := ptm.maxInflight - totalBeforePoll
	if spaces > 0 {

		// Run through the paused orchestrators for fairness control
		// Note not controlled by mutex, as only modified on this routine.
		for signingAddress, pausedUntil := range ptm.signingAddressesPausedUntil {
			if time.Now().Before(pausedUntil) {
				log.L(ctx).Debugf("Engine excluded orchestrator for signing address %s from polling as it's paused util %s", signingAddress, pausedUntil.String())
				stateCounts[string(OrchestratorStatePaused)] = stateCounts[string(OrchestratorStatePaused)] + 1
				inFlightSigningAddresses = append(inFlightSigningAddresses, signingAddress)
			}
		}

		var additionalNonInFlightSigners []*txFromOnly
		// We retry the get from persistence indefinitely (until the context cancels)
		err := ptm.retry.Do(ctx, func(attempt int) (retry bool, err error) {
			// (raw SQL as couldn't convince gORM to build this)
			const dbQueryBase = `SELECT DISTINCT t."from" FROM "public_txns" AS t ` +
				`LEFT JOIN "public_completions" AS c ON t."pub_txn_id" = c."pub_txn_id" ` +
				`WHERE c."pub_txn_id" IS NULL AND "suspended" IS FALSE`

			const dbQueryNothingInFlight = dbQueryBase + ` LIMIT ?`
			if len(inFlightSigningAddresses) == 0 {
				return true, ptm.p.DB().Raw(dbQueryNothingInFlight, spaces).Scan(&additionalNonInFlightSigners).Error
			}

			const dbQueryInFlight = dbQueryBase + ` AND t."from" NOT IN (?) LIMIT ?`
			return true, ptm.p.DB().Raw(dbQueryInFlight, inFlightSigningAddresses, spaces).Scan(&additionalNonInFlightSigners).Error
		})
		if err != nil {
			log.L(ctx).Infof("Engine polling context cancelled while retrying")
			return -1, totalBeforePoll
		}

		log.L(ctx).Debugf("Engine polled %d items to fill in %d empty slots.", len(additionalNonInFlightSigners), spaces)

		// (Re)obtain the lock to add the additional ones
		ptm.inFlightOrchestratorMux.Lock()
		defer ptm.inFlightOrchestratorMux.Unlock()

		for _, r := range additionalNonInFlightSigners {
			if _, exist := ptm.inFlightOrchestrators[r.From]; !exist {
				oc := NewOrchestrator(ptm, r.From, ptm.conf)
				ptm.inFlightOrchestrators[r.From] = oc
				stateCounts[string(oc.state)] = stateCounts[string(oc.state)] + 1
				_, _ = oc.Start(ptm.ctx)
				log.L(ctx).Infof("Engine added orchestrator for signing address %s", r.From)
			}
		}
		total = len(ptm.inFlightOrchestrators)
		if total > 0 {
			polled = total - totalBeforePoll
		}
	} else {

		// (Re)obtain the lock to do fairness control
		ptm.inFlightOrchestratorMux.Lock()
		defer ptm.inFlightOrchestratorMux.Unlock()

		// the in-flight orchestrator pool is full, do the fairness control

		// TODO: don't stop more than required number of slots

		// Run through the existing running orchestrators and stop the ones that exceeded the max process timeout
		for signingAddress, oc := range ptm.inFlightOrchestrators {
			if time.Since(oc.orchestratorBirthTime) > ptm.orchestratorSwapTimeout {
				log.L(ctx).Infof("Engine pause, attempt to stop orchestrator for signing address %s", signingAddress)
				oc.Stop()
				ptm.signingAddressesPausedUntil[signingAddress] = time.Now().Add(ptm.orchestratorSwapTimeout)
			}
		}
	}
	ptm.thMetrics.RecordInFlightOrchestratorPoolMetrics(ctx, stateCounts, ptm.maxInflight-len(ptm.inFlightOrchestrators))
	log.L(ctx).Debugf("Engine poll loop took %s", time.Since(pollStart))
	return polled, total
}

func (ptm *pubTxManager) handleUpdates() {
	ptm.updateMux.Lock()
	updates := ptm.updates
	ptm.updates = nil
	ptm.updateMux.Unlock()

	ptm.inFlightOrchestratorMux.Lock()
	defer ptm.inFlightOrchestratorMux.Unlock()

	for _, update := range updates {
		inFlightOrchestrator, orchestratorInFlight := ptm.inFlightOrchestrators[*update.from]
		if orchestratorInFlight {
			inFlightOrchestrator.dispatchUpdate(update)
		}
	}
}

func (ptm *pubTxManager) MarkInFlightOrchestratorsStale() {
	// try to send an item in `InFlightStale` channel, which has a buffer of 1
	// to trigger a polling event to update the in flight transaction orchestrators
	// if it already has an item in the channel, this function does nothing
	select {
	case ptm.inFlightOrchestratorStale <- true:
	default:
	}
}
