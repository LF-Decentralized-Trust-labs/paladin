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

package baseledgertx

import (
	"context"
	"fmt"
	"testing"

	baseTypes "github.com/kaleido-io/paladin/core/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testTransactionData string = "0x7369676e6564206d657373616765"
const testHashedSignedMessage string = "0x307837333639363736653635363432303664363537333733363136373635"
const testTxHash string = "0x0503bb2e013a6ecfe29c6c7e073d6f0cf834edf6d305606c4e4623c98cb7fa5a"
const testWrongTxHash string = "0x0503bb2e013a6ecfe29c6c3e073d6f0cf834edf6d305606c4e4623c98cb7fa5a"

func TestTxSubmissionWithSignedMessage(t *testing.T) {

	textTxHashByte32 := types.MustParseBytes32(testTxHash)
	textWrongTxHashByte32 := types.MustParseBytes32(testWrongTxHash)
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mtx := it.stateManager.GetTx()

	// successful send with tx hash returned
	txSendMock := mEC.On("SendRawTransaction", ctx, mock.Anything)
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(&textTxHashByte32, nil)
	}).Once()

	txHash, _, errReason, outCome, err := it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash)

	// successful send with tx hash missing
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, nil)
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash

	// error send due to tx hash mismatch
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(&textWrongTxHashByte32, nil)
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "PD011905", err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, "", txHash)

	// underpriced
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("transaction underpriced"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "transaction underpriced", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionUnderpriced, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash)
	// reverted
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("execution reverted"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "execution reverted", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionReverted, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash)
	// known transaction
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("known transaction"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeAlreadyKnown, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash
	// nonce too low
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("nonce too low"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeNonceTooLow, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash

	// other error
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(types.HexBytes)
		assert.Equal(t, types.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("error submitting transaction"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "error submitting", err)
	assert.Equal(t, ethclient.ErrorReason(""), errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash
}

func TestTxSubmissionWithSignedMessageWithRetry(t *testing.T) {

	textTxHashByte32 := types.MustParseBytes32(testTxHash)
	textWrongTxHashByte32 := types.MustParseBytes32(testWrongTxHash)
	ctx := context.Background()
	testInFlightTransactionStateManagerWithMocks := NewTestInFlightTransactionWithMocks(t)
	it := testInFlightTransactionStateManagerWithMocks.it
	mEC := testInFlightTransactionStateManagerWithMocks.mEC
	mtx := it.stateManager.GetTx()

	it.transactionSubmissionRetryCount = 1 // retry once

	// successful send with tx hash returned
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err := it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash)

	// successful send with tx hash missing
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(
		nil,
		nil,
	).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash

	// successful send but tx hash mismatch first time
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(&textWrongTxHashByte32, nil).Once()
	// but corrected in the retry
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash)

	// categorized errors should not be retried
	// underpriced
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("transaction underpriced")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "transaction underpriced", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionUnderpriced, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash)
	// reverted
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("execution reverted")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Regexp(t, "execution reverted", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionReverted, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash)
	// known transaction
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("known transaction")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeAlreadyKnown, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash
	// nonce too low
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("nonce too low")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeNonceTooLow, outCome)
	assert.Equal(t, testTxHash, txHash) // able to use the calculated hash

	// un-categorized errors should be retried
	// successful send when first time returned un-categorized error
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("error submitting transaction")).Once()

	// but the second time was successful
	mEC.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx, mtx, []byte(testTransactionData))
	assert.Nil(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash)

	// retry error
	canceledContext, cancel := context.WithCancel(ctx)
	cancel()
	mEC.On("SendRawTransaction", canceledContext, mock.Anything).Return(nil, fmt.Errorf("error submitting transaction")).Once()
	txHash, _, errReason, outCome, err = it.submitTX(canceledContext, mtx, []byte(testHashedSignedMessage))
	assert.Regexp(t, "FF00154", err)
	assert.Regexp(t, "FF00154", errReason)
	assert.Equal(t, baseTypes.SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, "", txHash)
}
