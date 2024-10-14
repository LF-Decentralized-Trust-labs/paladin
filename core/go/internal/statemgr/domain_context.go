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
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type domainContext struct {
	context.Context

	id                 uuid.UUID
	ss                 *stateManager
	domainName         string
	customHashFunction bool
	contractAddress    tktypes.EthAddress
	stateLock          sync.Mutex
	unFlushed          *writeOperation
	flushing           *writeOperation
	domainContexts     map[uuid.UUID]*domainContext
	closed             bool

	// We track creatingStates states beyond the flush - until the transaction that created them is removed, or a full reset
	// This is because the DB will never return them as "available"
	creatingStates map[string]*components.StateWithLabels

	// State locks are an in memory structure only, recording a set of locks associated with each transaction.
	// These are held only in memory, and used during DB queries to create a view on top of the database
	// that can make both additional states available, and remove visibility to states.
	txLocks []*components.StateLock
}

// Very important that callers Close domain contexts they open
func (ss *stateManager) NewDomainContext(ctx context.Context, domain components.Domain, contractAddress tktypes.EthAddress) components.DomainContext {
	id := uuid.New()
	log.L(ctx).Debugf("Domain context %s for domain %s contract %s closed", id, domain.Name(), contractAddress)

	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	dc := &domainContext{
		Context:            log.WithLogField(ctx, "domain_ctx", fmt.Sprintf("%s_%s", domain.Name(), id)),
		id:                 id,
		ss:                 ss,
		domainName:         domain.Name(),
		customHashFunction: domain.CustomHashFunction(),
		contractAddress:    contractAddress,
		creatingStates:     make(map[string]*components.StateWithLabels),
		domainContexts:     make(map[uuid.UUID]*domainContext),
	}
	ss.domainContexts[id] = dc
	return dc
}

// nil if not found
func (ss *stateManager) GetDomainContext(ctx context.Context, id uuid.UUID) components.DomainContext {
	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	ret, found := ss.domainContexts[id]
	if found {
		return ret
	}
	return nil // means an actual nil value to the interface
}

func (ss *stateManager) ListDomainContexts() []components.DomainContextInfo {
	ss.domainContextLock.Lock()
	defer ss.domainContextLock.Unlock()

	dcs := make([]components.DomainContextInfo, 0, len(ss.domainContexts))
	for _, dc := range ss.domainContexts {
		dcs = append(dcs, dc.Info())
	}
	return dcs

}

func (dc *domainContext) Ctx() context.Context {
	return dc.Context
}

func (dc *domainContext) getUnFlushedSpends() (spending []tktypes.HexBytes, nullifiers []*components.StateNullifier, nullifierIDs []tktypes.HexBytes, err error) {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, nil, nil, flushErr
	}

	for _, l := range dc.txLocks {
		if l.Type.V() == components.StateLockTypeSpend {
			spending = append(spending, l.State)
		}
	}
	nullifiers = append(nullifiers, dc.unFlushed.stateNullifiers...)
	if dc.flushing != nil {
		nullifiers = append(nullifiers, dc.flushing.stateNullifiers...)
	}
	nullifierIDs = make([]tktypes.HexBytes, len(nullifiers))
	for i, nullifier := range nullifiers {
		nullifierIDs[i] = nullifier.ID
	}
	return spending, nullifiers, nullifierIDs, nil
}

func (dc *domainContext) mergeUnFlushedApplyLocks(schema components.Schema, dbStates []*components.State, query *query.QueryJSON, requireNullifier bool) (_ []*components.State, err error) {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, flushErr
	}

	// Get the list of new un-flushed states, which are not already locked for spend
	matches := make([]*components.StateWithLabels, 0, len(dc.creatingStates))
	schemaId := schema.Persisted().ID
	for _, state := range dc.creatingStates {
		if !state.Schema.Equals(&schemaId) {
			continue
		}
		spent := false
		for _, lock := range dc.txLocks {
			if lock.State.Equals(state.ID) && lock.Type.V() == components.StateLockTypeSpend {
				spent = true
				break
			}
		}
		// Cannot return it if it's spent or locked for spending
		if spent {
			continue
		}

		if requireNullifier && state.Nullifier == nil {
			continue
		}

		// Now we see if it matches the query
		labelSet := dc.ss.labelSetFor(schema)
		match, err := filters.EvalQuery(dc, query, labelSet, state.LabelValues)
		if err != nil {
			return nil, err
		}
		if match {
			dup := false
			for _, dbState := range dbStates {
				if dbState.ID.Equals(state.ID) {
					dup = true
					break
				}
			}
			if !dup {
				log.L(dc).Debugf("Matched state %s from un-flushed writes", &state.ID)
				// Take a shallow copy, as we'll apply the locks as they exist right now
				shallowCopy := *state
				matches = append(matches, &shallowCopy)
			}
		}
	}

	retStates := dbStates
	if len(matches) > 0 {
		// Build the merged list - this involves extra cost, as we deliberately don't reconstitute
		// the labels in JOIN on DB load (affecting every call at the DB side), instead we re-parse
		// them as we need them
		if retStates, err = dc.mergeInMemoryMatches(schema, dbStates, matches, query); err != nil {
			return nil, err
		}
	}

	return dc.applyLocks(retStates), nil
}

func (dc *domainContext) Info() components.DomainContextInfo {
	return components.DomainContextInfo{
		ID:              dc.id,
		DomainName:      dc.domainName,
		ContractAddress: dc.contractAddress,
	}
}

func (dc *domainContext) mergeInMemoryMatches(schema components.Schema, states []*components.State, extras []*components.StateWithLabels, query *query.QueryJSON) (_ []*components.State, err error) {

	// Reconstitute the labels for all the loaded states into the front of an aggregate list
	fullList := make([]*components.StateWithLabels, len(states), len(states)+len(extras))
	persistedStateIDs := make(map[string]bool)
	for i, s := range states {
		if fullList[i], err = schema.RecoverLabels(dc, s); err != nil {
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
	if err = filters.SortValueSetInPlace(dc, dc.ss.labelSetFor(schema), fullList, sortInstructions...); err != nil {
		return nil, err
	}

	// We only want the states (not the labels needed during sort),
	// and only up to the limit that might have been breached adding in our in-memory states
	len := len(fullList)
	if query.Limit != nil && len > *query.Limit {
		len = *query.Limit
	}
	retList := make([]*components.State, len)
	for i := 0; i < len; i++ {
		retList[i] = fullList[i].State
	}
	return retList, nil

}

func (dc *domainContext) FindAvailableStates(schemaID tktypes.Bytes32, query *query.QueryJSON) (components.Schema, []*components.State, error) {

	// Build a list of spending states
	spending, _, _, err := dc.getUnFlushedSpends()
	if err != nil {
		return nil, nil, err
	}

	// Run the query against the DB
	schema, states, err := dc.ss.findStates(dc, dc.domainName, dc.contractAddress, schemaID, query, StateStatusAvailable, spending...)
	if err != nil {
		return nil, nil, err
	}

	// Merge in un-flushed states to results
	states, err = dc.mergeUnFlushedApplyLocks(schema, states, query, false)
	return schema, states, err
}

func (dc *domainContext) FindAvailableNullifiers(schemaID tktypes.Bytes32, query *query.QueryJSON) (components.Schema, []*components.State, error) {

	// Build a list of unflushed and spending nullifiers
	spending, nullifiers, nullifierIDs, err := dc.getUnFlushedSpends()
	if err != nil {
		return nil, nil, err
	}
	statesWithNullifiers := make([]tktypes.HexBytes, len(nullifiers))
	for i, n := range nullifiers {
		statesWithNullifiers[i] = n.State
	}

	// Run the query against the DB
	schema, states, err := dc.ss.findAvailableNullifiers(dc, dc.domainName, dc.contractAddress, schemaID, query, spending, nullifierIDs)
	if err != nil {
		return nil, nil, err
	}

	// Merge in un-flushed states to results
	states, err = dc.mergeUnFlushedApplyLocks(schema, states, query, true)
	return schema, states, err
}

func (dc *domainContext) UpsertStates(stateUpserts ...*components.StateUpsert) (states []*components.State, err error) {

	states = make([]*components.State, len(stateUpserts))
	stateLocks := make([]*components.StateLock, 0, len(stateUpserts))
	withValues := make([]*components.StateWithLabels, len(stateUpserts))
	toMakeAvailable := make([]*components.StateWithLabels, 0, len(stateUpserts))
	for i, ns := range stateUpserts {
		schema, err := dc.ss.GetSchema(dc, dc.domainName, ns.SchemaID, nil, true)
		if err != nil {
			return nil, err
		}

		vs, err := schema.ProcessState(dc, dc.contractAddress, ns.Data, ns.ID, dc.customHashFunction)
		if err != nil {
			return nil, err
		}
		withValues[i] = vs
		states[i] = withValues[i].State
		if ns.CreatedBy != nil {
			createLock := &components.StateLock{
				Type:        components.StateLockTypeCreate.Enum(),
				Transaction: *ns.CreatedBy,
				State:       withValues[i].State.ID,
			}
			stateLocks = append(stateLocks, createLock)
			toMakeAvailable = append(toMakeAvailable, vs)
			log.L(dc).Infof("Upserting state %s with create lock tx=%s", states[i].ID, ns.CreatedBy)
		} else {
			log.L(dc).Infof("Upserting state %s (no create lock)", states[i].ID)
		}
	}

	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, flushErr
	}

	// Only those transactions with a creating TX lock can be returned from queries
	// (any other states supplied for flushing are just to ensure we have a copy of the state
	// for data availability when the existing/later confirm is available)
	for _, s := range toMakeAvailable {
		dc.creatingStates[s.ID.String()] = s
	}
	err = dc.addStateLocks(stateLocks...)
	if err != nil {
		return nil, err
	}

	// Add all the states to the flush that will go to the DB
	dc.unFlushed.states = append(dc.unFlushed.states, withValues...)
	return states, nil
}

func (dc *domainContext) UpsertNullifiers(nullifiers ...*components.NullifierUpsert) error {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return flushErr
	}

	for _, nullifierInput := range nullifiers {
		nullifier := &components.StateNullifier{
			DomainName: dc.domainName,
			ID:         nullifierInput.ID,
			State:      nullifierInput.State,
		}
		nullifier.DomainName = dc.domainName
		creatingState := dc.creatingStates[nullifier.State.String()]
		if creatingState == nil {
			return i18n.NewError(dc, msgs.MsgStateNullifierStateNotInCtx, nullifier.State, nullifier.ID)
		} else if creatingState.Nullifier != nil && !creatingState.Nullifier.ID.Equals(nullifier.ID) {
			return i18n.NewError(dc, msgs.MsgStateNullifierConflict, nullifier.State, creatingState.Nullifier.ID)
		}
		creatingState.Nullifier = nullifier
		dc.unFlushed.stateNullifiers = append(dc.unFlushed.stateNullifiers, nullifier)
	}

	return nil
}

func (dc *domainContext) addStateLocks(locks ...*components.StateLock) error {
	for _, l := range locks {
		lockType, err := l.Type.Validate()
		if err != nil {
			return err
		}

		if l.Transaction == (uuid.UUID{}) {
			return i18n.NewError(dc, msgs.MsgStateLockNoTransaction)
		} else if len(l.State) == 0 {
			return i18n.NewError(dc, msgs.MsgStateLockNoState)
		}

		// For creating the state must be in our map (via Upsert) or we will fail to return it
		creatingState := dc.creatingStates[l.State.String()]
		if lockType == components.StateLockTypeCreate && creatingState == nil {
			return i18n.NewError(dc, msgs.MsgStateLockCreateNotInContext, l.State)
		}

		// Note we do NOT check for conflicts on existing state locks
		log.L(dc).Debugf("state %s adding %s lock tx=%s)", l.State, lockType, l.Transaction)
		dc.txLocks = append(dc.txLocks, l)
	}
	return nil
}

func (dc *domainContext) applyLocks(states []*components.State) []*components.State {
	for _, s := range states {
		s.Locks = []*components.StateLock{}
		for _, l := range dc.txLocks {
			if l.State.Equals(s.ID) {
				s.Locks = append(s.Locks, l)
			}
		}
	}
	return states
}

func (dc *domainContext) AddStateLocks(locks ...*components.StateLock) (err error) {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return flushErr
	}

	return dc.addStateLocks(locks...)
}

// Clear all in-memory locks associated with individual transactions, because they are no longer needed/applicable
// Most likely because the state transitions have now been finalized.
//
// Note it's important that this occurs after the confirmation record of creation of a state is fully committed
// to the database, as the in-memory "creating" record for a state will be removed as part of this.
func (dc *domainContext) ResetTransactions(transactions ...uuid.UUID) {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	newLocks := make([]*components.StateLock, 0)
	for _, lock := range dc.txLocks {
		skip := false
		for _, tx := range transactions {
			if lock.Transaction == tx {
				if lock.Type.V() == components.StateLockTypeCreate {
					// Clean up the creating record
					delete(dc.creatingStates, lock.State.String())
				}
				skip = true
				break
			}
		}
		if !skip {
			newLocks = append(newLocks, lock)
		}
	}
	dc.txLocks = newLocks
}

func (dc *domainContext) StateLocksByTransaction() map[uuid.UUID][]components.StateLock {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	txLocksCopy := make(map[uuid.UUID][]components.StateLock)
	for _, l := range dc.txLocks {
		txLocksCopy[l.Transaction] = append(txLocksCopy[l.Transaction], *l)
	}
	return txLocksCopy
}

// Reset puts the world back to fresh - including completing any flush.
//
// Must be called after a flush error before the context can be used, as on a flush
// error the caller must reset their processing to the last point of consistency
// as they cannot trust in-memory state
//
// Note it does not cancel or check the status of any in-progress flush, as the
// things that are flushed are inert records in isolation.
// Reset instead is intended to be a boundary where the calling code knows explicitly
// that any state-locks and states that haven't reached a confirmed flush must
// be re-written into the DomainContext.
func (dc *domainContext) Reset() {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	err := dc.clearExistingFlush()
	if err != nil {
		log.L(dc).Warnf("Reset recovering from flush error: %s", err)
	}

	dc.creatingStates = make(map[string]*components.StateWithLabels)
	dc.flushing = nil
	dc.unFlushed = nil
	dc.txLocks = nil
}

func (dc *domainContext) Close() {
	dc.stateLock.Lock()
	dc.closed = true
	dc.stateLock.Unlock()

	log.L(dc).Debugf("Domain context %s for domain %s contract %s closed", dc.id, dc.domainName, dc.contractAddress)

	dc.ss.domainContextLock.Lock()
	defer dc.ss.domainContextLock.Unlock()
	delete(dc.ss.domainContexts, dc.id)
}

func (dc *domainContext) clearExistingFlush() error {
	// If we are already flushing, then we wait for that flush while holding the lock
	// here - until we can queue up the next flush.
	// e.g. we only get one flush ahead
	if dc.flushing != nil {
		select {
		case <-dc.flushing.flushed:
		case <-dc.Done():
			// The caller gave up on us, we cannot flush
			return i18n.NewError(dc, msgs.MsgContextCanceled)
		}
		return dc.flushing.flushResult
	}
	return nil
}

func (dc *domainContext) InitiateFlush(asyncCallback func(err error)) error {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	// Sync check if there's already an error
	if err := dc.clearExistingFlush(); err != nil {
		return err
	}

	// Ok we're good to go async
	flushing := dc.unFlushed
	dc.flushing = flushing
	dc.unFlushed = nil
	// Always dispatch a routine for the callback
	// even if there's a nil flushing - meaning nothing to do
	if flushing != nil {
		flushing.flushed = make(chan struct{})
		dc.ss.writer.queue(dc, flushing)
	}
	go dc.doFlush(asyncCallback, flushing)
	return nil
}

func (dc *domainContext) FlushSync() error {
	flushed := make(chan error)
	err := dc.InitiateFlush(func(err error) { flushed <- err })
	if err == nil {
		select {
		case err = <-flushed:
		case <-dc.Done():
			err = i18n.NewError(dc, msgs.MsgContextCanceled)
		}
	}
	return err
}

// MUST hold the lock to call this function
// Simply checks there isn't an un-cleared error that means the caller must reset.
func (dc *domainContext) checkResetInitUnFlushed() error {
	if dc.closed {
		return i18n.NewError(dc, msgs.MsgStateDomainContextClosed)
	}
	// Peek if there's a broken flush that needs a reset
	if dc.flushing != nil {
		select {
		case <-dc.flushing.flushed:
			if dc.flushing.flushResult != nil {
				log.L(dc).Errorf("flush %s failed - domain context must be reset", dc.flushing.id)
				return i18n.WrapError(dc, dc.flushing.flushResult, msgs.MsgStateFlushFailedDomainReset, dc.domainName, dc.contractAddress)

			}
		default:
		}
	}
	if dc.unFlushed == nil {
		dc.unFlushed = dc.ss.writer.newWriteOp(dc.domainName, dc.contractAddress)
	}
	return nil
}

// MUST NOT hold the lock to call this function - instead pass in a list of all the
// unflushed writers (max 2 in practice) that need to be successful for this flush to
// be considered complete
func (dc *domainContext) doFlush(cb func(error), flushing *writeOperation) {
	var err error
	// We might have found by the time we got the lock to flush, there was nothing to do
	if flushing != nil {
		log.L(dc).Debugf("waiting for flush %s", flushing.id)
		err = flushing.flush(dc)
		flushing.flushResult = err // for any other routines the blocked waiting
		log.L(dc).Debugf("flush %s completed (err=%v)", flushing.id, err)
		close(flushing.flushed)
	}
	cb(err)
}
