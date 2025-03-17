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

package ptmgrtypes

/*
   This package defines interfaces that are used within the privatetxnmgr package
   They are defined in a separate package to avoid circular dependencies between the privatetxnmgr package mocks.
   All interfaces of privatetxnmgr intended to be consumed by other packages should be defined in components package
*/

import (
	"context"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type EndorsementRequest struct {
	TransactionID string
	InputStates   []string
}

type Transaction struct {
	ID              string
	AssemblerNodeID string
	OutputStates    []string
	InputStates     []string
}

type Publisher interface {
	//Service for sending messages and events within the local node
	PublishTransactionDispatchedEvent(ctx context.Context, transactionId string, nonce uint64, signingAddress string)
	PublishTransactionPreparedEvent(ctx context.Context, transactionId string)
	PublishTransactionAssembledEvent(ctx context.Context, transactionId string, postAssembly *components.TransactionPostAssembly, requestID string)
	PublishTransactionAssembleFailedEvent(ctx context.Context, transactionId string, errorMessage string, requestID string)
	PublishTransactionSignedEvent(ctx context.Context, transactionId string, attestationResult *prototk.AttestationResult)
	PublishTransactionEndorsedEvent(ctx context.Context, transactionId string, idempotencyKey string, party string, attestationRequestName string, attestationResult *prototk.AttestationResult, revertReason *string)
	PublishResolveVerifierResponseEvent(ctx context.Context, transactionId string, lookup, algorithm, verifier, verifierType string)
	PublishResolveVerifierErrorEvent(ctx context.Context, transactionId string, lookup, algorithm, errorMessage string)
	PublishTransactionFinalizedEvent(ctx context.Context, transactionId string)
	PublishTransactionFinalizeError(ctx context.Context, transactionId string, revertReason string, err error)
	PublishTransactionConfirmedEvent(ctx context.Context, transactionId string)
	PublishNudgeEvent(ctx context.Context, transactionId string)
}

// Map of signing address to an ordered list of transaction flows that are ready to be dispatched by that signing address
type DispatchableTransactions map[string][]TransactionFlow

func (dtxs *DispatchableTransactions) IDs(ctx context.Context) []string {
	var ids []string
	for _, txs := range *dtxs {
		for _, tx := range txs {
			ids = append(ids, tx.ID(ctx).String())
		}
	}
	return ids
}

type Dispatcher interface {
	// Dispatcher is the component that takes responsibility for submitting the transactions in the sequence to the base ledger in the correct order
	DispatchTransactions(context.Context, DispatchableTransactions) error
}

type EndorsementGatherer interface {
	// TODO: Consider if this is the right object to hold this
	DomainContext() components.DomainContext

	//integrate with local signer and domain manager to satisfy the given endorsement request
	// that may have came from a transaction assembled locally or from another node
	GatherEndorsement(
		ctx context.Context,
		transactionSpecification *prototk.TransactionSpecification,
		verifiers []*prototk.ResolvedVerifier,
		signatures []*prototk.AttestationResult,
		inputStates []*prototk.EndorsableState,
		readStates []*prototk.EndorsableState,
		outputStates []*prototk.EndorsableState,
		infoStates []*prototk.EndorsableState,
		partyName string,
		endorsementRequest *prototk.AttestationRequest) (*prototk.AttestationResult, *string, error)
}

type ContentionResolver interface {
	Resolve(stateID, biddingContentionResolver1, biddingContentionResolver2 string) (string, error)
}

type TransportWriter interface {
	SendDelegationRequest(ctx context.Context, delegationId string, delegateNodeName string, transaction *components.PrivateTransaction, blockHeight int64) error
	SendDelegationRequestAcknowledgment(ctx context.Context, delegatingNodeName string, delegationId string, delegateNodeName string, transactionID string) error
	SendEndorsementRequest(ctx context.Context, idempotencyKey string, party string, targetNode string, contractAddress string, transactionID string, attRequest *prototk.AttestationRequest, transactionSpecification *prototk.TransactionSpecification, verifiers []*prototk.ResolvedVerifier, signatures []*prototk.AttestationResult, inputStates []*components.FullState, outputStates []*components.FullState, infoStates []*components.FullState) error
	SendAssembleRequest(ctx context.Context, assemblingNode string, assembleRequestID string, txID uuid.UUID, contractAddress string, preAssembly *components.TransactionPreAssembly, stateLocksJSON []byte, blockHeight int64) error
}

type TransactionFlowStatus int

const (
	TransactionFlowActive TransactionFlowStatus = iota
	TransactionFlowSuspend
	TransactionFlowResume
	TransactionFlowRemove
)

type TransactionFlow interface {
	GetTxStatus(ctx context.Context) (components.PrivateTxStatus, error)

	ApplyEvent(ctx context.Context, event PrivateTransactionEvent)
	Action(ctx context.Context)

	PrepareTransaction(ctx context.Context, defaultSigner string) (*components.PrivateTransaction, error)
	GetStateDistributions(ctx context.Context) (*components.StateDistributionSet, error)
	CoordinatingLocally(ctx context.Context) bool
	IsComplete(ctx context.Context) bool
	ReadyForSequencing(ctx context.Context) bool
	Dispatched(ctx context.Context) bool
	ID(ctx context.Context) uuid.UUID
	IsEndorsed(ctx context.Context) bool
	InputStateIDs(ctx context.Context) []string
	OutputStateIDs(ctx context.Context) []string
	Signer(ctx context.Context) string
}

type Clock interface {
	//wrapper of time.Now()
	//primarily to allow artificial clocks to be injected for testing
	Now() time.Time
}
type realClock struct{}

func (c *realClock) Now() time.Time {
	return time.Now()
}
func RealClock() Clock {
	return &realClock{}
}

type CoordinatorSelector interface {
	SelectCoordinatorNode(ctx context.Context, transaction *components.PrivateTransaction, environment SequencerEnvironment) (int64, string, error)
}

type SequencerEnvironment interface {
	GetBlockHeight() int64
}

// AssembleCoordinator is a component that is responsible for coordinating the assembly of all transactions for a given domain contract instance
// requests to assemble transactions are queued and the queue is processed on a single thread that blocks until one assemble completes before starting the next
type AssembleCoordinator interface {
	Start()
	Stop()
	QueueAssemble(ctx context.Context, assemblingNode string, transactionID uuid.UUID, transactionPreAssembly *components.TransactionPreAssembly)
	Complete(requestID string)
}

type LocalAssembler interface {
	AssembleLocal(
		ctx context.Context,
		requestID string,
		transactionID uuid.UUID,
		preAssembly *components.TransactionPreAssembly,
	)
}
