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

package privatetxnmgr

import (
	"context"
	"sync"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/ptmgrtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/privatetxnmgr/syncpoints"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

func NewTransactionFlow(
	ctx context.Context,
	transaction *components.PrivateTransaction,
	nodeName string,
	components components.AllComponents,
	domainAPI components.DomainSmartContract,
	domainContext components.DomainContext,
	publisher ptmgrtypes.Publisher,
	endorsementGatherer ptmgrtypes.EndorsementGatherer,
	identityResolver components.IdentityResolver,
	syncPoints syncpoints.SyncPoints,
	transportWriter ptmgrtypes.TransportWriter,
	requestTimeout time.Duration,
	selectCoordinator ptmgrtypes.CoordinatorSelector,
	assembleCoordinator ptmgrtypes.AssembleCoordinator,
	environment ptmgrtypes.SequencerEnvironment,
) ptmgrtypes.TransactionFlow {

	return &transactionFlow{
		stageErrorRetry:             10 * time.Second,
		domainAPI:                   domainAPI,
		domainContext:               domainContext,
		nodeName:                    nodeName,
		components:                  components,
		publisher:                   publisher,
		endorsementGatherer:         endorsementGatherer,
		transaction:                 transaction,
		status:                      "new",
		identityResolver:            identityResolver,
		syncPoints:                  syncPoints,
		transportWriter:             transportWriter,
		finalizeRequired:            false,
		finalizePending:             false,
		requestedVerifierResolution: false,
		requestedSignatures:         false,
		pendingEndorsementRequests:  make(map[string]map[string]*endorsementRequest),
		complete:                    false,
		localCoordinator:            true,
		dispatched:                  false,
		prepared:                    false,
		clock:                       ptmgrtypes.RealClock(),
		requestTimeout:              requestTimeout,
		selectCoordinator:           selectCoordinator,
		assembleCoordinator:         assembleCoordinator,
		environment:                 environment,
	}
}

type endorsementRequest struct {
	//time the request was made
	requestTime time.Time
	//unique string to identify the request (non unique across retries)
	idempotencyKey string
}
type transactionFlow struct {
	stageErrorRetry             time.Duration
	components                  components.AllComponents
	nodeName                    string
	domainAPI                   components.DomainSmartContract
	domainContext               components.DomainContext
	transaction                 *components.PrivateTransaction
	publisher                   ptmgrtypes.Publisher
	endorsementGatherer         ptmgrtypes.EndorsementGatherer
	status                      string
	latestEvent                 string
	latestError                 string
	identityResolver            components.IdentityResolver
	syncPoints                  syncpoints.SyncPoints
	transportWriter             ptmgrtypes.TransportWriter
	finalizeRevertReason        string
	finalizeRequired            bool
	finalizePending             bool
	delegatePending             bool
	pendingDelegationRequestID  string
	delegateRequestTime         time.Time
	delegateRequestBlockHeight  int64
	delegated                   bool
	delegateRequestTimer        *time.Timer
	assemblePending             bool
	complete                    bool
	requestedVerifierResolution bool                                      //TODO add precision here so that we can track individual requests and implement retry as per endorsement
	requestedSignatures         bool                                      //TODO add precision here so that we can track individual requests and implement retry as per endorsement
	pendingEndorsementRequests  map[string]map[string]*endorsementRequest //map of attestationRequest names to a map of parties to a struct containing information about the active pending request
	localCoordinator            bool
	dispatched                  bool
	prepared                    bool
	clock                       ptmgrtypes.Clock
	requestTimeout              time.Duration
	selectCoordinator           ptmgrtypes.CoordinatorSelector
	assembleCoordinator         ptmgrtypes.AssembleCoordinator
	environment                 ptmgrtypes.SequencerEnvironment
	statusLock                  sync.RWMutex // under normal conditions, there should be only one contender for this lock ( the Write side of it) - i.e. the sequencer event loop so it should not normally slow things down
	// however, it is not safe for the API thread to read the in memory status while the even loop is writing so things will slow down on the event loop thread while an API consumer is reading the status
}

func (tf *transactionFlow) IsComplete(_ context.Context) bool {
	return tf.complete
}

func (tf *transactionFlow) ReadyForSequencing(ctx context.Context) bool {
	return tf.transaction.PostAssembly != nil
}

func (tf *transactionFlow) Dispatched(_ context.Context) bool {
	return tf.dispatched
}

func (tf *transactionFlow) IsEndorsed(ctx context.Context) bool {
	return !tf.hasOutstandingEndorsementRequests(ctx)
}

func (tf *transactionFlow) CoordinatingLocally(_ context.Context) bool {
	return tf.localCoordinator
}

func (tf *transactionFlow) PrepareTransaction(ctx context.Context, defaultSigner string) (*components.PrivateTransaction, error) {

	if tf.transaction.Signer == "" {
		log.L(ctx).Infof("Using random signing key from sequencer to prepare transaction: %s", defaultSigner)
		tf.transaction.Signer = defaultSigner
	}

	readTX := tf.components.Persistence().NOTX() // no DB transaction required here
	prepError := tf.domainAPI.PrepareTransaction(tf.domainContext, readTX, tf.transaction)
	if prepError != nil {
		log.L(ctx).Errorf("Error preparing transaction: %s", prepError)
		tf.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerPrepareError), prepError.Error())
		return nil, prepError
	}
	return tf.transaction, nil
}

func toEndorsableList(states []*components.FullState) []*prototk.EndorsableState {
	endorsableList := make([]*prototk.EndorsableState, len(states))
	for i, input := range states {
		endorsableList[i] = &prototk.EndorsableState{
			Id:            input.ID.String(),
			SchemaId:      input.Schema.String(),
			StateDataJson: string(input.Data),
		}
	}
	return endorsableList
}

func (tf *transactionFlow) GetStateDistributions(ctx context.Context) (*components.StateDistributionSet, error) {
	return newStateDistributionBuilder(tf.components, tf.transaction).Build(ctx)
}

func (tf *transactionFlow) InputStateIDs(_ context.Context) []string {

	inputStateIDs := make([]string, len(tf.transaction.PostAssembly.InputStates))
	for i, inputState := range tf.transaction.PostAssembly.InputStates {
		inputStateIDs[i] = inputState.ID.String()
	}
	return inputStateIDs
}

func (tf *transactionFlow) OutputStateIDs(_ context.Context) []string {

	//We use the output states here not the OutputStatesPotential because it is not possible for another transaction
	// to spend a state unless it has been written to the state store and at that point we have the state ID
	outputStateIDs := make([]string, len(tf.transaction.PostAssembly.OutputStates))
	for i, outputState := range tf.transaction.PostAssembly.OutputStates {
		outputStateIDs[i] = outputState.ID.String()
	}
	return outputStateIDs
}

func (tf *transactionFlow) Signer(_ context.Context) string {

	return tf.transaction.Signer
}

func (tf *transactionFlow) ID(_ context.Context) uuid.UUID {

	return tf.transaction.ID
}

func (tf *transactionFlow) PrivateTransaction() *components.PrivateTransaction {
	return tf.transaction
}
