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

package statestore

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/types"
)

// The DSI is the state interface that is exposed outside of the statestore package, for the
// transaction engine to use to safely query and update the state in the context of a particular
// domain.
//
// A single locked execution context is available per domain, per paladin runtime.
// In the future we may consider more granular execution contexts to increase parallelism.
//
// The locked execution context works only in-memory until explicitly committed, at which point
// all the operations queued up are flushed to the DB asynchronously.
//
// We can then continue to build the next set of flushable operations, while the first set is
// still flushing (a simple pipeline approach).
type DomainStateInterface interface {
	// EnsureABISchema is expected to be called on startup with all schemas required for operation of the domain
	EnsureABISchemas([]*abi.Parameter) ([]*Schema, error)
	// LockAvailableStates is the main query function, only returning states available to the current sequence,
	// and marking all states locked to the specified sequence.
	// Note this only locks the states for read - if they are being spent, then a subsequent MarkStatesSpending
	// call is required.
	LockAvailableStates(sequenceID uuid.UUID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier) (s []*State, err error)
	// WriteNewStates creates new states that are locked to the specified sequence from creation, and available
	// immediately to LockAvailableStates (even before Commit is called)
	WriteNewStates(sequenceID uuid.UUID, schemaID string, states []types.RawJSON) (s []*State, err error)
	// MarkStatesSpending updates the lock record to state that the state is now locked for spending, and
	// thus subsequent calls to LockAvailableStates will not return these states
	MarkStatesSpending(sequenceID uuid.UUID, schemaID string, states []types.RawJSON) (s []*State, err error)
	// DeleteSequence queues up removal of all lock records for a given sequence
	// Note that the private data of the states themselves are not removed
	ResetSequence(sequenceID uuid.UUID) error
	// Reset simply clears the un-flushed set (which happens automatically if the commit fails)
	Reset()
	// Flush moves the un-flushed set into flushing status, queueing to a DB writer to batch write
	// to the database.
	// Subsequent calls to the DMI will add to a new un-flushed set.
	// If there is already a commit in progress, this call will BLOCK until that commit has occurred.
	// Once no commit is in progress, this call will return and then invoke the callback (on the
	// DMI execution context) with the state now reset to before the last flush.
	Flush(cb func(ctx context.Context, dsi DomainStateInterface, err error))
}

type domainContext struct {
	ss        *stateStore
	ctx       context.Context
	domainID  string
	lock      sync.Mutex
	unFlushed *writeOperation
	flushing  *writeOperation
}

func (dc *domainContext) run(fn func(ctx context.Context, dsi DomainStateInterface) error) error {

}

func (dc *domainContext) EnsureABISchema() (*Schema, error) {

}

func (dc *domainContext) LockAvailableStates(sequenceID uuid.UUID, schemaID string, query *filters.QueryJSON, status StateStatusQualifier) (s []*State, err error) {

}

func (dc *domainContext) WriteNewStates(sequenceID uuid.UUID, schemaID string, states []types.RawJSON) (s []*State, err error) {

}

func (dc *domainContext) DeleteSequence(sequenceID uuid.UUID) error {

}

func (dc *domainContext) Rollback() error {

}

func (dc *domainContext) Commit() error {

}
