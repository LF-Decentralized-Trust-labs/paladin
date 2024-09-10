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
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/internal/transactionstore"
	"github.com/stretchr/testify/assert"
)

func TestTransactionProcessor(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}
	tp := NewPaladinTransactionProcessor(ctx, testTx, newTestStageController(ctx)).(*PaladinTxProcessor)
	tp.stageController = newTestStageController(ctx)
	assert.Nil(t, tp.GetStageContext(ctx))
	assert.Nil(t, tp.GetStageTriggerError(ctx))
}

func TestTransactionProcessorPersistTxUpdates(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}
	tp := NewPaladinTransactionProcessor(ctx, testTx, newTestStageController(ctx)).(*PaladinTxProcessor)
	tp.stageController = newTestStageController(ctx)
	assert.Nil(t, tp.GetStageContext(ctx))
	assert.Nil(t, tp.GetStageTriggerError(ctx))
	assert.Empty(t, testTx.SequenceID)

	tp.Init(ctx)
	assert.NotEmpty(t, tp.stageContext)

	tp.AddStageEvent(ctx, &enginespi.StageEvent{
		Stage: testStage,
		Data: &testActionOutput{
			Message: "continue",
		},
	})
	firstSeqID := testTx.SequenceID
	assert.NotEmpty(t, testTx.SequenceID)

	testTx.Contract = "complete"
	tp.AddStageEvent(ctx, &enginespi.StageEvent{
		Stage: testStage,
		Data: &testActionOutput{
			Message: "continue",
		},
	})
	assert.NotEqual(t, firstSeqID, testTx.SequenceID)
}

func TestTransactionProcessorInitiateOnEvent(t *testing.T) {
	ctx := context.Background()
	newTxID := uuid.New()
	testTx := &transactionstore.TransactionWrapper{
		Transaction: transactionstore.Transaction{
			ID: newTxID,
		},
		PrivateTransaction: &components.PrivateTransaction{
			ID: newTxID,
		},
	}
	tp := NewPaladinTransactionProcessor(ctx, testTx, newTestStageController(ctx)).(*PaladinTxProcessor)
	tp.stageController = newTestStageController(ctx)
	assert.Nil(t, tp.GetStageContext(ctx))
	assert.Nil(t, tp.GetStageTriggerError(ctx))
	assert.Empty(t, testTx.SequenceID)

	assert.Empty(t, tp.stageContext)

	tp.AddStageEvent(ctx, &enginespi.StageEvent{
		Stage: testStage,
		Data: &testActionOutput{
			Message: "continue",
		},
	})

	assert.NotEmpty(t, testTx.SequenceID)
}