/*
 * Copyright © 2025 Kaleido, Inc.
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
package transaction

import (
	"context"
	"testing"
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/stateview"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/testutil"
	"github.com/LFDT-Paladin/paladin/core/mocks/sequencercommonmocks"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransaction_NilPrivateTransaction_ReturnsError(t *testing.T) {
	ctx := context.Background()
	_, err := NewTransaction(ctx, nil, nil, "node1", nil, nil, nil, nil, nil, func(_ context.Context) {}, func() int64 { return 0 }, common.RealClock(), time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot create transaction without private tx")
}

func TestNewTransaction_Success_ReturnsOriginatorTransaction(t *testing.T) {
	ctx := context.Background()
	pt := testutil.NewPrivateTransactionBuilderForTesting().Build()
	engine := sequencercommonmocks.NewEngineIntegration(t)
	recorder := testutil.NewSentMessageRecorder()
	queue := func(context.Context, common.Event) {}
	m := metrics.InitMetrics(context.Background(), prometheus.NewRegistry())

	ot, err := NewTransaction(ctx, pt, nil, "node1", recorder, stateview.NewReader(pt.Address.HexString(), recorder, time.Second, common.RealClock()), queue, engine, m, func(_ context.Context) {}, func() int64 { return 0 }, common.RealClock(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, ot)
	assert.Equal(t, pt.ID, ot.GetID())
	assert.Equal(t, State_Initial, ot.GetCurrentState())
	assert.Same(t, pt, ot.GetPrivateTransaction())
}

func TestTransaction_GetPrivateTransaction_ReturnsPt(t *testing.T) {
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()
	pt := txn.pt
	assert.Same(t, pt, txn.GetPrivateTransaction())
}

func TestTransaction_GetStatus_NilPt_ReturnsUnknown(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()
	txn.pt = nil
	status := txn.GetStatus(ctx)
	assert.Equal(t, "", status.TxID)
	assert.Equal(t, "unknown", status.Status)
}

func TestTransaction_GetStatus_NilPostAssembly_ReturnsStatusWithNilEndorsements(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()
	txn.pt.PostAssembly = nil
	txn.stateMachine.SetCurrentState(State_Assembling)
	status := txn.GetStatus(ctx)
	assert.Equal(t, txn.pt.ID.String(), status.TxID)
	assert.Equal(t, "Assembling", status.Status)
	assert.Nil(t, status.Endorsements)
}

func TestTransaction_GetStatus_ReturnsStatusWithEndorsements(t *testing.T) {
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()
	txn.pt.PostAssembly = &components.TransactionPostAssembly{
		AssembleResponse: &prototk.TransactionPostAssembly{
			AttestationPlan: []*prototk.AttestationRequest{
				{Name: "att1", AttestationType: prototk.AttestationType_ENDORSE, Parties: []string{"party1"}, VerifierType: "v1"},
			},
			Endorsements: []*prototk.AttestationResult{
				{Name: "att1", Verifier: &prototk.ResolvedVerifier{Lookup: "party1", VerifierType: "v1"}},
			},
		},
	}
	txn.stateMachine.SetCurrentState(State_Assembling)
	status := txn.GetStatus(ctx)
	assert.Equal(t, txn.pt.ID.String(), status.TxID)
	assert.Equal(t, "Assembling", status.Status)
	require.Len(t, status.Endorsements, 1)
	assert.Equal(t, "party1", status.Endorsements[0].Party)
	assert.True(t, status.Endorsements[0].EndorsementReceived)
	assert.Same(t, txn.pt, status.Transaction)
}

func TestTransaction_GetLastDelegatedTime_InitiallyNil(t *testing.T) {
	// Test that GetLastDelegatedTime returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	lastDelegatedTime := txn.GetLastDelegatedTime()
	assert.Nil(t, lastDelegatedTime, "GetLastDelegatedTime should return nil for a newly created transaction")
}

func TestTransaction_UpdateLastDelegatedTime_SetsTime(t *testing.T) {
	// Test that UpdateLastDelegatedTime sets a non-nil time value
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Initially should be nil
	require.Nil(t, txn.GetLastDelegatedTime(), "LastDelegatedTime should be nil initially")

	// Update the time
	txn.updateLastDelegatedTime()

	// Verify that the time is now set
	lastDelegatedTime := txn.GetLastDelegatedTime()
	assert.NotNil(t, lastDelegatedTime, "GetLastDelegatedTime should return a non-nil value after UpdateLastDelegatedTime")
}

func TestTransaction_UpdateLastDelegatedTime_UpdatesTime(t *testing.T) {
	// Test that multiple calls to UpdateLastDelegatedTime update the time value
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// First update
	txn.updateLastDelegatedTime()
	firstTime := txn.GetLastDelegatedTime()
	require.NotNil(t, firstTime, "First update should set a time")

	// Wait a small amount to ensure the next time will be different
	time.Sleep(10 * time.Millisecond)

	// Second update
	txn.updateLastDelegatedTime()
	secondTime := txn.GetLastDelegatedTime()
	require.NotNil(t, secondTime, "Second update should set a time")

	// Verify that the times are different

	assert.True(t, secondTime.After(*firstTime), "Second time should be after the first time")
}

func TestTransaction_GetLastDelegatedTime_ReturnsUpdatedTime(t *testing.T) {
	// Test that GetLastDelegatedTime returns the time set by UpdateLastDelegatedTime
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	// Update the time
	txn.updateLastDelegatedTime()

	// Get the time multiple times and verify it returns the same value
	time1 := txn.GetLastDelegatedTime()
	time2 := txn.GetLastDelegatedTime()

	assert.Equal(t, time1, time2, "GetLastDelegatedTime should return the same value when called multiple times without updating")
	assert.NotNil(t, time1, "GetLastDelegatedTime should return a non-nil value")
}

func TestTransaction_GetFirstDelegatedTime_InitiallyNil(t *testing.T) {
	// Test that GetFirstDelegatedTime returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	firstDelegatedTime := txn.GetFirstDelegatedTime()
	assert.Nil(t, firstDelegatedTime, "GetFirstDelegatedTime should return nil for a newly created transaction")
}

func TestTransaction_GetFirstDelegatedTime_ReturnsSetTime(t *testing.T) {
	// Test that GetFirstDelegatedTime returns the time recorded on first delegation
	ctx := context.Background()
	builder := NewTransactionBuilderForTesting(t, State_Pending)
	txn, _ := builder.BuildWithMocks()

	require.NoError(t, action_Delegated(ctx, txn, &DelegatedEvent{
		BaseEvent:   BaseEvent{TransactionID: txn.pt.ID},
		Coordinator: "coord@node1",
	}))

	firstDelegatedTime := txn.GetFirstDelegatedTime()
	require.NotNil(t, firstDelegatedTime, "GetFirstDelegatedTime should return a non-nil value after delegation")
	assert.Equal(t, txn.firstDelegatedTime, firstDelegatedTime, "GetFirstDelegatedTime should return the recorded first-delegation time")
}

func TestTransaction_GetCurrentState_ReturnsInitialState(t *testing.T) {
	// Test that GetCurrentState returns the initial state for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	state := txn.GetCurrentState()
	assert.Equal(t, State_Initial, state, "GetCurrentState should return State_Initial for a newly created transaction")
}

func TestTransaction_GetCurrentState_ReturnsDifferentStates(t *testing.T) {
	// Test that GetCurrentState returns the correct state for different state values
	testCases := []struct {
		name  string
		state State
	}{
		{"Initial", State_Initial},
		{"Pending", State_Pending},
		{"Delegated", State_Delegated},
		{"Assembling", State_Assembling},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := NewTransactionBuilderForTesting(t, tc.state)
			txn, _ := builder.BuildWithMocks()

			state := txn.GetCurrentState()
			assert.Equal(t, tc.state, state, "GetCurrentState should return the expected state")
		})
	}
}

func TestTransaction_GetSignerAddress_ReturnsNilInitially(t *testing.T) {
	// Test that GetSignerAddress returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	signerAddress := txn.GetSignerAddress()
	assert.Nil(t, signerAddress, "GetSignerAddress should return nil for a newly created transaction")
}

func TestTransaction_GetSignerAddress_ReturnsSetAddress(t *testing.T) {
	// Test that GetSignerAddress returns the address that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedAddress := pldtypes.RandAddress()
	txn.signerAddress = expectedAddress

	signerAddress := txn.GetSignerAddress()
	assert.Equal(t, expectedAddress, signerAddress, "GetSignerAddress should return the address that was set")
	assert.NotNil(t, signerAddress, "GetSignerAddress should return a non-nil address")
}

func TestTransaction_GetLatestSubmissionHash_ReturnsNilInitially(t *testing.T) {
	// Test that GetLatestSubmissionHash returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	hash := txn.GetLatestSubmissionHash()
	assert.Nil(t, hash, "GetLatestSubmissionHash should return nil for a newly created transaction")
}

func TestTransaction_GetLatestSubmissionHash_ReturnsSetHash(t *testing.T) {
	// Test that GetLatestSubmissionHash returns the hash that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedHash := ptrTo(pldtypes.RandBytes32())
	txn.latestSubmissionHash = expectedHash

	hash := txn.GetLatestSubmissionHash()
	assert.Equal(t, expectedHash, hash, "GetLatestSubmissionHash should return the hash that was set")
	assert.NotNil(t, hash, "GetLatestSubmissionHash should return a non-nil hash")
}

func TestTransaction_GetNonce_ReturnsNilInitially(t *testing.T) {
	// Test that GetNonce returns nil for a newly created transaction
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	nonce := txn.GetNonce()
	assert.Nil(t, nonce, "GetNonce should return nil for a newly created transaction")
}

func TestTransaction_GetNonce_ReturnsSetNonce(t *testing.T) {
	// Test that GetNonce returns the nonce that was set
	builder := NewTransactionBuilderForTesting(t, State_Initial)
	txn, _ := builder.BuildWithMocks()

	expectedNonce := uint64(12345)
	txn.nonce = &expectedNonce

	nonce := txn.GetNonce()
	assert.Equal(t, &expectedNonce, nonce, "GetNonce should return the nonce that was set")
	assert.NotNil(t, nonce, "GetNonce should return a non-nil nonce")
	assert.Equal(t, expectedNonce, *nonce, "GetNonce should return the correct nonce value")
}
