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
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testHashedSignedMessage string = "0x307837333639363736653635363432303664363537333733363136373635"
const testTxHash string = "0x0503bb2e013a6ecfe29c6c7e073d6f0cf834edf6d305606c4e4623c98cb7fa5a"
const testWrongTxHash string = "0x0503bb2e013a6ecfe29c6c3e073d6f0cf834edf6d305606c4e4623c98cb7fa5a"

func testCancel(ctx context.Context) bool {
	return false
}

func TestTxSubmissionWithSignedMessage(t *testing.T) {

	textTxHashByte32 := pldtypes.MustParseBytes32(testTxHash)
	textWrongTxHashByte32 := pldtypes.MustParseBytes32(testWrongTxHash)

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.SubmissionRetry.MaxAttempts = confutil.P(1)
	})
	defer done()
	it, ifts := newInflightTransaction(o, 1)
	ifts.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		TransactionHash: &textTxHashByte32,
	})

	// successful send with tx hash returned
	txSendMock := m.ethClient.On("SendRawTransaction", ctx, mock.Anything)
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(&textTxHashByte32, nil)
	}).Once()

	txHash, _, errReason, outCome, err := it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String())

	// successful send with tx hash missing
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, nil)
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash

	// error send due to tx hash mismatch
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(&textWrongTxHashByte32, nil)
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "PD011905", err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Nil(t, txHash)

	// underpriced
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("transaction underpriced"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "transaction underpriced", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionUnderpriced, errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash.String())
	// reverted
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("execution reverted"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "execution reverted", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionReverted, errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash.String())
	// known transaction
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("known transaction"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeAlreadyKnown, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash
	// nonce too low
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("nonce too low"))
	}).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeNonceTooLow, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash

	// other error
	txSendMock.Run(func(args mock.Arguments) {
		txRawMessage := args[1].(pldtypes.HexBytes)
		assert.Equal(t, pldtypes.MustParseHexBytes(testHashedSignedMessage), txRawMessage)
		txSendMock.Return(nil, fmt.Errorf("error submitting transaction"))
	}).Once()

	_, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "error submitting", err)
	assert.Equal(t, ethclient.ErrorReason(""), errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
}

func TestTxSubmissionWithSignedMessageWithRetry(t *testing.T) {

	textTxHashByte32 := pldtypes.MustParseBytes32(testTxHash)
	textWrongTxHashByte32 := pldtypes.MustParseBytes32(testWrongTxHash)

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *pldconf.PublicTxManagerConfig) {
		conf.Orchestrator.SubmissionRetry.MaxAttempts = confutil.P(2)
	})
	defer done()
	it, ifts := newInflightTransaction(o, 1)
	ifts.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		TransactionHash: &textTxHashByte32,
	})

	// successful send with tx hash returned
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err := it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String())

	// successful send with tx hash missing
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(
		nil,
		nil,
	).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash

	// successful send but tx hash mismatch first time
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(&textWrongTxHashByte32, nil).Once()
	// but corrected in the retry
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String())

	// categorized errors should not be retried
	// underpriced
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("transaction underpriced")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "transaction underpriced", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionUnderpriced, errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash.String())
	// reverted
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("execution reverted")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "execution reverted", err)
	assert.Equal(t, ethclient.ErrorReasonTransactionReverted, errReason)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Equal(t, testTxHash, txHash.String())
	// known transaction
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("known transaction")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeAlreadyKnown, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash
	// nonce too low
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("nonce too low")).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeNonceTooLow, outCome)
	assert.Equal(t, testTxHash, txHash.String()) // able to use the calculated hash

	// un-categorized errors should be retried
	// successful send when first time returned un-categorized error
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(nil, fmt.Errorf("error submitting transaction")).Once()

	// but the second time was successful
	m.ethClient.On("SendRawTransaction", ctx, mock.Anything).Return(&textTxHashByte32, nil).Once()

	txHash, _, errReason, outCome, err = it.submitTX(ctx,
		[]byte(testTransactionData),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	require.NoError(t, err)
	assert.Empty(t, errReason)
	assert.Equal(t, SubmissionOutcomeSubmittedNew, outCome)
	assert.Equal(t, testTxHash, txHash.String())

	// retry error
	canceledContext, cancel := context.WithCancel(ctx)
	cancel()
	m.ethClient.On("SendRawTransaction", canceledContext, mock.Anything).Return(nil, fmt.Errorf("error submitting transaction")).Once()
	txHash, _, _, outCome, err = it.submitTX(canceledContext,
		[]byte(testHashedSignedMessage),
		it.stateManager.GetTransactionHash(),
		it.stateManager.GetSignerNonce(),
		it.stateManager.GetLastSubmitTime(),
		testCancel)
	assert.Regexp(t, "PD020000", err)
	assert.Equal(t, SubmissionOutcomeFailedRequiresRetry, outCome)
	assert.Nil(t, txHash)
}
