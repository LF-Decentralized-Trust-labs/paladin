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

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

// TODO there is a lot of boilerplate code in lots of different places that is needed every time we add a new event or internode message exchange
// should consider refactoring the overrall code structure here
type PrivateTransactionEvent interface {
	GetTransactionID() string
	GetContractAddress() string
	SetContractAddress(string)
	Validate(ctx context.Context) error
}

type PrivateTransactionEventBase struct {
	TransactionID   string
	ContractAddress string
}

func (e *PrivateTransactionEventBase) Validate(ctx context.Context) error {
	return nil
}

func (e *PrivateTransactionEventBase) GetTransactionID() string {
	return e.TransactionID
}

func (e *PrivateTransactionEventBase) GetContractAddress() string {
	return e.ContractAddress
}

func (e *PrivateTransactionEventBase) SetContractAddress(contractAddress string) {
	e.ContractAddress = contractAddress
}

type TransactionSubmittedEvent struct {
	PrivateTransactionEventBase
}

// existing Transaction has been loaded into memory
type TransactionSwappedInEvent struct {
	PrivateTransactionEventBase
}

type DelegationForInFlightEvent struct {
	PrivateTransactionEventBase
	BlockHeight int64
}

type TransactionAssembledEvent struct {
	PrivateTransactionEventBase
	PostAssembly      *components.TransactionPostAssembly
	AssembleRequestID string
}

type TransactionAssembleFailedEvent struct {
	PrivateTransactionEventBase
	AssembleRequestID string
	Error             string
}

type TransactionSignedEvent struct {
	PrivateTransactionEventBase
	AttestationResult *prototk.AttestationResult
}

type TransactionEndorsedEvent struct {
	PrivateTransactionEventBase
	RevertReason           *string
	Endorsement            *prototk.AttestationResult
	Party                  string // In case Endorsement is nil, this is need to correlate with the attestation request
	AttestationRequestName string // In case Endorsement is nil, this is need to correlate with the attestation request
	IdempotencyKey         string
}

type TransactionDispatchedEvent struct {
	PrivateTransactionEventBase
	Nonce          uint64
	SigningAddress string
}

type TransactionPreparedEvent struct {
	PrivateTransactionEventBase
}

type TransactionConfirmedEvent struct {
	PrivateTransactionEventBase
}

type TransactionRevertedEvent struct {
	PrivateTransactionEventBase
}

type TransactionDelegationAcknowledgedEvent struct {
	PrivateTransactionEventBase
	DelegationRequestID string
}

type TransactionBlockedEvent struct {
	PrivateTransactionEventBase
}

type ResolveVerifierResponseEvent struct {
	PrivateTransactionEventBase
	Lookup       *string
	Algorithm    *string
	Verifier     *string
	VerifierType *string
}

type ResolveVerifierErrorEvent struct {
	PrivateTransactionEventBase
	Lookup       *string
	Algorithm    *string
	ErrorMessage *string
}

func (event *ResolveVerifierResponseEvent) Validate(ctx context.Context) error {

	// TODO why are these  pointers?  should they be strings? If we keep them as pointers, we need to add a validate() method
	if event.Lookup == nil {
		log.L(ctx).Error("Lookup is nil")
		//ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Lookup")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Lookup")
	}
	if event.Algorithm == nil {
		log.L(ctx).Error("Algorithm is nil")
		//ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Algorithm")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Algorithm")
	}
	if event.Verifier == nil {
		log.L(ctx).Error("Verifier is nil")
		//ts.latestError = i18n.ExpandWithCode(ctx, i18n.MessageKey(msgs.MsgPrivateTxManagerInvalidEventMissingField), "Verifier")
		return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInvalidEventMissingField, "Verifier")
	}
	return nil
}

type TransactionFinalizedEvent struct {
	PrivateTransactionEventBase
}

type TransactionNudgeEvent struct {
	//used to trigger the sequence to re-evaluate a transaction's state and next action
	//in lieu of a real event
	PrivateTransactionEventBase
}

type TransactionFinalizeError struct {
	PrivateTransactionEventBase
	RevertReason string // reason we were trying to finalize the transaction
	ErrorMessage string // reason the transaction could not be finalized
}

// Replies are correlated to the corresponding request and not necessarily to a specific transaction and/or contract
type PrivateTransactionReplyBase struct {
	RequestID string
}
type ResolveVerifierReply struct {
	PrivateTransactionReplyBase
	Lookup    *string
	Algorithm *string
	Verifier  *string
}

type ResolveVerifierError struct {
	PrivateTransactionReplyBase
	Lookup       *string
	Algorithm    *string
	ErrorMessage *string
}
