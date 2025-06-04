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
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProduceLatestInFlightStageContextSubmitPanic(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
	})

	// switch to submit
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031"))),
	})
	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	// unexpected error
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPanicOutput(ctx, InFlightTxStageSubmitting)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	// rolled back to signing stage as per current design
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)

}

func TestProduceLatestInFlightStageContextSubmitComplete(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
	})

	// switch to submit
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031")))
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: txHash,
	})

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)

	submissionTime := confutil.P(pldtypes.TimestampNow())
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeSubmittedNew, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	assert.Equal(t, txHash, rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.FirstSubmit)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)

	// submission attempt completed - nonce too low
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeNonceTooLow, ethclient.ErrorReason(""), nil)
	rsc.StageOutputsToBePersisted = nil
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	assert.Equal(t, submissionTime, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)
	assert.Equal(t, txHash, rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)
}

func TestProduceLatestInFlightStageContextCannotSubmit(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         pldtypes.Uint64ToUint256(32247127816),
			MaxPriorityFeePerGas: pldtypes.Uint64ToUint256(32146027800),
		},
	})

	// switch to submit
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	// Previous cost unknown when state is not validated
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)

	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         big.NewInt(0),
		PreviousNonceCostUnknown: true, // previous cost unknown, cannot submit
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "64494255632000", tOut.Cost.String())
	assert.False(t, tOut.TransactionSubmitted)

	// Previous cost unknown when state is validated
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
}

func TestProduceLatestInFlightStageContextSubmitCompleteAlreadyKnown(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
		FirstSubmit: confutil.P(pldtypes.TimestampNow()),
	})

	// switch to submit
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000031")))
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: txHash,
	})
	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)

	submissionTime := confutil.P(pldtypes.TimestampNow())
	// // submission attempt completed - already known
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Empty(t, rsc.StageOutputsToBePersisted.StatusUpdates)
	assert.Equal(t, txHash, rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)

	// submission attempt completed - already known for the first time submission
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil

	currentGeneration.InMemoryTxStateManager.(*inMemoryTxState).mtx.LastSubmit = nil
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Empty(t, rsc.StageOutputsToBePersisted.StatusUpdates)
	assert.Equal(t, txHash, rsc.StageOutputsToBePersisted.TxUpdates.TransactionHash)

	// submission attempt completed - already known for the an existing time submission
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		FirstSubmit:     confutil.P(pldtypes.TimestampNow()),
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("already known"))),
	})
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Empty(t, rsc.StageOutputsToBePersisted.StatusUpdates)
}

func TestProduceLatestInFlightStageContextSubmitErrors(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
		FirstSubmit: confutil.P(pldtypes.TimestampNow()),
	})

	// switch to submit
	txHash := confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001")))
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: txHash,
	})

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)

	submissionTime := confutil.P(pldtypes.TimestampNow())
	submissionErr := fmt.Errorf("submission error")

	// submission attempt errored - required re-preparation
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.False(t, tOut.TransactionSubmitted)
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	assert.Equal(t, submissionErr.Error(), *rsc.StageOutputsToBePersisted.TxUpdates.ErrorMessage)
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.Nil(t, rsc.StageOutputsToBePersisted.TxUpdates.NewSubmission)

	// submission attempt errored - required re-preparation during resubmission
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	newWarnTime := confutil.P(pldtypes.TimestampNow())
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		FirstSubmit:     confutil.P(pldtypes.TimestampNow()),
		TransactionHash: txHash,
	})
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx, nil, newWarnTime, SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
	tOut = it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "20000", tOut.Cost.String())
	assert.True(t, tOut.TransactionSubmitted)
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted)
	assert.Equal(t, 1, len(rsc.StageOutputsToBePersisted.StatusUpdates))
	_ = rsc.StageOutputsToBePersisted.StatusUpdates[0](mTS.statusUpdater)
	assert.Equal(t, submissionErr.Error(), *rsc.StageOutputsToBePersisted.TxUpdates.ErrorMessage)
	assert.Equal(t, InFlightTxStageSubmitting, rsc.Stage)
	assert.NotNil(t, rsc.StageOutputsToBePersisted.TxUpdates)
	assert.NotNil(t, rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)

	// persisting error waiting for persistence retry timeout
	assert.False(t, rsc.StageErrored)
	it.persistenceRetryTimeout = 5 * time.Second
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist signing sub-status error"))
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// persisted stage error - required more funds
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	rsc.StageOutput = &StageOutput{
		SubmitOutput: &SubmitOutputs{
			SubmissionOutcome: SubmissionOutcomeFailedRequiresRetry,
			ErrorReason:       string(ethclient.ErrorReasonInsufficientFunds),
			Err:               fmt.Errorf("insufficient funds"),
		},
	}
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), nil)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	rsc = it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	assert.True(t, rsc.StageErrored)

	// persisting error retrying
	it.persistenceRetryTimeout = 0
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), fmt.Errorf("persist submit error"))
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	it.persistenceRetryTimeout = 5 * time.Second

}

func TestProduceLatestInFlightStageContextSubmitRePrepare(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))),
	})

	// switch to submit
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	currentGeneration.SetTransientPreviousStageOutputs(&TransientPreviousStageOutputs{
		SignedMessage:   []byte("signedMessage"),
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))),
	})

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived)

	// persisted stage error - require re-preparation
	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.GetCurrentGeneration(ctx).AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), nil)
	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	rsc.StageOutput = &StageOutput{
		SubmitOutput: &SubmitOutputs{
			SubmissionOutcome: SubmissionOutcomeFailedRequiresRetry,
			ErrorReason:       string(ethclient.ErrorReasonInsufficientFunds),
			Err:               fmt.Errorf("insufficient funds"),
		},
	}
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.True(t, rsc.StageErrored)
	assert.Equal(t, InFlightTxStageSubmitting, currentGeneration.stage)
}

func TestProduceLatestInFlightStageContextResubmission(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	// the transaction already has details of a last submission
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))),
		LastSubmit:      confutil.P(pldtypes.TimestampNow()),
	})

	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	rsc := NewRunningStageContext(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, currentGeneration.InMemoryTxStateManager)
	currentGeneration.runningStageContext = rsc

	currentGeneration.bufferedStageOutputs = make([]*StageOutput, 0)
	// make sure this time is different
	newLastSubmit := pldtypes.TimestampFromUnix(time.Now().Add(5 * time.Second).Unix())
	it.stateManager.GetCurrentGeneration(ctx).AddSubmitOutput(ctx,
		confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))),
		&newLastSubmit,
		SubmissionOutcomeNonceTooLow,
		"",
		nil,
	)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	assert.Equal(t, newLastSubmit, *rsc.StageOutputsToBePersisted.TxUpdates.LastSubmit)
}

func TestTriggerSubmitTx(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.SubmissionRetry.MaxAttempts = confutil.P(1)
	})
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = false
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err pldtypes.RawJSON, actionOccurred *pldtypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		},
		TransactionHash: confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))),
	})

	// trigger signing
	assert.Nil(t, it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx))
	called := make(chan struct{})

	sendRawTransactionMock := m.ethClient.On("SendRawTransaction", ctx, mock.Anything)
	sendRawTransactionMock.Run(func(args mock.Arguments) {
		sendRawTransactionMock.Return(nil, fmt.Errorf("pop"))
		close(called)
	}).Once()
	err := it.TriggerSubmitTx(ctx, nil, confutil.P(pldtypes.Bytes32Keccak([]byte("0x000001"))))
	require.NoError(t, err)
	<-called
	currentGeneration := it.stateManager.GetCurrentGeneration(ctx).(*inFlightTransactionStateGeneration)
	for len(currentGeneration.bufferedStageOutputs) == 0 {
		// wait for event
	}
	assert.Len(t, currentGeneration.bufferedStageOutputs, 1)
	assert.NotNil(t, currentGeneration.bufferedStageOutputs[0].SubmitOutput)
	assert.NotNil(t, currentGeneration.bufferedStageOutputs[0].SubmitOutput.Err)
	assert.Empty(t, currentGeneration.bufferedStageOutputs[0].SubmitOutput.TxHash)
	assert.Empty(t, currentGeneration.bufferedStageOutputs[0].SubmitOutput.ErrorReason)
	assert.NotEmpty(t, currentGeneration.bufferedStageOutputs[0].SubmitOutput.SubmissionTime)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, currentGeneration.bufferedStageOutputs[0].SubmitOutput.SubmissionOutcome)
}
