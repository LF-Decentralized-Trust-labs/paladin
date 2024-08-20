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

package stages

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/internal/transactionstore"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDispatchStageMatch(t *testing.T) {
	ctx := context.Background()
	txNodeID := "current_node_id"

	preReqTx := &transactionstore.Transaction{
		ID: uuid.New(),
	}

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())

	testTx := &transactionstore.Transaction{
		ID:           uuid.New(),
		DispatchNode: txNodeID,
		PreReqTxs:    []string{preReqTx.ID.String()},
	}
	mSFS := &enginemocks.StageFoundationService{}
	mIR := &enginemocks.IdentityResolver{}
	mSFS.On("IdentityResolver").Return(mIR)
	mIR.On("IsCurrentNode", txNodeID).Return(true)

	// expect transaction to not match dispatch stage, when tx payload is not prepared.
	assert.False(t, ds.MatchStage(ctx, testTx, mSFS))

	// expect transaction to match dispatch stage when tx payload is ready
	testTx.DispatchTxPayload = "payload"
	assert.True(t, ds.MatchStage(ctx, testTx, mSFS))

	// expect transaction to not match dispatch stage when there is a tx submitted already
	testTx.DispatchTxID = uuid.NewString()
	assert.False(t, ds.MatchStage(ctx, testTx, mSFS))
	testTx.DispatchTxID = ""

	//
	mDC := &enginemocks.DependencyChecker{}
	mSFS.On("DependencyChecker").Return(mDC)
	mDC.On("PreReqsMatchCondition", ctx, nil, mock.Anything).Run(func(args mock.Arguments) {
		checkFn := args[3].(func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool))
		assert.False(t, checkFn(preReqTx))
	}).Return(false)

}

func TestDispatchStagePreReqCheck(t *testing.T) {
	ctx := context.Background()
	txNodeID := "current_node_id"
	txSigningAddress := "0xb60e8dd61c5d32be8058bb8eb970870f07233155"
	txDifferentSigningAddr := "0xCfcEcEFf128aE953a272A05Ea43969c9E5ba87dB"
	testTxHash := "0x5c7f2a3d5e77b95e0dbb8d2b6b9b58ec19356f7096e50904b36baac6d0f11a89"

	preReqTx := &transactionstore.Transaction{
		ID:           uuid.New(),
		DispatchNode: txNodeID,
	}

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())

	testTx := &transactionstore.Transaction{
		ID:                uuid.New(),
		DispatchNode:      txNodeID,
		PreReqTxs:         []string{preReqTx.ID.String()},
		DispatchTxPayload: "payload",
	}
	mSFS := &enginemocks.StageFoundationService{}
	mIR := &enginemocks.IdentityResolver{}
	mSFS.On("IdentityResolver").Return(mIR)
	mIR.On("IsCurrentNode", txNodeID).Return(true)

	// returns no deps if no dispatch address assigned yet
	txPreReq := ds.GetIncompletePreReqTxIDs(ctx, testTx, mSFS)
	assert.Nil(t, txPreReq)
	testTx.DispatchAddress = txSigningAddress

	mDC := &enginemocks.DependencyChecker{}
	mSFS.On("DependencyChecker").Return(mDC)
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		checkFn := args[2].(func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool))
		assert.False(t, checkFn(preReqTx))
	}).Return([]string{preReqTx.ID.String()})

	// PreReq transaction dispatched by the same address
	preReqTx.DispatchAddress = txSigningAddress
	// return dependency when preReq transaction haven't get a dispatch tx id (nonce allocation) yet
	txPreReq = ds.GetIncompletePreReqTxIDs(ctx, testTx, mSFS)
	assert.NotNil(t, txPreReq)
	assert.Equal(t, 1, len(txPreReq.TxIDs))
	assert.Equal(t, preReqTx.ID.String(), txPreReq.TxIDs[0])

	// return nil, when preReqTx has nonce allocated
	preReqTx.DispatchTxID = uuid.NewString()
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		checkFn := args[2].(func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool))
		assert.True(t, checkFn(preReqTx))
	}).Return(nil)
	txPreReq = ds.GetIncompletePreReqTxIDs(ctx, testTx, mSFS)
	assert.Nil(t, txPreReq)

	//  PreReq transaction dispatched by a different address
	// return dependency when preReq transaction haven't been confirmed on the base ledger yet
	preReqTx.DispatchAddress = txDifferentSigningAddr
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		checkFn := args[2].(func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool))
		assert.False(t, checkFn(preReqTx))
	}).Return([]string{preReqTx.ID.String()})
	txPreReq = ds.GetIncompletePreReqTxIDs(ctx, testTx, mSFS)
	assert.NotNil(t, txPreReq)
	assert.Equal(t, 1, len(txPreReq.TxIDs))
	assert.Equal(t, preReqTx.ID.String(), txPreReq.TxIDs[0])

	// return nil, when preReqTx has been confirmed
	preReqTx.ConfirmedTxHash = testTxHash
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		checkFn := args[2].(func(preReqTx transactionstore.TxStateGetters) (preReqComplete bool))
		assert.True(t, checkFn(preReqTx))
	}).Return(nil)
	txPreReq = ds.GetIncompletePreReqTxIDs(ctx, testTx, mSFS)
	assert.Nil(t, txPreReq)
}

func TestDispatchStageAssignDispatchAddress(t *testing.T) {
	ctx := context.Background()
	txNodeID := "current_node_id"
	txSigningAddress := "0xb60e8dd61c5d32be8058bb8eb970870f07233155"

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())
	preReqTx := &transactionstore.Transaction{
		ID:              uuid.New(),
		DispatchNode:    txNodeID,
		DispatchAddress: txSigningAddress,
		DispatchTxID:    uuid.NewString(),
	}

	testTx := &transactionstore.Transaction{
		ID:                uuid.New(),
		DispatchNode:      txNodeID,
		DispatchTxPayload: "payload",
		PreReqTxs:         []string{preReqTx.ID.String()},
	}
	mSFS := &enginemocks.StageFoundationService{}
	mIR := &enginemocks.IdentityResolver{}
	mSFS.On("IdentityResolver").Return(mIR)
	mIR.On("IsCurrentNode", txNodeID).Return(true)

	mDC := &enginemocks.DependencyChecker{}
	mSFS.On("DependencyChecker").Return(mDC)

	// return error when no dispatch address is found
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Return(nil)
	mDC.On("GetPreReqDispatchAddresses", ctx, testTx.PreReqTxs).Once().Return([]string{txSigningAddress})
	mIR.On("GetDispatchAddress", []string{txSigningAddress}).Once().Return("")
	actionOutput, actionErr := ds.PerformAction(ctx, testTx, mSFS)
	assert.Regexp(t, "PD010302", actionErr.Error())
	assert.Nil(t, actionOutput)

	// output the dispatch address
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Return(nil)
	mDC.On("GetPreReqDispatchAddresses", ctx, testTx.PreReqTxs).Once().Return([]string{txSigningAddress})
	mIR.On("GetDispatchAddress", []string{txSigningAddress}).Once().Return(txSigningAddress)

	actionOutput, actionErr = ds.PerformAction(ctx, testTx, mSFS)
	assert.NoError(t, actionErr)
	assert.Equal(t, DispatchAddress(txSigningAddress), actionOutput)

	// process the output event generate correct next step
	upe, txUpdate, nextStep := ds.ProcessEvents(ctx, testTx, mSFS, []*types.StageEvent{
		{
			ID:    uuid.NewString(),
			Stage: ds.Name(),
			TxID:  testTx.ID.String(),
			Data:  actionOutput,
		},
	})
	assert.Empty(t, upe)
	assert.NotNil(t, txUpdate)
	assert.NotNil(t, txUpdate.DispatchAddress)
	assert.Equal(t, txSigningAddress, *txUpdate.DispatchAddress)
	assert.Equal(t, types.NextStepNewAction, nextStep)
}

func TestDispatchStageSubmitTx(t *testing.T) {
	ctx := context.Background()
	txNodeID := "current_node_id"
	txSigningAddress := "0xb60e8dd61c5d32be8058bb8eb970870f07233155"

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())
	preReqTx := &transactionstore.Transaction{
		ID:              uuid.New(),
		DispatchNode:    txNodeID,
		DispatchAddress: txSigningAddress,
		DispatchTxID:    uuid.NewString(),
	}

	testTx := &transactionstore.Transaction{
		ID:                uuid.New(),
		DispatchNode:      txNodeID,
		DispatchTxPayload: "payload",
		DispatchAddress:   txSigningAddress,
		PreReqTxs:         []string{preReqTx.ID.String()},
	}
	mSFS := &enginemocks.StageFoundationService{}
	mIR := &enginemocks.IdentityResolver{}
	mSFS.On("IdentityResolver").Return(mIR)
	mIR.On("IsCurrentNode", txNodeID).Return(true)

	mDC := &enginemocks.DependencyChecker{}
	mSFS.On("DependencyChecker").Return(mDC)

	// returns error when pre-req not met
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Return([]string{preReqTx.ID.String()})
	actionOutput, actionErr := ds.PerformAction(ctx, testTx, mSFS)
	assert.Regexp(t, "PD010303", actionErr.Error())
	assert.Nil(t, actionOutput)

	// TODO: submission result check
	mDC.On("PreReqsMatchCondition", ctx, mock.Anything, mock.Anything).Once().Return(nil)
	_, actionErr = ds.PerformAction(ctx, testTx, mSFS)
	assert.NoError(t, actionErr)

	// process the event generate correct next step
	dispatchTxID := uuid.NewString()
	upe, txUpdate, nextStep := ds.ProcessEvents(ctx, testTx, mSFS, []*types.StageEvent{
		{
			ID:    uuid.NewString(),
			Stage: ds.Name(),
			TxID:  testTx.ID.String(),
			Data: TxSubmissionOutput{
				TransactionID: dispatchTxID,
			},
		},
	})
	assert.Empty(t, upe)
	assert.NotNil(t, txUpdate)
	assert.NotNil(t, txUpdate.DispatchTxID)
	assert.Equal(t, dispatchTxID, *txUpdate.DispatchTxID)
	assert.Equal(t, types.NextStepNewStage, nextStep)

	// wait if there is an error
	upe, txUpdate, nextStep = ds.ProcessEvents(ctx, testTx, mSFS, []*types.StageEvent{
		{
			ID:    uuid.NewString(),
			Stage: ds.Name(),
			TxID:  testTx.ID.String(),
			Data: TxSubmissionOutput{
				ErrorMessage: "submission error",
			},
		},
	})
	assert.Empty(t, upe)
	assert.Nil(t, txUpdate)
	assert.Equal(t, types.NextStepWait, nextStep)
}

func TestDispatchStageProcessEvents(t *testing.T) {
	ctx := context.Background()
	txNodeID := "current_node_id"
	txSigningAddress := "0xb60e8dd61c5d32be8058bb8eb970870f07233155"

	ds := &DispatchStage{}
	assert.Equal(t, "dispatch", ds.Name())
	preReqTx := &transactionstore.Transaction{
		ID: uuid.New(),
	}

	testTx := &transactionstore.Transaction{
		ID:                uuid.New(),
		DispatchNode:      txNodeID,
		DispatchTxPayload: "payload",
		DispatchAddress:   txSigningAddress,
		PreReqTxs:         []string{preReqTx.ID.String()},
	}
	mSFS := &enginemocks.StageFoundationService{}
	// wait on panic error and return unprocessed events
	upe, txUpdate, nextStep := ds.ProcessEvents(ctx, testTx, mSFS, []*types.StageEvent{
		{
			ID:    uuid.NewString(),
			Stage: ds.Name(),
			TxID:  testTx.ID.String(),
		}, {
			ID:    uuid.NewString(),
			Stage: "different",
			TxID:  testTx.ID.String(),
			Data:  12,
		},
	})
	assert.Equal(t, 1, len(upe))
	assert.Equal(t, "different", upe[0].Stage)
	assert.Nil(t, txUpdate)
	assert.Equal(t, types.NextStepWait, nextStep)

}
