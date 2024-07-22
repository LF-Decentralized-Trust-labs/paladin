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

package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/stretchr/testify/assert"
)

const testStage = "test"

type testActionOutput struct {
	Message string
}

type testStageProcessor struct {
}

func (tsp *testStageProcessor) Name() string {
	return "test"
}

func (tsp *testStageProcessor) GetIncompletePreReqTxIDs(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) *types.TxProcessPreReq {
	return nil
}

func (tsp *testStageProcessor) MatchStage(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) bool {
	return true
}

func (tsp *testStageProcessor) ProcessEvents(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService, stageEvents []*types.StageEvent) (unprocessedStageEvents []*types.StageEvent, txUpdates *transactionstore.TransactionUpdate, nextStep types.StageProcessNextStep) {
	unprocessedStageEvents = []*types.StageEvent{}
	nextStep = types.NextStepWait
	for _, se := range stageEvents {
		if string(se.Stage) == testStage {
			// pretend we processed it
			if se.Data.(*testActionOutput).Message == "complete" {
				nextStep = types.NextStepNewStage
			} else {
				txUpdates = &transactionstore.TransactionUpdate{
					SequenceID: confutil.P(uuid.New()),
				}
			}
		} else {
			unprocessedStageEvents = append(unprocessedStageEvents, se)
		}
	}
	return
}
func (tsp *testStageProcessor) PerformAction(ctx context.Context, tsg transactionstore.TxStateGetters, sfs types.StageFoundationService) (actionOutput interface{}, actionErr error) {
	if tsg.GetContract(ctx) == "error" {
		return nil, errors.New("pop")
	} else if tsg.GetContract(ctx) == "complete" {
		return &testActionOutput{
			Message: "complete",
		}, nil
	} else {
		return &testActionOutput{
			Message: "continue",
		}, nil
	}
}

func newTestStageController(ctx context.Context) *PaladinStageController {
	sc := NewPaladinStageController(ctx, types.NewPaladinStageFoundationService(nil, nil, nil)).(*PaladinStageController)

	sc.stageProcessors = map[string]TxStageProcessor{
		testStage: &testStageProcessor{},
	}
	return sc
}

func TestBasicStageController(t *testing.T) {
	ctx := context.Background()
	sc := newTestStageController(ctx)
	testTx := &transactionstore.Transaction{
		ID:       uuid.New(),
		Contract: "complete",
	}
	// TODO: replace dummy checks with real implementation
	// test function works with test processor
	s := sc.CalculateStage(ctx, testTx)
	assert.Equal(t, "test", string(s))

	output, err := sc.PerformActionForStage(ctx, testStage, testTx)
	assert.Equal(t, "complete", output.(*testActionOutput).Message)
	assert.Empty(t, err)

	events, txUpdates, completed := sc.ProcessEventsForStage(ctx, testStage, testTx, []*types.StageEvent{
		{
			Stage: testStage,
			Data:  output,
		},
	})
	assert.Empty(t, events)
	assert.Empty(t, txUpdates)
	assert.Equal(t, types.NextStepNewStage, completed)

	// panic when stage is unknown
	unknownStage := "unknown"

	assert.Panics(t, func() {
		_, _ = sc.PerformActionForStage(ctx, unknownStage, testTx)
	})

	assert.Panics(t, func() {
		_, _, _ = sc.ProcessEventsForStage(ctx, unknownStage, testTx, []*types.StageEvent{})
	})
}
