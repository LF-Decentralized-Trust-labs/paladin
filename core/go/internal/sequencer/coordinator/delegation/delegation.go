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
package delegation

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/sequencer/transport"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type TransactionState string

const (
	TransactionState_Pooled                TransactionState = "TransactionState_Pooled"
	TransactionState_Assembled             TransactionState = "TransactionState_Assembled"
	TransactionState_ConfirmingForDispatch TransactionState = "TransactionState_ConfirmingForDispatch"
	TransactionState_Dispatched            TransactionState = "TransactionState_Dispatched"
	TransactionState_Submitted             TransactionState = "TransactionState_Submitted"
	TransactionState_Committed             TransactionState = "TransactionState_Committed"
	TransactionState_Rejected              TransactionState = "TransactionState_Rejected"
	TransactionState_ConfirmedSuccess      TransactionState = "TransactionState_ConfirmedSuccess"
	TransactionState_ConfirmedReverted     TransactionState = "TransactionState_ConfirmedReverted"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

// Delegation represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
type Delegation struct {
	*components.PrivateTransaction
	sender            string
	dispatchConfirmed bool
	//SignerLocator        *string
	signerAddress        *tktypes.EthAddress
	latestSubmissionHash *tktypes.Bytes32
	nonce                *uint64
	stateMachine         *StateMachine
}

func NewDelegation(sender string, pt *components.PrivateTransaction) *Delegation {
	d := &Delegation{
		sender:             sender,
		PrivateTransaction: pt,
	}
	d.InitializeStateMachine(State_Pooled)
	return d
}

func (d *Delegation) GetSignerAddress() *tktypes.EthAddress {
	return d.signerAddress
}

func (d *Delegation) GetNonce() *uint64 {
	return d.nonce
}

func (d *Delegation) GetState() State {
	return d.stateMachine.currentState
}

func (d *Delegation) GetLatestSubmissionHash() *tktypes.Bytes32 {
	return d.latestSubmissionHash
}

// Hash method of Delegation
func (d *Delegation) Hash() ([]byte, error) {
	if d.PrivateTransaction == nil {
		//TODO should this be an error?
		return nil, nil
	}

	//find the attestation result that has the same name as the signature attestation

	signatureAttestationName, err := d.SignatureAttestationName()
	if err != nil {
		return nil, err
	}
	for _, endorsement := range d.PostAssembly.Endorsements {
		if endorsement.Name == signatureAttestationName {
			return endorsement.Payload, nil
		}
	}
	//TODO should this be an error?
	return nil, nil
}

// SignatureAttestationName is a method of Delegation that returns the name of the attestation in the attestation plan that is a signature
func (d *Delegation) SignatureAttestationName() (string, error) {
	for _, attRequest := range d.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_SIGN {
			return attRequest.Name, nil
		}
	}
	return "", nil
}

func (d *Delegation) handleEndorsementResponse(_ context.Context, response *transport.EndorsementResponse) error {
	//TODO check that this matches a pending request
	//TODO check that it is not a rejection

	d.PostAssembly.Endorsements = append(d.PostAssembly.Endorsements, response.Endorsement)

	return nil
}

func (d *Delegation) handleDispatchConfirmationResponse(_ context.Context, _ *transport.DispatchConfirmationResponse) error {
	//TODO check that this matches a pending request
	//TODO check that it is not a rejection

	d.dispatchConfirmed = true
	return nil
}

func (d *Delegation) Sender() string {
	return d.sender
}

func (d *Delegation) IsEndorsed(ctx context.Context) bool {
	return !d.hasOutstandingEndorsementRequests(ctx)
}

func (d *Delegation) OutputStateIDs(_ context.Context) []string {

	//We use the output states here not the OutputStatesPotential because it is not possible for another transaction
	// to spend a state unless it has been written to the state store and at that point we have the state ID
	outputStateIDs := make([]string, len(d.PostAssembly.OutputStates))
	for i, outputState := range d.PostAssembly.OutputStates {
		outputStateIDs[i] = outputState.ID.String()
	}
	return outputStateIDs
}

func (d *Delegation) InputStateIDs(_ context.Context) []string {

	inputStateIDs := make([]string, len(d.PostAssembly.InputStates))
	for i, inputState := range d.PostAssembly.InputStates {
		inputStateIDs[i] = inputState.ID.String()
	}
	return inputStateIDs
}

func (d *Delegation) Txn() *components.PrivateTransaction {
	return d.PrivateTransaction
}

func (d *Delegation) hasOutstandingEndorsementRequests(ctx context.Context) bool {
	return len(d.outstandingEndorsementRequests(ctx)) > 0
}

func (d *Delegation) outstandingEndorsementRequests(ctx context.Context) []*endorsementRequirement {
	outstandingEndorsementRequests := make([]*endorsementRequirement, 0)
	if d.PostAssembly == nil {
		log.L(ctx).Debugf("PostAssembly is nil so there are no outstanding endorsement requests")
		return outstandingEndorsementRequests
	}
	for _, attRequest := range d.PostAssembly.AttestationPlan {
		if attRequest.AttestationType == prototk.AttestationType_ENDORSE {
			for _, party := range attRequest.Parties {
				found := false
				for _, endorsement := range d.PostAssembly.Endorsements {
					found = endorsement.Name == attRequest.Name &&
						party == endorsement.Verifier.Lookup &&
						attRequest.VerifierType == endorsement.Verifier.VerifierType
					log.L(ctx).Infof("endorsement matched=%t: request[name=%s,party=%s,verifierType=%s] endorsement[name=%s,party=%s,verifierType=%s] verifier=%s",
						found,
						attRequest.Name, party, attRequest.VerifierType,
						endorsement.Name, endorsement.Verifier.Lookup, endorsement.Verifier.VerifierType,
						endorsement.Verifier.Verifier,
					)
					if found {
						break
					}
				}
				if !found {
					log.L(ctx).Debugf("endorsement request for %s outstanding for transaction %s", party, d.ID)
					outstandingEndorsementRequests = append(outstandingEndorsementRequests, &endorsementRequirement{party: party, attRequest: attRequest})
				}
			}
		}
	}
	return outstandingEndorsementRequests
}

func (d *Delegation) sendAssembleRequest(_ context.Context) error {
	return nil
}

func (d *Delegation) sendEndorsementRequests(_ context.Context) error {
	return nil
}
