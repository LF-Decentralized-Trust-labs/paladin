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

package components

import (
	"context"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type StateManager interface {
	ManagerLifecycle

	// Get a list of all active domain contexts
	ListDomainContexts() []DomainContextInfo

	// Create a new domain context - caller is responsible for closing it
	NewDomainContext(ctx context.Context, domain Domain, contractAddress tktypes.EthAddress) DomainContext

	// Get a previously created domain context
	GetDomainContext(ctx context.Context, id uuid.UUID) DomainContext

	// Ensure ABI schemas upserts all the specified schemas, using the given DB transaction
	EnsureABISchemas(ctx context.Context, dbTX *gorm.DB, domainName string, defs []*abi.Parameter) ([]Schema, error)

	// State finalizations are written on the DB context of the block indexer, by the domain manager.
	WriteStateFinalizations(ctx context.Context, dbTX *gorm.DB, spends []*pldapi.StateSpendRecord, reads []*pldapi.StateReadRecord, confirms []*pldapi.StateConfirmRecord, infoRecords []*pldapi.StateInfoRecord) (err error)

	// MUST NOT be called for states received over a network from another node.
	// Writes a batch of states that have been pre-verified BY THIS NODE so can bypass domain hash verification.
	WritePreVerifiedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*StateUpsertOutsideContext) ([]*pldapi.State, error)

	// Write a batch of states that have been received over the network. ID hash calculation will be validated by the domain as prior to storage
	WriteReceivedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*StateUpsertOutsideContext) ([]*pldapi.State, error)

	// Write a batch of nullifiers that correspond to states just received
	WriteNullifiersForReceivedStates(ctx context.Context, dbTX *gorm.DB, domainName string, nullifiers []*NullifierUpsert) error

	// GetState returns a state by ID, with optional labels
	GetState(ctx context.Context, dbTX *gorm.DB, domainName string, contractAddress tktypes.EthAddress, stateID tktypes.HexBytes, failNotFound, withLabels bool) (*pldapi.State, error)

	// Get all states created, read or spent by a confirmed transaction
	GetTransactionStates(ctx context.Context, dbTX *gorm.DB, txID uuid.UUID) (*pldapi.TransactionStates, error)
}

type DomainContextInfo struct {
	ID              uuid.UUID          `json:"id"`
	DomainName      string             `json:"domain"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
}

// The DSI is the state interface that is exposed outside of the statestore package, for the
// transaction engine to use to safely query and update the state in the context of a particular
// domain.
//
// A single locked execution context is available per domain private contract, per paladin runtime.
// In the future we may consider more granular execution contexts to increase parallelism.
//
// The locked execution context works only in-memory until explicitly committed, at which point
// all the operations queued up are flushed to the DB asynchronously.
//
// We can then continue to build the next set of flushable operations, while the first set is
// still flushing (a simple pipeline approach).
type DomainContext interface {
	Ctx() context.Context // easier to mock than embedding the context.Context interface

	// Get the ID, domain and address of this domain context
	Info() DomainContextInfo

	// FindAvailableStates is the main query function, only returning states that are available.
	// Note this does not lock these states in any way, you must call that afterwards as:
	// 1) We don't know which will be selected as important by the domain - some might be un-used
	// 2) We deliberately return states that are locked to a transaction (but not spent yet) - which means the
	//    result of the any assemble that uses those states, will be a transaction that must
	//    be on the same transaction where those states are locked.
	//
	// The dbTX is passed in to allow re-use of a connection during read operations.
	FindAvailableStates(dbTX *gorm.DB, schemaID tktypes.Bytes32, query *query.QueryJSON) (Schema, []*pldapi.State, error)

	// Return a snapshot of all currently known state locks
	ExportSnapshot() ([]byte, error)

	// ImportSnapshot is used to restore the state of the domain context, by adding a set of locks
	ImportSnapshot([]byte) error

	// FindAvailableNullifiers is similar to FindAvailableStates, but for domains that leverage
	// nullifiers to record spending.
	//
	// The dbTX is passed in to allow re-use of a connection during read operations.
	FindAvailableNullifiers(dbTX *gorm.DB, schemaID tktypes.Bytes32, query *query.QueryJSON) (Schema, []*pldapi.State, error)

	// AddStateLocks updates the in-memory state of the domain context, to record a set of locks
	// that affect queries on available states and nullifiers.
	//
	// - Spend locks mark the states unavailable
	// - Create locks make un-confirmed states available for selection (also automatically added in UpsertStates with non-nil transaction)
	// - Read locks just mark the relationship for later processing
	//
	// This is an in-memory record that will be lost on Reset, and can be deleted using ClearTransaction
	AddStateLocks(locks ...*pldapi.StateLock) (err error)

	// UpsertStates creates or updates states.
	// They are available immediately within the domain for return in FindAvailableStates
	// on the domain (even before the flush).
	// If a non-nil transaction ID is supplied, then the states are marked as being created by
	// the specified transaction using an in-memory lock.
	//
	// States will be written to the DB on the next flush (the associated lock is not)
	// The dbTX is passed in to allow re-use of a connection during read operations.
	UpsertStates(dbTX *gorm.DB, states ...*StateUpsert) (s []*pldapi.State, err error)

	// UpsertNullifiers creates nullifier records associated with states.
	// Nullifiers are an alternate state identifier (separate from the state ID) that can be used
	// when recording spent states.
	//
	// Nullifiers will be written to the DB on the next flush
	UpsertNullifiers(nullifiers ...*NullifierUpsert) error

	// Call this to remove all locks associated with individual transactions without clearing the whole state.
	// For example if a notification has been received that the transaction is either confirmed, or rejected.
	//
	// This only affects in memory state.
	//
	// No dependency analysis is done by this function call - that is the responsibility of the caller.
	ResetTransactions(transactionID ...uuid.UUID)

	// Return a complete copy of the current set of locks being managed in this context
	// Mainly for debugging (lots of memory is copied) so any case this function is used on a critical path
	// should be considered as a requirement for a new function on this interface that can be performed
	// safely under the mutex of the domain context.
	StateLocksByTransaction() map[uuid.UUID][]pldapi.StateLock

	// Reset restores the world to the current state of the database, clearing any errors
	// from failed flush, all un-flushed writes, and all in-memory state locks.
	// It does not wait for an in-progress flush to complete
	Reset()

	// Flush moves the un-flushed set into flushing status, queueing to a DB writer to batch write
	// to the database.
	//
	// The domain context needs to know when the flush has completed for success or failure, as it needs to
	// clear the flushing state from the in-memory context. This can only be done after the DB transaction
	// commits, as only then is it assured that the states will be returned by the DB and do not need
	// to be held in memory any longer. So the returned callback function must be called on commit OR ROLLBACK
	// of the database transaction.
	//
	// If an error is returned by this function, then the postDBTx callback will be nil
	Flush(dbTX *gorm.DB) (postDBTx func(error), err error)

	// Removes the domain context from the state manager, and prevents any further use
	Close()
}

type StateUpsert struct {
	ID        tktypes.HexBytes `json:"id"`
	Schema    tktypes.Bytes32  `json:"schema"`
	Data      tktypes.RawJSON  `json:"data"`
	CreatedBy *uuid.UUID       `json:"createdBy,omitempty"` // not exported
}

type StateUpsertOutsideContext struct {
	ID              tktypes.HexBytes
	SchemaID        tktypes.Bytes32
	ContractAddress tktypes.EthAddress
	Data            tktypes.RawJSON
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*pldapi.State
	LabelValues filters.ValueSet
}

func (s *StateWithLabels) ValueSet() filters.ValueSet {
	return s.LabelValues
}

type NullifierUpsert struct {
	ID    tktypes.HexBytes `json:"id"              gorm:"primaryKey"`
	State tktypes.HexBytes `json:"-"`
}

type Schema interface {
	Type() pldapi.SchemaType
	ID() tktypes.Bytes32
	Signature() string
	Persisted() *pldapi.Schema
	ProcessState(ctx context.Context, contractAddress tktypes.EthAddress, data tktypes.RawJSON, id tktypes.HexBytes, customHash bool) (*StateWithLabels, error)
	RecoverLabels(ctx context.Context, s *pldapi.State) (*StateWithLabels, error)
}
