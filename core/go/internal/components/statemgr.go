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
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

type StateManager interface {
	ManagerLifecycle

	// Get a list of all active domain contexts
	ListDomainContexts() []DomainContextInfo

	// Create a new domain context - caller is responsible for closing it
	NewDomainContext(ctx context.Context, domain Domain, contractAddress tktypes.EthAddress, dbTX *gorm.DB) DomainContext

	// Get a previously created domain context
	GetDomainContext(ctx context.Context, id uuid.UUID) DomainContext

	// Ensure ABI schemas upserts all the specified schemas, using the given DB transaction
	EnsureABISchemas(ctx context.Context, dbTX *gorm.DB, domainName string, defs []*abi.Parameter) ([]Schema, error)

	// State finalizations are written on the DB context of the block indexer, by the domain manager.
	WriteStateFinalizations(ctx context.Context, dbTX *gorm.DB, spends []*StateSpend, confirms []*StateConfirm) (err error)

	// MUST NOT be called for states received over a network from another node.
	// Writes a batch of states that have been pre-verified BY THIS NODE so can bypass domain hash verification.
	WritePreVerifiedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*StateUpsertOutsideContext) ([]*State, error)

	// Write a batch of states that have been received over the network. ID hash calculation will be validated by the domain as prior to storage
	WriteReceivedStates(ctx context.Context, dbTX *gorm.DB, domainName string, states []*StateUpsertOutsideContext) ([]*State, error)

	// GetState returns a state by ID, with optional labels
	GetState(ctx context.Context, dbTX *gorm.DB, domainName string, contractAddress tktypes.EthAddress, stateID tktypes.HexBytes, failNotFound, withLabels bool) (*State, error)
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
	FindAvailableStates(schemaID tktypes.Bytes32, query *query.QueryJSON) (Schema, []*State, error)

	// FindAvailableNullifiers is similar to FindAvailableStates, but for domains that leverage
	// nullifiers to record spending.
	FindAvailableNullifiers(schemaID tktypes.Bytes32, query *query.QueryJSON) (Schema, []*State, error)

	// AddStateLocks updates the in-memory state of the domain context, to record a set of locks
	// that affect queries on available states and nullifiers.
	//
	// - Spend locks mark the states unavailable
	// - Create locks make un-confirmed states available for selection (also automatically added in UpsertStates with non-nil transaction)
	// - Read locks just mark the relationship for later processing
	//
	// This is an in-memory record that will be lost on Reset, and can be deleted using ClearTransaction
	AddStateLocks(locks ...*StateLock) (err error)

	// UpsertStates creates or updates states.
	// They are available immediately within the domain for return in FindAvailableStates
	// on the domain (even before the flush).
	// If a non-nil transaction ID is supplied, then the states are marked as being created by
	// the specified transaction using an in-memory lock.
	//
	// States will be written to the DB on the next flush (the associated lock is not)
	UpsertStates(states ...*StateUpsert) (s []*State, err error)

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
	StateLocksByTransaction() map[uuid.UUID][]StateLock

	// Reset restores the world to the current state of the database, clearing any errors
	// from failed flush, all un-flushed writes, and all in-memory state locks.
	// It does not wait for an in-progress flush to complete
	Reset()

	// Flush moves the un-flushed set into flushing status, queueing to a DB writer to batch write
	// to the database.
	//
	// We will wait for any previously initiated flush to complete.
	// Synchronous error will be returned if a previously initiated flush fails/failed
	// and Reset() has not yet been called.
	//
	// Then if any previous flush is cleared ok, we go asynchronous allowing the current goroutine to continue.
	//
	// The supplied callback will be called once all writes up to the point of this call have
	// been flushed to the database - for success or failure.
	InitiateFlush(cb func(error)) error

	// Convenience function for synchronous flush (mainly for testing)
	FlushSync() error

	// Removes the domain context from the state manager, and prevents any further use
	Close()
}

type State struct {
	ID              tktypes.HexBytes   `json:"id"                  gorm:"primaryKey"`
	Created         tktypes.Timestamp  `json:"created"             gorm:"autoCreateTime:nano"`
	DomainName      string             `json:"domain"              gorm:"primaryKey"`
	Schema          tktypes.Bytes32    `json:"schema"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            tktypes.RawJSON    `json:"data"`
	Labels          []*StateLabel      `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Int64Labels     []*StateInt64Label `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Confirmed       *StateConfirm      `json:"confirmed,omitempty" gorm:"foreignKey:state;references:id;"`
	Spent           *StateSpend        `json:"spent,omitempty"     gorm:"foreignKey:state;references:id;"`
	Locks           []*StateLock       `json:"locks,omitempty"     gorm:"-"` // in memory only processing here
	Nullifier       *StateNullifier    `json:"nullifier,omitempty" gorm:"foreignKey:state;references:id;"`
}

type StateUpsert struct {
	ID        tktypes.HexBytes
	SchemaID  tktypes.Bytes32
	Data      tktypes.RawJSON
	CreatedBy *uuid.UUID
}

type StateUpsertOutsideContext struct {
	ID              tktypes.HexBytes
	SchemaID        tktypes.Bytes32
	ContractAddress tktypes.EthAddress
	Data            tktypes.RawJSON
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*State
	LabelValues filters.ValueSet
}

func (s *StateWithLabels) ValueSet() filters.ValueSet {
	return s.LabelValues
}

type StateLabel struct {
	DomainName string           `gorm:"primaryKey"`
	State      tktypes.HexBytes `gorm:"primaryKey"`
	Label      string
	Value      string
}

type StateInt64Label struct {
	DomainName string           `gorm:"primaryKey"`
	State      tktypes.HexBytes `gorm:"primaryKey"`
	Label      string
	Value      int64
}

// State record can be updated before, during and after confirm records are written
// For example the confirmation of the existence of states will be coming all the time
// from the base ledger, for which we will never receive the private state itself.
// Immutable once written
type StateConfirm struct {
	DomainName  string           `json:"domain"       gorm:"primaryKey"`
	State       tktypes.HexBytes `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID        `json:"transaction"`
}

// State record can be updated before, during and after spend records are written
// Immutable once written
type StateSpend struct {
	DomainName  string           `json:"domain"       gorm:"primaryKey"`
	State       tktypes.HexBytes `json:"-"            gorm:"primaryKey"`
	Transaction uuid.UUID        `json:"transaction"`
}

type StateLockType string

const (
	StateLockTypeCreate StateLockType = "create"
	StateLockTypeRead   StateLockType = "read"
	StateLockTypeSpend  StateLockType = "spend"
)

func (tt StateLockType) Enum() tktypes.Enum[StateLockType] {
	return tktypes.Enum[StateLockType](tt)
}

func (tt StateLockType) Options() []string {
	return []string{
		string(StateLockTypeCreate),
		string(StateLockTypeRead),
		string(StateLockTypeSpend),
	}
}

// State locks record which transaction a state is being locked to, either
// spending a previously confirmed state, or an optimistic record of creating
// (and maybe later spending) a state that is yet to be confirmed.
type StateLock struct {
	DomainName  string                      `json:"domain"`
	State       tktypes.HexBytes            `json:"state,omitempty"`
	Transaction uuid.UUID                   `json:"transaction"`
	Type        tktypes.Enum[StateLockType] `json:"type"`
}

// State nullifiers are used when a domain chooses to use a separate identifier
// specifically for spending states (i.e. not the state ID).
// Domains that choose to leverage this architecture will create nullifier
// entries for all unspent states, and create a StateSpend entry for the
// nullifier (not for the state) when it is spent.
// Immutable once written
type StateNullifier struct {
	DomainName string           `json:"domain"          gorm:"primaryKey"`
	ID         tktypes.HexBytes `json:"id"              gorm:"primaryKey"`
	State      tktypes.HexBytes `json:"-"`
	Spent      *StateSpend      `json:"spent,omitempty" gorm:"foreignKey:state;references:id;"`
}

type NullifierUpsert struct {
	ID    tktypes.HexBytes `json:"id"              gorm:"primaryKey"`
	State tktypes.HexBytes `json:"-"`
}

type Schema interface {
	Type() SchemaType
	ID() tktypes.Bytes32
	Signature() string
	Persisted() *SchemaPersisted
	ProcessState(ctx context.Context, contractAddress tktypes.EthAddress, data tktypes.RawJSON, id tktypes.HexBytes, customHash bool) (*StateWithLabels, error)
	RecoverLabels(ctx context.Context, s *State) (*StateWithLabels, error)
}

type SchemaType string

const (
	// ABI schema uses the same semantics as events for defining indexed fields (must be top-level)
	SchemaTypeABI SchemaType = "abi"
)

type SchemaPersisted struct {
	ID         tktypes.Bytes32   `json:"id"          gorm:"primaryKey"`
	Created    tktypes.Timestamp `json:"created"     gorm:"autoCreateTime:false"` // we calculate the created time ourselves due to complex in-memory caching
	DomainName string            `json:"domain"`
	Type       SchemaType        `json:"type"`
	Signature  string            `json:"signature"`
	Definition tktypes.RawJSON   `json:"definition"`
	Labels     []string          `json:"labels"      gorm:"type:text[]; serializer:json"`
}
