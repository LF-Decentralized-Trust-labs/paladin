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

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/statedistribution"
	pbSequence "github.com/kaleido-io/paladin/core/pkg/proto/sequence"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
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

type Sequencer interface {
	/*
		HandleTransactionAssembledEvent needs to be called whenever a transaction has been assembled by any node in the network, including the local node.
	*/
	HandleTransactionAssembledEvent(ctx context.Context, event *pbSequence.TransactionAssembledEvent)

	/*
		HandleTransactionEndorsedEvent needs to be called whenever a the endorsement rules for the given domain have been satisfied for a given transaction.
	*/
	HandleTransactionEndorsedEvent(ctx context.Context, event *pbSequence.TransactionEndorsedEvent) error

	/*
		HandleTransactionDispatchResovedEvent needs to be called whenever a the signign address for a transaction has been resolved.
	*/
	HandleTransactionDispatchResolvedEvent(ctx context.Context, event *pbSequence.TransactionDispatchResolvedEvent) error

	/*
		HandleTransactionConfirmedEvent needs to be called whenever a transaction has been confirmed on the base ledger
		i.e. it has been included in a block with enough subsequent blocks to consider this final for that particular chain.
	*/
	HandleTransactionConfirmedEvent(ctx context.Context, event *pbSequence.TransactionConfirmedEvent) error

	/*
		OnTransationReverted needs to be called whenever a transaction has been rejected by any of the validation
		steps on any nodes or the base leddger contract. The transaction may or may not be reassembled after this
		hanlder is called.
	*/
	HandleTransactionRevertedEvent(ctx context.Context, event *pbSequence.TransactionRevertedEvent) error

	/*
		HandleTransactionDelegatedEvent needs to be called whenever a transaction has been delegated from one node to another
		this is an event that is broadcast to all nodes after the fact and should not be confused with the DelegateTransaction message which is
		an instruction to the delegate node.
	*/
	HandleTransactionDelegatedEvent(ctx context.Context, event *pbSequence.TransactionDelegatedEvent) error

	/*
		AssignTransaction is an instruction for the given transaction to be managed by this sequencer
	*/
	AssignTransaction(ctx context.Context, transactionID string)

	/*
		RemoveTransaction is an instruction for the given transaction to be no longer be managed by this sequencer
		A re-assembled version ( with the same ID ) of the transaction may be assigned to the sequencer at a later time.
	*/
	RemoveTransaction(ctx context.Context, transactionID string)

	/*
		ApproveEndorsement is a synchronous check of whether a given transaction could be endorsed by the local node. It asks the question:
		"given the information available to the local node at this point in time, does it appear that this transaction has no contention on input states".
	*/
	ApproveEndorsement(ctx context.Context, endorsementRequest EndorsementRequest) (bool, error)

	SetDispatcher(dispatcher Dispatcher)
}

type Publisher interface {
	//Service for sending messages and events within the local node
	PublishTransactionBlockedEvent(ctx context.Context, transactionId string)
	PublishTransactionDispatchedEvent(ctx context.Context, transactionId string, nonce uint64, signingAddress string)
	PublishTransactionSignedEvent(ctx context.Context, transactionId string, attestationResult *prototk.AttestationResult)
	PublishTransactionEndorsedEvent(ctx context.Context, transactionId string, attestationResult *prototk.AttestationResult, revertReason *string)
	PublishResolveVerifierResponseEvent(ctx context.Context, transactionId string, lookup, algorithm, verifier, verifierType string)
	PublishResolveVerifierErrorEvent(ctx context.Context, transactionId string, lookup, algorithm, errorMessage string)
	PublishTransactionFinalizedEvent(ctx context.Context, transactionId string)
	PublishTransactionFinalizeError(ctx context.Context, transactionId string, revertReason string, err error)
}

// Map of signing address to an ordered list of transaction IDs that are ready to be dispatched by that signing address
type DispatchableTransactions map[string][]string
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
		partyName string,
		endorsementRequest *prototk.AttestationRequest) (*prototk.AttestationResult, *string, error)
}

type ContentionResolver interface {
	Resolve(stateID, biddingContentionResolver1, biddingContentionResolver2 string) (string, error)
}

type TransportWriter interface {
	// Provided to the sequencer to allow it to send messages to other nodes in the network
	SendDelegateTransactionMessage(ctx context.Context, transactionId string, delegateNodeId string) error
	SendState(ctx context.Context, stateId string, schemaId string, stateDataJson string, party string) error
	SendDelegationRequest(ctx context.Context, delegationId string, delegateNodeId string, transaction *components.PrivateTransaction) error
}

type TxProcessorStatus int

const (
	TxProcessorActive TxProcessorStatus = iota
	TxProcessorSuspend
	TxProcessorResume
	TxProcessorRemove
)

type TxProcessor interface {
	Init(ctx context.Context)
	GetTxStatus(ctx context.Context) (components.PrivateTxStatus, error)
	GetStatus(ctx context.Context) TxProcessorStatus

	HandleTransactionSubmittedEvent(ctx context.Context, event *TransactionSubmittedEvent) error
	HandleTransactionSwappedInEvent(ctx context.Context, event *TransactionSwappedInEvent) error
	HandleTransactionAssembledEvent(ctx context.Context, event *TransactionAssembledEvent) error
	HandleTransactionSignedEvent(ctx context.Context, event *TransactionSignedEvent) error
	HandleTransactionEndorsedEvent(ctx context.Context, event *TransactionEndorsedEvent) error
	HandleTransactionDispatchedEvent(ctx context.Context, event *TransactionDispatchedEvent) error
	HandleTransactionConfirmedEvent(ctx context.Context, event *TransactionConfirmedEvent) error
	HandleTransactionRevertedEvent(ctx context.Context, event *TransactionRevertedEvent) error
	HandleTransactionDelegatedEvent(ctx context.Context, event *TransactionDelegatedEvent) error
	HandleResolveVerifierResponseEvent(ctx context.Context, event *ResolveVerifierResponseEvent) error
	HandleResolveVerifierErrorEvent(ctx context.Context, event *ResolveVerifierErrorEvent) error
	HandleTransactionFinalizedEvent(ctx context.Context, event *TransactionFinalizedEvent) error
	HandleTransactionFinalizeError(ctx context.Context, event *TransactionFinalizeError) error

	PrepareTransaction(ctx context.Context) (*components.PrivateTransaction, error)
	GetStateDistributions(ctx context.Context) []*statedistribution.StateDistribution
}
