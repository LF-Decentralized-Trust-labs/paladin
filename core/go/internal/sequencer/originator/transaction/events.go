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
	"time"

	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
)

type Event interface {
	common.Event
	GetTransactionID() uuid.UUID
}

type BaseEvent struct {
	common.BaseEvent
	TransactionID uuid.UUID
}

func (e *BaseEvent) GetTransactionID() uuid.UUID {
	return e.TransactionID
}

type ConfirmedSuccessEvent struct {
	BaseEvent
}

func (*ConfirmedSuccessEvent) Type() EventType {
	return Event_ConfirmedSuccess
}

func (*ConfirmedSuccessEvent) TypeString() string {
	return "Event_ConfirmedSuccess"
}

type ConfirmedRevertedEvent struct {
	BaseEvent
	RevertReason   pldtypes.HexBytes
	FailureMessage string
	WillRetry      bool
}

func (*ConfirmedRevertedEvent) Type() EventType {
	return Event_ConfirmedReverted
}

func (*ConfirmedRevertedEvent) TypeString() string {
	return "Event_ConfirmedReverted"
}

type CreatedEvent struct {
	BaseEvent
	PrivateTransaction *components.PrivateTransaction
}

func (*CreatedEvent) Type() EventType {
	return Event_Created
}

func (*CreatedEvent) TypeString() string {
	return "Event_Created"
}

type VerifiersResolvedEvent struct {
	BaseEvent
	ResolvedVerifiers []*prototk.ResolvedVerifier
}

func (*VerifiersResolvedEvent) Type() EventType {
	return Event_VerifiersResolved
}

func (*VerifiersResolvedEvent) TypeString() string {
	return "Event_VerifiersResolved"
}

type VerifierResolutionFailedEvent struct {
	BaseEvent
}

func (*VerifierResolutionFailedEvent) Type() EventType {
	return Event_VerifierResolutionFailed
}

func (*VerifierResolutionFailedEvent) TypeString() string {
	return "Event_VerifierResolutionFailed"
}

type VerifierResolutionRetryEvent struct {
	BaseEvent
}

func (*VerifierResolutionRetryEvent) Type() EventType {
	return Event_VerifierResolutionRetry
}

func (*VerifierResolutionRetryEvent) TypeString() string {
	return "Event_VerifierResolutionRetry"
}

type DelegatedEvent struct {
	BaseEvent
	Coordinator string
}

func (*DelegatedEvent) Type() EventType {
	return Event_Delegated
}

func (*DelegatedEvent) TypeString() string {
	return "Event_Delegated"
}

func (e *DelegatedEvent) GetCoordinator() string {
	return e.Coordinator
}

type AssembleRequestReceivedEvent struct {
	BaseEvent
	RequestID              uuid.UUID
	Coordinator            string
	CoordinatorBlockHeight int64
	BlockHeightTolerance   int64
	Expiry                 time.Time
}

func (*AssembleRequestReceivedEvent) Type() EventType {
	return Event_AssembleRequestReceived
}

func (*AssembleRequestReceivedEvent) TypeString() string {
	return "Event_AssembleRequestReceived"
}

type AssembleSuccessEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleSuccessEvent) Type() EventType {
	return Event_AssembleSuccess
}

func (*AssembleSuccessEvent) TypeString() string {
	return "Event_AssembleSuccess"
}

// SignSuccessEvent is queued by the background sign goroutine once it has signed all local SIGN
// attestations of the assembled plan. RequestID is the assemble-request-id the signatures correspond to.
type SignSuccessEvent struct {
	BaseEvent
	Results   []*prototk.AttestationResult
	RequestID uuid.UUID
}

func (*SignSuccessEvent) Type() EventType {
	return Event_SignSuccess
}

func (*SignSuccessEvent) TypeString() string {
	return "Event_SignSuccess"
}

// SignErrorEvent is queued by the background sign goroutine when signing a SIGN attestation failed.
type SignErrorEvent struct {
	BaseEvent
	RequestID uuid.UUID
}

func (*SignErrorEvent) Type() EventType {
	return Event_SignError
}

func (*SignErrorEvent) TypeString() string {
	return "Event_SignError"
}

type AssembleRevertEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleRevertEvent) Type() EventType {
	return Event_AssembleRevert
}

func (*AssembleRevertEvent) TypeString() string {
	return "Event_AssembleRevert"
}

type AssembleParkEvent struct {
	BaseEvent
	PostAssembly *components.TransactionPostAssembly
	RequestID    uuid.UUID
}

func (*AssembleParkEvent) Type() EventType {
	return Event_AssemblePark
}

func (*AssembleParkEvent) TypeString() string {
	return "Event_AssemblePark"
}

type AssembleErrorEvent struct {
	BaseEvent
	RequestID uuid.UUID
}

func (*AssembleErrorEvent) Type() EventType {
	return Event_AssembleError
}

func (*AssembleErrorEvent) TypeString() string {
	return "Event_AssembleError"
}

type PreDispatchRequestReceivedEvent struct {
	BaseEvent
	RequestID   uuid.UUID
	Coordinator string
}

func (*PreDispatchRequestReceivedEvent) Type() EventType {
	return Event_PreDispatchRequestReceived
}

func (*PreDispatchRequestReceivedEvent) TypeString() string {
	return "Event_PreDispatchRequestReceived"
}

// EventWithCoordinator is implemented by events that carry the coordinator node identity as their sender,
// enabling a common validator to gate processing based on the transaction's current delegate.
// GetCoordinator returns the node name populated from the transport layer's FromNode field which carries
// the sender's LocalNodeName().
type EventWithCoordinator interface {
	GetCoordinator() string
}

type DispatchedEvent struct {
	BaseEvent
	SignerAddress pldtypes.EthAddress
	Coordinator   string
}

func (*DispatchedEvent) Type() EventType {
	return Event_Dispatched
}

func (*DispatchedEvent) TypeString() string {
	return "Event_Dispatched"
}

func (e *DispatchedEvent) GetCoordinator() string {
	return e.Coordinator
}

type NonceAssignedEvent struct {
	BaseEvent
	SignerAddress pldtypes.EthAddress // include the signer address in case we never actually saw a dispatch event
	Nonce         uint64
	Coordinator   string
}

func (*NonceAssignedEvent) Type() EventType {
	return Event_NonceAssigned
}

func (*NonceAssignedEvent) TypeString() string {
	return "Event_NonceAssigned"
}

func (e *NonceAssignedEvent) GetCoordinator() string {
	return e.Coordinator
}

type SubmittedEvent struct {
	BaseEvent
	SignerAddress        pldtypes.EthAddress // include the signer address and nonce in case we never actually saw a dispatch event or nonce assigned event
	Nonce                uint64
	LatestSubmissionHash pldtypes.Bytes32
	Coordinator          string
}

func (*SubmittedEvent) Type() EventType {
	return Event_Submitted
}

func (*SubmittedEvent) TypeString() string {
	return "Event_Submitted"
}

func (e *SubmittedEvent) GetCoordinator() string {
	return e.Coordinator
}

type ResumedEvent struct {
	BaseEvent
}

func (*ResumedEvent) Type() EventType {
	return Event_Resumed
}

func (*ResumedEvent) TypeString() string {
	return "Event_Resumed"
}
