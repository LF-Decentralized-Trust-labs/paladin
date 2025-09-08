// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemgr

import (
	"context"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"gorm.io/gorm/clause"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
)

// Each domain context can have up to two of these
// - one active
// - one flushing
// the rotation and is protected by the stateLock of the parent stateContext
type pendingStateWrites struct {
	dc          *domainContext
	flushResult error
	flushed     chan struct{}
	// States and state nullifiers can be flushed to persistence, although
	// are only available for consumption outside of a DomainContext
	// with creation locks once they are confirmed via the blockchain.
	states          []*components.StateWithLabels
	stateNullifiers []*pldapi.StateNullifier
}

func (dc *domainContext) newPendingStateWrites() *pendingStateWrites {
	return &pendingStateWrites{
		dc:      dc,
		flushed: make(chan struct{}),
	}
}

// must host the state lock when calling
func (op *pendingStateWrites) setError(err error) {
	if op.flushResult == nil {
		op.flushResult = err
		close(op.flushed)
	}
}

func (op *pendingStateWrites) exec(ctx context.Context, dbTX persistence.DBTX) error {

	// Build lists of things to insert (we are insert only)
	var states []*pldapi.State
	var stateLocks []*pldapi.StateLock
	var stateNullifiers []*pldapi.StateNullifier
	for _, s := range op.states {
		states = append(states, s.State)
	}
	if len(op.stateNullifiers) > 0 {
		stateNullifiers = append(stateNullifiers, op.stateNullifiers...)
	}
	log.L(ctx).Debugf("Writing state batch states=%d locks=%d nullifiers=%d ",
		len(states), len(stateLocks), len(stateNullifiers))

	var err error

	if len(states) > 0 {
		err = op.dc.ss.writeStates(ctx, dbTX, states)
	}

	if err == nil && len(stateNullifiers) > 0 {
		err = dbTX.DB().
			Table("state_nullifiers").
			Clauses(clause.OnConflict{
				DoNothing: true, // immutable
			}).
			Create(stateNullifiers).
			Error
	}
	// We don't actually provide any result, so just build an array of nil results
	return err
}
