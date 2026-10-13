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
	"sync"
	"time"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/originator/stateview"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type assembleRequestFromCoordinator struct {
	coordinatorsBlockHeight int64
	coordinator             string // the node the request came from; state view requests are answered by (and only by) this node
	requestID               uuid.UUID
	expiry                  time.Time
}

type OriginatorTransaction interface {
	HandleEvent(ctx context.Context, event common.Event) error
	GetID() uuid.UUID
	GetCurrentState() State
	GetPrivateTransaction() *components.PrivateTransaction
	GetStatus(ctx context.Context) components.PrivateTxStatus
	GetFirstDelegatedTime() *time.Time
}

// OriginatorTransaction tracks the state of a transaction that is being sent by the local node in originator state.
// It implements statemachine.Lockable; the state machine holds this lock for the duration of each ProcessEvent call.
// pt holds the private transaction; it is not embedded so that all modifications must go through this package.
type originatorTransaction struct {
	sync.RWMutex
	stateMachine                     *StateMachine
	pt                               *components.PrivateTransaction
	localTx                          *components.ResolvedTransaction // resolved before delegation and reused on the assemble path to avoid a per-assembly DB read
	nodeName                         string
	engineIntegration                common.EngineIntegration
	transportWriter                  transport.TransportWriter
	stateViewReader                  stateview.Reader
	queueEventForOriginator          func(context.Context, common.Event)
	currentDelegate                  string
	lastDelegatedTime                *time.Time
	firstDelegatedTime               *time.Time // set on first delegation to the current coordinator; reset when the coordinator changes
	latestAssembleRequest            *assembleRequestFromCoordinator
	latestFulfilledAssembleRequestID uuid.UUID
	latestPreDispatchRequestID       uuid.UUID
	signerAddress                    *pldtypes.EthAddress
	latestSubmissionHash             *pldtypes.Bytes32
	nonce                            *uint64
	metrics                          metrics.DistributedSequencerMetrics
	refreshBlockHeight               func(context.Context)
	getBlockHeight                   func() int64
	cancelCurrentAssembly            context.CancelFunc
	currentAssemblyRequestID         uuid.UUID
	cancelCurrentSign                context.CancelFunc
	clock                            common.Clock
	resolveRetryBackoff              time.Duration
	cancelResolveRetry               func() // cancels a pending verifier-resolution retry timer; nil when none is scheduled
	stateEntryTime                   time.Time
}

func NewTransaction(
	ctx context.Context,
	pt *components.PrivateTransaction,
	localTx *components.ResolvedTransaction,
	nodeName string,
	transportWriter transport.TransportWriter,
	stateViewReader stateview.Reader,
	queueEventForOriginator func(context.Context, common.Event),
	engineIntegration common.EngineIntegration,
	metrics metrics.DistributedSequencerMetrics,
	refreshBlockHeight func(context.Context),
	getBlockHeight func() int64,
	clock common.Clock,
	resolveRetryBackoff time.Duration,
) (OriginatorTransaction, error) {
	if pt == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot create transaction without private tx")
	}

	return newTransaction(
		pt,
		localTx,
		nodeName,
		engineIntegration,
		transportWriter,
		stateViewReader,
		queueEventForOriginator,
		metrics,
		refreshBlockHeight,
		getBlockHeight,
		clock,
		resolveRetryBackoff,
	), nil
}

func newTransaction(
	pt *components.PrivateTransaction,
	localTx *components.ResolvedTransaction,
	nodeName string,
	engineIntegration common.EngineIntegration,
	transportWriter transport.TransportWriter,
	stateViewReader stateview.Reader,
	queueEventForOriginator func(context.Context, common.Event),
	metrics metrics.DistributedSequencerMetrics,
	refreshBlockHeight func(context.Context),
	getBlockHeight func() int64,
	clock common.Clock,
	resolveRetryBackoff time.Duration,
) *originatorTransaction {
	txn := &originatorTransaction{
		pt:                      pt,
		localTx:                 localTx,
		nodeName:                nodeName,
		engineIntegration:       engineIntegration,
		transportWriter:         transportWriter,
		stateViewReader:         stateViewReader,
		queueEventForOriginator: queueEventForOriginator,
		metrics:                 metrics,
		refreshBlockHeight:      refreshBlockHeight,
		getBlockHeight:          getBlockHeight,
		clock:                   clock,
		resolveRetryBackoff:     resolveRetryBackoff,
	}
	txn.initializeStateMachine(State_Initial)
	return txn
}

func (t *originatorTransaction) GetID() uuid.UUID {
	t.RLock()
	defer t.RUnlock()
	return t.pt.ID
}

// GetPrivateTransaction returns the private transaction for code where we really cannot do without the whole struct.
func (t *originatorTransaction) GetPrivateTransaction() *components.PrivateTransaction {
	t.RLock()
	defer t.RUnlock()
	return t.pt
}

// GetStatus returns the transaction status for external use. Caller may call from any goroutine.
func (t *originatorTransaction) GetStatus(ctx context.Context) components.PrivateTxStatus {
	t.RLock()
	defer t.RUnlock()
	if t.pt == nil {
		return components.PrivateTxStatus{TxID: "", Status: "unknown"}
	}
	return components.PrivateTxStatus{
		TxID:         t.pt.ID.String(),
		Status:       t.stateMachine.GetCurrentState().String(),
		LatestEvent:  t.stateMachine.GetLatestEvent(),
		Endorsements: t.getEndorsementStatus(ctx),
		Transaction:  t.pt,
	}
}

func (t *originatorTransaction) getEndorsementStatus(ctx context.Context) []components.PrivateTxEndorsementStatus {
	if t.pt == nil || t.pt.PostAssembly == nil {
		return nil
	}
	endorsementRequestStates := make([]components.PrivateTxEndorsementStatus, len(t.pt.PostAssembly.AssembleResponse.GetAttestationPlan()))
	for i, attRequest := range t.pt.PostAssembly.AssembleResponse.GetAttestationPlan() {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				endorsementRequestState := &components.PrivateTxEndorsementStatus{Party: party, EndorsementReceived: false}
				for _, endorsement := range t.pt.PostAssembly.AssembleResponse.GetEndorsements() {
					log.L(ctx).Debugf("existing endorsement from party %s", endorsement.Verifier.Lookup)
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType
					if found {
						endorsementRequestState.EndorsementReceived = true
						break
					}
				}
				endorsementRequestStates[i] = *endorsementRequestState
			}
		}
	}
	return endorsementRequestStates
}

func ptrTo[T any](v T) *T {
	return &v
}

func (t *originatorTransaction) GetCurrentState() State {
	t.RLock()
	defer t.RUnlock()
	return t.stateMachine.GetCurrentState()
}

func (t *originatorTransaction) GetSignerAddress() *pldtypes.EthAddress {
	t.RLock()
	defer t.RUnlock()
	return t.signerAddress
}

func (t *originatorTransaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	t.RLock()
	defer t.RUnlock()
	return t.latestSubmissionHash
}

func (t *originatorTransaction) GetNonce() *uint64 {
	t.RLock()
	defer t.RUnlock()
	return t.nonce
}

func (t *originatorTransaction) GetLastDelegatedTime() *time.Time {
	t.RLock()
	defer t.RUnlock()
	return t.lastDelegatedTime
}

func (t *originatorTransaction) GetFirstDelegatedTime() *time.Time {
	t.RLock()
	defer t.RUnlock()
	return t.firstDelegatedTime
}
