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
package sequencer

import (
	"context"

	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type endorsementRequirement struct {
	attRequest *prototk.AttestationRequest
	party      string
}

// Delegation represents a transaction that is being coordinated by a contract sequencer agent in Coordinator state.
type Delegation struct {
	sender string
	*components.PrivateTransaction
}

func NewDelegation(sender string, pt *components.PrivateTransaction) *Delegation {
	return &Delegation{
		sender:             sender,
		PrivateTransaction: pt,
	}
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
