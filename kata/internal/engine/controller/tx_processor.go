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

package controller

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type stageContextAction int

const (
	resumeStage = iota
	initStage
	switchStage
)

type TxProcessor interface {
	GetStageContext(ctx context.Context) *StageContext
	GetStageTriggerError(ctx context.Context) error

	// stage outputs management
	AddStageEvent(ctx context.Context, stageEvent *types.StageEvent)
	Init(ctx context.Context)
}

type StageContext struct {
	ctx            context.Context
	ID             string
	Stage          string
	stageEntryTime time.Time
}

func NewPaladinTransactionProcessor(ctx context.Context, tsm transactionstore.TxStateManager, sc StageController) TxProcessor {
	return &PaladinTxProcessor{
		tsm:                 tsm,
		stageController:     sc,
		stageErrorRetry:     10 * time.Second,
		bufferedStageEvents: make([]*types.StageEvent, 0),
	}
}

type PaladinTxProcessor struct {
	stageContextMutex sync.Mutex
	stageContext      *StageContext
	stageTriggerError error
	stageErrorRetry   time.Duration
	tsm               transactionstore.TxStateManager

	stageController StageController

	bufferedStageEventsMapMutex sync.Mutex
	bufferedStageEvents         []*types.StageEvent
}

func (ts *PaladinTxProcessor) Init(ctx context.Context) {
	ts.createStageContext(ctx, initStage)
}

func (ts *PaladinTxProcessor) createStageContext(ctx context.Context, action stageContextAction) {
	ts.stageContextMutex.Lock()
	defer ts.stageContextMutex.Unlock()
	if action != switchStage && ts.stageContext != nil { // we only override existing stage context when switching stage
		// stage context already initialized, skip
		log.L(ctx).Tracef("Transaction with ID %s, on stage %s, no need for new stage context", ts.tsm.GetTxID(ctx), ts.stageContext.Stage)
		return
	}
	nowTime := time.Now() // pin the now time
	stage := ts.stageController.CalculateStage(ctx, ts.tsm)
	nextStepContext := &StageContext{
		Stage:          stage,
		ID:             uuid.NewString(),
		stageEntryTime: nowTime,
		ctx:            log.WithLogField(ctx, "stage", string(stage)),
	}
	if ts.stageContext != nil {
		if ts.stageContext.Stage == nextStepContext.Stage { // switching to existing stage ---> this is a retry
			// redoing the current stage
			log.L(ctx).Warnf("Transaction with ID %s retrying action, already on stage %s for %s", ts.tsm.GetTxID(ctx), stage, time.Since(ts.stageContext.stageEntryTime))
			nextStepContext.stageEntryTime = ts.stageContext.stageEntryTime
		} else {
			log.L(ctx).Tracef("Transaction with ID %s, switching from %s to %s after %s", ts.tsm.GetTxID(ctx), ts.stageContext.Stage, nextStepContext.Stage, time.Since(ts.stageContext.stageEntryTime))
		}
	} else {
		// init succeeded
		log.L(ctx).Tracef("Transaction with ID %s, initiated on stage %s", ts.tsm.GetTxID(ctx), nextStepContext.Stage)
	}
	ts.stageContext = nextStepContext
	ts.stageTriggerError = nil
	if action != resumeStage { // if we are resuming a stage when received its action event, don't perf the action again
		ts.PerformActionForStageAsync(ctx)
	} else {
		log.L(ctx).Tracef("Transaction with ID %s, resuming for %s stage", ts.tsm.GetTxID(ctx), stage)
	}

}

func (ts *PaladinTxProcessor) PerformActionForStageAsync(ctx context.Context) {
	stageContext := ts.stageContext
	if stageContext == nil {
		panic("stage context not set")
	}
	log.L(ctx).Tracef("Transaction with ID %s, triggering action for %s stage", ts.tsm.GetTxID(ctx), stageContext.Stage)
	ts.executeAsync(func() {
		synchronousActionOutput, err := ts.stageController.PerformActionForStage(ctx, string(stageContext.Stage), ts.tsm)
		ts.stageTriggerError = err
		if synchronousActionOutput != nil {
			ts.AddStageEvent(ts.stageContext.ctx, &types.StageEvent{
				ID:    stageContext.ID,
				TxID:  ts.tsm.GetTxID(ctx),
				Stage: stageContext.Stage,
				Data:  synchronousActionOutput,
			})
		}
		if err != nil {
			// if errored, clean stage context
			ts.stageContextMutex.Lock()
			ts.stageContext = nil
			ts.stageContextMutex.Unlock()
			// retry after the timeout
			time.Sleep(ts.stageErrorRetry)
			ts.createStageContext(ctx, initStage)
		}
	}, ctx)
}

func (ts *PaladinTxProcessor) addPanicOutput(ctx context.Context, sc StageContext) {
	start := time.Now()
	// unexpected error, set an empty input for the stage
	// so that the stage handler will handle this as unexpected error
	ts.AddStageEvent(ctx, &types.StageEvent{
		Stage: sc.Stage,
		ID:    sc.ID,
		TxID:  ts.tsm.GetTxID(ctx),
	})
	log.L(ctx).Debugf("%s addPanicOutput took %s to write the result", ts.tsm.GetTxID(ctx), time.Since(start))
}

func (ts *PaladinTxProcessor) executeAsync(funcToExecute func(), ctx context.Context) {
	sc := *ts.stageContext
	go func() {
		defer func() {
			if err := recover(); err != nil {
				// if the function panicked, catch it and write a panic error to the output queue
				log.L(ctx).Errorf("Panic error detected for transaction %s, when executing: %s, error: %+v", ts.tsm.GetTxID(ctx), sc.Stage, err)
				ts.addPanicOutput(ctx, sc)
			}
		}()
		funcToExecute() // in non-panic scenarios, this function will add output to the output queue
	}()
}

func (ts *PaladinTxProcessor) GetStageContext(ctx context.Context) *StageContext {
	return ts.stageContext
}

func (ts *PaladinTxProcessor) GetStageTriggerError(ctx context.Context) error {
	return ts.stageTriggerError
}

func (ts *PaladinTxProcessor) AddStageEvent(ctx context.Context, stageEvent *types.StageEvent) {
	ts.bufferedStageEventsMapMutex.Lock()
	defer ts.bufferedStageEventsMapMutex.Unlock()
	ts.bufferedStageEvents = append(ts.bufferedStageEvents, stageEvent)

	ts.createStageContext(ctx, resumeStage)

	unProcessedBufferedStageEvents, txUpdates, nextStep := ts.stageController.ProcessEventsForStage(ctx, string(ts.stageContext.Stage), ts.tsm, ts.bufferedStageEvents)

	if unProcessedBufferedStageEvents != nil {
		ts.bufferedStageEvents = unProcessedBufferedStageEvents
	}
	if txUpdates != nil {
		// persistence is synchronous, so it must NOT run on the main go routine to avoid blocking
		ts.tsm.ApplyTxUpdates(ctx, txUpdates)
	}
	if nextStep == types.NextStepNewStage {
		ts.createStageContext(ctx, switchStage)
	} else if nextStep == types.NextStepNewAction {
		ts.PerformActionForStageAsync(ctx)
	}
	// other wise, the stage told the processor to wait for async events
}
