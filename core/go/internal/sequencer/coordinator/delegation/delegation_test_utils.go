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
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type DelegationBuilderForTesting struct {
	t                    *testing.T
	id                   uuid.UUID
	sender               string
	dispatchConfirmed    bool
	signerAddress        *tktypes.EthAddress
	latestSubmissionHash *tktypes.Bytes32
	nonce                *uint64
	state                State
}

// Function NewDelegationBuilderForTesting creates a DelegationBuilderForTesting with random values for all fields
// use the builder methods to set specific values for fields before calling Build to create a new Delegation
func NewDelegationBuilderForTesting(t *testing.T, state State) *DelegationBuilderForTesting {
	builder := &DelegationBuilderForTesting{
		id:                   uuid.New(),
		sender:               uuid.NewString(),
		dispatchConfirmed:    false,
		signerAddress:        nil,
		latestSubmissionHash: nil,
		t:                    t,
		state:                state,
	}
	switch state {
	case State_Submitted:
		nonce := rand.Uint64()
		builder.nonce = &nonce
		builder.signerAddress = tktypes.RandAddress()
		latestSubmissionHash := tktypes.Bytes32(tktypes.RandBytes(32))
		builder.latestSubmissionHash = &latestSubmissionHash
	}
	return builder
}

func (b *DelegationBuilderForTesting) Build() *Delegation {
	privateTransaction := &components.PrivateTransaction{
		ID: b.id,
	}
	d := NewDelegation(b.sender, privateTransaction)
	d.dispatchConfirmed = b.dispatchConfirmed
	d.signerAddress = b.signerAddress
	d.latestSubmissionHash = b.latestSubmissionHash
	d.nonce = b.nonce
	d.stateMachine.currentState = b.state
	return d

}
