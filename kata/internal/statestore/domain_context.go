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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/filters"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

type DomainContextFunction func(ctx context.Context, dsi DomainStateInterface) error

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
	EnsureABISchemas(defs []*abi.Parameter) ([]Schema, error)

	// FindAvailableStates is the main query function, only returning states that are available.
	// Note this does not lock these states in any way, you must call that afterwards as:
	// 1) We don't know which will be selected as important by the domain - some might be un-used
	// 2) We deliberately return states that are locked to a transaction (but not spent yet) - which means the
	//    result of the any assemble that uses those states, will be a transaction that must
	//    be on the same transaction where those states are locked.
	FindAvailableStates(schemaID string, query *filters.QueryJSON) (s []*State, err error)

	// MarkStatesSpending writes a lock record so the state is now locked for spending, and
	// thus subsequent calls to FindAvailableStates will not return these states.
	MarkStatesSpending(transactionID uuid.UUID, stateIDs []string) error

	// MarkStatesRead writes a lock record so the state is now locked to this transaction
	// for reading - thus subsequent calls to FindAvailableStates will return these states
	// with the lock record attached.
	// That will inform them they need to join to this transaction if they wish to use those states.
	MarkStatesRead(transactionID uuid.UUID, stateIDs []string) error

	// UpsertStates creates or updates states.
	// They are available immediately within the domain for return in FindAvailableStates
	// on the domain (even before the flush).
	// If a non-nil transaction ID is supplied, then the states are mark locked to the specified
	// transaction. They can then be locked-for-creation, locked-for-spending, for simply
	// locked for existence to avoid other transactions spending them.
	UpsertStates(transactionID *uuid.UUID, states []*StateUpsert) (s []*State, err error)

	// ResetTransaction queues up removal of all lock records for a given transaction
	// Note that the private data of the states themselves are not removed
	ResetTransaction(transactionID uuid.UUID) error

	// Flush moves the un-flushed set into flushing status, queueing to a DB writer to batch write
	// to the database.
	// Subsequent calls to the DMI will add to a new un-flushed set.
	//
	// If there is already a flush in progress, this call will BLOCK until that flush has finished.
	//
	// The flush itself then happens asynchronously - any error will be reported back to the next call
	// that happens after the error occurs, and the unFlushed state will be cleared at that point.
	//
	// This reverts the domain back in the same way as a crash-restart.
	//
	// Callback is invoked ONLY on a successful flush, asynchronously on the Domain Context.
	// So if supplied, the caller must not rely on it being called, and must not block holding the
	// domain context until it is called.
	//
	// NOTE: For special cases where a domain callback requires a sync flush or error to complete processing,
	//       use RunInDomainContextFlush()
	Flush(successCallback ...DomainContextFunction) error
}

type domainContext struct {
	ss          *stateStore
	domainID    string
	ctx         context.Context
	stateLock   sync.Mutex
	latch       chan struct{}
	unFlushed   *writeOperation
	flushing    *writeOperation
	flushResult chan error
}

func (ss *stateStore) RunInDomainContext(domainID string, fn DomainContextFunction) error {
	return ss.getDomainContext(domainID).run(fn)
}

func (ss *stateStore) RunInDomainContextFlush(domainID string, fn DomainContextFunction) error {
	dc := ss.getDomainContext(domainID)
	err := dc.run(fn)
	if err == nil {
		err = dc.Flush()
	}
	if err == nil {
		err = dc.checkFlushCompletion(true)
	}
	return err
}

func (ss *stateStore) getDomainContext(domainID string) *domainContext {
	ss.domainLock.Lock()
	defer ss.domainLock.Unlock()

	dc := ss.domainContexts[domainID]
	if dc == nil {
		dc = &domainContext{
			ss:        ss,
			domainID:  domainID,
			ctx:       log.WithLogField(ss.bgCtx, "domain_context", domainID),
			latch:     make(chan struct{}, 1),
			unFlushed: ss.writer.newWriteOp(domainID),
		}
		ss.domainContexts[domainID] = dc
	}
	return dc
}

// The latch protects to ensure only a single routine executes against the DSI interface concurrently.
// We have no opinion in this module about how long-lived the function that holds the latch is -
// it can be a long lived go-routine, or short lived event handlers on lots of different go-routines
// (obviously a mixture of both would not work).
func (dc *domainContext) takeLatch() error {
	select {
	case <-dc.ctx.Done():
		return i18n.NewError(dc.ctx, msgs.MsgContextCanceled)
	case dc.latch <- struct{}{}:
		return nil
	}
}

func (dc *domainContext) returnLatch() {
	<-dc.latch
}

func (dc *domainContext) run(fn func(ctx context.Context, dsi DomainStateInterface) error) error {
	// Latch is held for entire function call, but the call happens on the caller's routine.
	// (state is locked separately)
	if err := dc.takeLatch(); err != nil {
		return err
	}
	defer dc.returnLatch()
	return fn(dc.ctx, dc)
}

func (dc *domainContext) EnsureABISchemas(defs []*abi.Parameter) ([]Schema, error) {

	// Validate all the schemas
	prepared := make([]Schema, len(defs))
	toFlush := make([]*SchemaPersisted, len(defs))
	for i, def := range defs {
		s, err := newABISchema(dc.ctx, dc.domainID, def)
		if err != nil {
			return nil, err
		}
		prepared[i] = s
		toFlush[i] = s.SchemaPersisted
	}

	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return nil, flushErr
	}

	// Add them to the unflushed state
	dc.unFlushed.schemas = append(dc.unFlushed.schemas, toFlush...)
	return prepared, nil
}

func (dc *domainContext) getUnFlushedSpending() ([]*idOnly, error) {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return nil, flushErr
	}

	var spendLocks []*idOnly
	for _, l := range dc.unFlushed.stateLocks {
		if l.Spending {
			spendLocks = append(spendLocks, &idOnly{ID: l.State})
		}
	}
	if dc.flushing != nil {
		for _, l := range dc.flushing.stateLocks {
			if l.Spending {
				spendLocks = append(spendLocks, &idOnly{ID: l.State})
			}
		}
	}
	return spendLocks, nil
}

func (dc *domainContext) mergedUnFlushed(schema Schema, states []*State, query *filters.QueryJSON) (_ []*State, err error) {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return nil, flushErr
	}

	// Get the list of new un-flushed states, which are not already locked for spend
	allUnFlushedStates := dc.unFlushed.states
	allUnFlushedStateLocks := dc.unFlushed.stateLocks
	if dc.flushing != nil {
		allUnFlushedStates = append(allUnFlushedStates, dc.flushing.states...)
		allUnFlushedStateLocks = append(allUnFlushedStateLocks, dc.flushing.stateLocks...)
	}
	matches := make([]*StateWithLabels, 0, len(dc.unFlushed.states))
	for _, s := range allUnFlushedStates {
		var locked *StateLock
		for _, l := range allUnFlushedStateLocks {
			if s.ID == l.State {
				locked = l
				break
			}
		}
		// Cannot return it if it's locked for spending already
		if locked != nil && locked.Spending {
			continue
		}
		// Now we see if it matches the query
		labelSet := dc.ss.labelSetFor(schema)
		match, err := query.Eval(dc.ctx, labelSet, s.LabelValues)
		if err != nil {
			return nil, err
		}
		if match {
			log.L(dc.ctx).Debugf("Matched state %s from un-flushed writes", &s.ID)
			matches = append(matches, s)
		}
	}

	if len(matches) > 0 {
		// Build the merged list - this involves extra cost, as we deliberately don't reconstitute
		// the labels in JOIN on DB load (affecting every call at the DB side), instead we re-parse
		// them as we need them
		return dc.mergeInMemoryMatches(schema, states, matches, query)
	}

	return states, nil
}

func (dc *domainContext) mergeInMemoryMatches(schema Schema, states []*State, extras []*StateWithLabels, query *filters.QueryJSON) (_ []*State, err error) {

	// Reconstitute the labels for all the loaded states into the front of an aggregate list
	fullList := make([]*StateWithLabels, len(states), len(states)+len(extras))
	persistedStateIDs := make(map[string]bool)
	for i, s := range states {
		if fullList[i], err = schema.RecoverLabels(dc.ctx, s); err != nil {
			return nil, err
		}
		persistedStateIDs[s.ID.String()] = true
	}

	// Copy the matches to the end of that same list
	// However, we can't be certain that some of the states that were in the flushing list, haven't made it
	// to the DB yet - so we do need to de-dup here.
	for _, s := range extras {
		if !persistedStateIDs[s.ID.String()] {
			fullList = append(fullList, s)
		}
	}

	// Sort it in place - note we ensure we always have a sort instruction on the DB
	sortInstructions := query.Sort
	if err = filters.SortValueSetInPlace(dc.ctx, dc.ss.labelSetFor(schema), fullList, sortInstructions...); err != nil {
		return nil, err
	}

	// We only want the states (not the labels needed during sort),
	// and only up to the limit that might have been breached adding in our in-memory states
	len := len(fullList)
	if query.Limit != nil && len > *query.Limit {
		len = *query.Limit
	}
	retList := make([]*State, len)
	for i := 0; i < len; i++ {
		retList[i] = fullList[i].State
	}
	return retList, nil

}

func (dc *domainContext) FindAvailableStates(schemaID string, query *filters.QueryJSON) (s []*State, err error) {

	// Build a list of excluded states
	excluded, err := dc.getUnFlushedSpending()
	if err != nil {
		return nil, err
	}

	// Run the query against the DB
	schema, states, err := dc.ss.findStates(dc.ctx, dc.domainID, schemaID, query, StateStatusAvailable, excluded...)
	if err != nil {
		return nil, err
	}

	// Merge in un-flushed states to results
	return dc.mergedUnFlushed(schema, states, query)
}

func (dc *domainContext) UpsertStates(transactionID *uuid.UUID, stateUpserts []*StateUpsert) (states []*State, err error) {

	states = make([]*State, len(stateUpserts))
	withValues := make([]*StateWithLabels, len(stateUpserts))
	for i, ns := range stateUpserts {
		schema, err := dc.ss.GetSchema(dc.ctx, dc.domainID, ns.SchemaID, true)
		if err != nil {
			return nil, err
		}

		withValues[i], err = schema.ProcessState(dc.ctx, ns.Data)
		if err != nil {
			return nil, err
		}
		states[i] = withValues[i].State
		if transactionID != nil {
			states[i].Locked = &StateLock{
				Transaction: *transactionID,
				State:       withValues[i].State.ID,
				Creating:    ns.Creating,
				Spending:    ns.Spending,
			}
			log.L(dc.ctx).Infof("Upserting state %s locked to tx=%s creating=%t spending=%t", states[i].ID, transactionID, states[i].Locked.Creating, states[i].Locked.Spending)
		} else {
			log.L(dc.ctx).Infof("Upserting state %s UNLOCKED", states[i].ID)
		}
	}

	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return nil, flushErr
	}

	// We need to de-duplicate out any previous un-flushed state writes of the same ID
	deDuppedUnFlushedStates := make([]*StateWithLabels, 0, len(dc.unFlushed.states))
	for _, existing := range dc.unFlushed.states {
		var replaced bool
		for _, s := range withValues {
			if existing.ID == s.ID {
				replaced = true
				break
			}
		}
		if !replaced {
			deDuppedUnFlushedStates = append(deDuppedUnFlushedStates, existing)
		}
	}
	// Now we can add our own un-flushed writes to the de-duplicated lists
	dc.unFlushed.states = append(deDuppedUnFlushedStates, withValues...)
	// Then add all the state locks if this is transaction flush (which need individual de-duping too)
	if transactionID != nil {
		for _, s := range withValues {
			_, err = dc.setUnFlushedLock(*transactionID, s.State.ID, func(sl *StateLock) {
				// Upsert semantics for states will replace any existing locks with the explicitly set locks in the upsert
				sl.Creating = s.Locked.Creating
				sl.Spending = s.Locked.Spending
			})
		}
	}

	return states, nil
}

func (dc *domainContext) lockStates(transactionID uuid.UUID, stateIDStrings []string, setLockState func(*StateLock)) (err error) {
	stateIDs := make([]types.Bytes32, len(stateIDStrings))
	for i, id := range stateIDStrings {
		stateIDs[i], err = types.ParseBytes32Ctx(dc.ctx, id)
		if err != nil {
			return err
		}
	}

	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return flushErr
	}

	// Update an existing un-flushed record, or add a new one.
	// Note we might fail on a clash (and the caller should then reset this transaction)
	for _, id := range stateIDs {
		if _, err := dc.setUnFlushedLock(transactionID, id, setLockState); err != nil {
			return err
		}
	}
	return nil
}

func (dc *domainContext) setUnFlushedLock(transactionID uuid.UUID, stateID types.Bytes32, setLockState func(*StateLock)) (*StateLock, error) {
	// Update an existing un-flushed record if one exists
	for _, l := range dc.unFlushed.stateLocks {
		if l.State == stateID {
			if l.Transaction != transactionID {
				// This represents a failure to call ResetTransaction() correctly
				return nil, i18n.NewError(dc.ctx, msgs.MsgStateLockConflictUnexpected, l.Transaction, transactionID)
			}
			setLockState(l)
			return l, nil
		}
	}
	// Otherwise create a new one
	l := &StateLock{State: stateID, Transaction: transactionID}
	dc.unFlushed.stateLocks = append(dc.unFlushed.stateLocks, l)
	setLockState(l)
	return l, nil
}

func (dc *domainContext) MarkStatesRead(transactionID uuid.UUID, stateIDs []string) (err error) {
	return dc.lockStates(transactionID, stateIDs, func(*StateLock) {})
}

func (dc *domainContext) MarkStatesSpending(transactionID uuid.UUID, stateIDs []string) (err error) {
	return dc.lockStates(transactionID, stateIDs, func(l *StateLock) { l.Spending = true })
}

func (dc *domainContext) ResetTransaction(transactionID uuid.UUID) error {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkFlushCompletion(false); flushErr != nil {
		return flushErr
	}

	// Remove anything un-flushed for this transaction, as we will delete everything instead
	newStateLocks := make([]*StateLock, 0, len(dc.unFlushed.stateLocks))
	for _, l := range dc.unFlushed.stateLocks {
		if l.Transaction != transactionID {
			newStateLocks = append(newStateLocks, l)
		}
	}
	dc.unFlushed.stateLocks = newStateLocks
	// Add the delete to be flushed
	dc.unFlushed.transactionLockDeletes = append(dc.unFlushed.transactionLockDeletes, transactionID)
	return nil
}

func (dc *domainContext) Flush(successCallbacks ...DomainContextFunction) error {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	// Note the implementation of this function ensures this is safe while holding the lock
	// we can't get to the point dc.flushing is non-nil, until we can be confident
	// dc.flushed is also non-nil and will be closed.
	//
	// This is a long-lived lock, but it only happens when we're double-flushing
	// (the previous flush didn't finish before the next is initiated)
	if flushErr := dc.checkFlushCompletion(true); flushErr != nil {
		return flushErr
	}

	// Cycle it out
	dc.flushing = dc.unFlushed
	dc.flushResult = make(chan error, 1)
	dc.unFlushed = dc.ss.writer.newWriteOp(dc.domainID)

	// We pass the vars directly to the routine, so the routine does not need the lock
	go dc.flushOp(dc.flushing, dc.flushResult, successCallbacks...)
	return nil
}

// flushOp MUST NOT take the stateLock
func (dc *domainContext) flushOp(op *writeOperation, flushed chan error, successCallbacks ...DomainContextFunction) {
	dc.ss.writer.queue(dc.ctx, op)
	err := op.flush(dc.ctx)
	flushed <- err
	if err == nil {
		for _, cb := range successCallbacks {
			callback := cb
			go func() { _ = dc.run(callback) }()
		}
	}
}

// checkFlushCompletion MUST be called holding the lock
func (dc *domainContext) checkFlushCompletion(block bool) error {
	if dc.flushing == nil {
		return nil
	}
	var flushErr error
	if block {
		flushErr = <-dc.flushResult
	} else {
		select {
		case flushErr = <-dc.flushResult:
		default:
			log.L(dc.ctx).Debugf("flush is still active")
			return nil
		}
	}
	// If we reached here, we've popped a flush result - clear the status
	dc.flushing = nil
	dc.flushResult = nil
	// If there was an error, we need to clean out the whole un-flushed state before we return it
	if flushErr != nil {
		dc.unFlushed = dc.ss.writer.newWriteOp(dc.domainID)
		return i18n.WrapError(dc.ctx, flushErr, msgs.MsgStateFlushFailedDomainReset, dc.domainID)
	}
	return nil
}
