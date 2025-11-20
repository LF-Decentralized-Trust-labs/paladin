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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/components"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/metrics"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/transport"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

type assembleRequestFromCoordinator struct {
	coordinatorsBlockHeight int64
	stateLocksJSON          []byte
	requestID               uuid.UUID
	preAssembly             []byte
}

// Transaction tracks the state of a transaction that is being sent by the local node in originator state.
type Transaction struct {
	stateMachine *StateMachine
	*components.PrivateTransaction
	engineIntegration                common.EngineIntegration
	transportWriter                  transport.TransportWriter
	eventHandler                     func(context.Context, common.Event) error
	currentDelegate                  string
	latestAssembleRequest            *assembleRequestFromCoordinator
	latestFulfilledAssembleRequestID uuid.UUID
	latestPreDispatchRequestID       uuid.UUID
	signerAddress                    *pldtypes.EthAddress
	latestSubmissionHash             *pldtypes.Bytes32
	nonce                            *uint64
	metrics                          metrics.DistributedSequencerMetrics
}

func NewTransaction(
	ctx context.Context,
	pt *components.PrivateTransaction,
	transportWriter transport.TransportWriter,
	eventHandler func(context.Context, common.Event) error,
	engineIntegration common.EngineIntegration,
	metrics metrics.DistributedSequencerMetrics,

) (*Transaction, error) {
	txn := &Transaction{
		PrivateTransaction: pt,
		engineIntegration:  engineIntegration,
		eventHandler:       eventHandler,
		transportWriter:    transportWriter,
		metrics:            metrics,
	}

	txn.InitializeStateMachine(State_Initial)

	return txn, nil
}

func (t *Transaction) Hash(ctx context.Context) (*pldtypes.Bytes32, error) {
	if t.PrivateTransaction == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot hash transaction without PrivateTransaction")
	}
	if t.PostAssembly == nil {
		return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, "cannot hash transaction without PostAssembly")
	}

	log.L(ctx).Debugf("hashing transaction %s with %d signatures and %d endorsements", t.ID.String(), len(t.PostAssembly.Signatures), len(t.PostAssembly.Endorsements))

	// MRW TODO MUST DO - it's not clear is a originator transaction hash if valid without any signatures or endorsements.
	// After assemble a Pente TX can have just the assembler's endorsement (not everyone else's), so comparing hashes with > 1 endorsements will fail
	// if len(t.PostAssembly.Signatures) == 0 {
	// 	return nil, i18n.NewError(ctx, msgs.MsgSequencerInternalError, " cannot hash transaction without at least one Signature")
	// }

	hash := sha3.NewLegacyKeccak256()
	for _, signature := range t.PostAssembly.Signatures {
		hash.Write(signature.Payload)
	}
	var h32 pldtypes.Bytes32
	_ = hash.Sum(h32[0:0])
	return &h32, nil

}

func (t *Transaction) GetEndorsementStatus(ctx context.Context) []components.PrivateTxEndorsementStatus {
	endorsementRequestStates := make([]components.PrivateTxEndorsementStatus, len(t.PostAssembly.AttestationPlan))
	for i, attRequest := range t.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				endorsementRequestState := &components.PrivateTxEndorsementStatus{Party: party, EndorsementReceived: false}
				for _, endorsement := range t.PostAssembly.Endorsements {
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

//TODO the following getter methods are not safe to call on anything other than the sequencer goroutine because they are reading data structures that are being modified by the state machine.
// We should consider making them safe to call from any goroutine by reading maintaining a copy of the data structures that are updated async from the sequencer thread under a mutex

func (t *Transaction) GetCurrentState() State {
	return t.stateMachine.currentState
}

func (t *Transaction) GetLatestEvent() string {
	return t.stateMachine.latestEvent
}

func (t *Transaction) GetSignerAddress() *pldtypes.EthAddress {
	return t.signerAddress
}

func (t *Transaction) GetLatestSubmissionHash() *pldtypes.Bytes32 {
	return t.latestSubmissionHash
}

func (t *Transaction) GetNonce() *uint64 {
	return t.nonce
}
