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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
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
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)

	// unexpected error
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPanicOutput(ctx, InFlightTxStageSubmitting)
	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: true,
	})
	assert.NotEmpty(t, *tOut)
	assert.Regexp(t, "PD011919", tOut.Error)
	assert.NotEqual(t, rsc, it.stateManager.GetRunningStageContext(ctx))
	// rolled back to signing stage as per current design
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)

}

func TestProduceLatestInFlightStageContextSubmitComplete(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := confutil.P(tktypes.Bytes32Keccak([]byte("0x000031")))
	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

	submissionTime := confutil.P(tktypes.TimestampNow())
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeSubmittedNew, ethclient.ErrorReason(""), nil)
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
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc = it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeNonceTooLow, ethclient.ErrorReason(""), nil)
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
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			MaxFeePerGas:         tktypes.Uint64ToUint256(32247127816),
			MaxPriorityFeePerGas: tktypes.Uint64ToUint256(32146027800),
		},
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	// Previous cost unknown when state is not validated
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

	tOut := it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         big.NewInt(0),
		PreviousNonceCostUnknown: true, // previous cost unknown, cannot submit
	})
	assert.NotEmpty(t, *tOut)
	assert.Equal(t, "64494255632000", tOut.Cost.String())
	assert.False(t, tOut.TransactionSubmitted)

	// Previous cost unknown when state is validated
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
}

func TestProduceLatestInFlightStageContextSubmitCompleteAlreadyKnown(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
		FirstSubmit: confutil.P(tktypes.TimestampNow()),
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := confutil.P(tktypes.Bytes32Keccak([]byte("0x000031")))
	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)
	// submission attempt completed - new transaction submitted
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)

	submissionTime := confutil.P(tktypes.TimestampNow())
	// // submission attempt completed - already known
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)

	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
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
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil

	inFlightStageMananger.InMemoryTxStateManager.(*inMemoryTxState).mtx.LastSubmit = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
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
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		FirstSubmit:     confutil.P(tktypes.TimestampNow()),
		TransactionHash: confutil.P(tktypes.Bytes32Keccak([]byte("already known"))),
	})
	rsc = it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeAlreadyKnown, ethclient.ErrorReason(""), nil)
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
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
		FirstSubmit: confutil.P(tktypes.TimestampNow()),
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")
	txHash := confutil.P(tktypes.Bytes32Keccak([]byte("0x000001")))

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, signedMsg)
	rsc := it.stateManager.GetRunningStageContext(ctx)

	submissionTime := confutil.P(tktypes.TimestampNow())
	submissionErr := fmt.Errorf("submission error")

	// submission attempt errored - required re-preparation
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)
	it.stateManager.AddSubmitOutput(ctx, txHash, submissionTime, SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
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
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	rsc.StageOutputsToBePersisted = nil
	rsc = it.stateManager.GetRunningStageContext(ctx)
	newWarnTime := confutil.P(tktypes.TimestampNow())
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		FirstSubmit:     confutil.P(tktypes.TimestampNow()),
		TransactionHash: txHash,
	})
	it.stateManager.AddSubmitOutput(ctx, nil, newWarnTime, SubmissionOutcomeFailedRequiresRetry, ethclient.ErrorReasonTransactionReverted, submissionErr)
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
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now().Add(it.persistenceRetryTimeout*2), fmt.Errorf("persist signing sub-status error"))
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})

	// persisted stage error - required more funds
	rsc = it.stateManager.GetRunningStageContext(ctx)
	rsc.StageOutput = &StageOutput{
		SubmitOutput: &SubmitOutputs{
			SubmissionOutcome: SubmissionOutcomeFailedRequiresRetry,
			ErrorReason:       string(ethclient.ErrorReasonInsufficientFunds),
			Err:               fmt.Errorf("insufficient funds"),
		},
	}
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), nil)
	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{
		AvailableToSpend:         nil,
		PreviousNonceCostUnknown: false,
	})
	rsc = it.stateManager.GetRunningStageContext(ctx)
	assert.True(t, rsc.StageErrored)

	// persisting error retrying
	it.persistenceRetryTimeout = 0
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), fmt.Errorf("persist submit error"))
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
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
		TransactionHash: confutil.P(tktypes.Bytes32Keccak([]byte("0x000001"))),
	})

	// switch to submit
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	signedMsg := []byte("signedMessage")

	it.TriggerNewStageRun(ctx, InFlightTxStageSubmitting, BaseTxSubStatusReceived, signedMsg)

	// persisted stage error - require re-preparation
	inFlightStageMananger.bufferedStageOutputs = make([]*StageOutput, 0)
	it.stateManager.AddPersistenceOutput(ctx, InFlightTxStageSubmitting, time.Now(), nil)
	rsc := it.stateManager.GetRunningStageContext(ctx)
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
	assert.Equal(t, InFlightTxStageSubmitting, inFlightStageMananger.stage)
}

func TestProduceLatestInFlightStageContextTriggerSubmit(t *testing.T) {
	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.SubmissionRetry.MaxAttempts = confutil.P(1)
	})
	defer done()
	it, mTS := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = false
	it.testOnlyNoEventMode = false
	mTS.statusUpdater = &mockStatusUpdater{
		updateSubStatus: func(ctx context.Context, imtx InMemoryTxStateReadOnly, subStatus BaseTxSubStatus, action BaseTxAction, info, err *fftypes.JSONAny, actionOccurred *tktypes.Timestamp) error {
			return nil
		},
	}
	mTS.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &pldapi.PublicTxGasPricing{
			GasPrice: tktypes.Uint64ToUint256(10),
		},
		TransactionHash: confutil.P(tktypes.Bytes32Keccak([]byte("0x000001"))),
	})

	// trigger signing
	assert.Nil(t, it.stateManager.GetRunningStageContext(ctx))
	called := make(chan struct{})

	sendRawTransactionMock := m.ethClient.On("SendRawTransaction", ctx, mock.Anything)
	sendRawTransactionMock.Run(func(args mock.Arguments) {
		sendRawTransactionMock.Return(nil, fmt.Errorf("pop"))
		close(called)
	}).Once()
	err := it.TriggerSubmitTx(ctx, nil)
	require.NoError(t, err)
	<-called
	inFlightStageMananger := it.stateManager.(*inFlightTransactionState)
	for len(inFlightStageMananger.bufferedStageOutputs) == 0 {
		// wait for event
	}
	assert.Len(t, inFlightStageMananger.bufferedStageOutputs, 1)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput)
	assert.NotNil(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.Err)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.TxHash)
	assert.Empty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.ErrorReason)
	assert.NotEmpty(t, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.SubmissionTime)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, inFlightStageMananger.bufferedStageOutputs[0].SubmitOutput.SubmissionOutcome)
}
