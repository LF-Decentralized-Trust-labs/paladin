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
	"encoding/json"
	"fmt"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/google/uuid"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type domainContext struct {
	context.Context

	id                 uuid.UUID
	ss                 *stateManager
	domainName         string
	customHashFunction bool
	contractAddress    pldtypes.EthAddress
	stateLock          sync.Mutex
	unFlushed          *pendingStateWrites
	flushing           *pendingStateWrites
	domainContexts     map[uuid.UUID]*domainContext
	closed             bool

	// We track creatingStates states beyond the flush - until the transaction that created them is removed, or a full reset
	// This is because the DB will never return them as "available"
	creatingStates map[string]*components.StateWithLabels

	// State locks are an in memory structure only, recording a set of locks associated with each transaction.
	// These are held only in memory, and used during DB queries to create a view on top of the database
	// that can make both additional states available, and remove visibility to states.
	txLocks []*pldapi.StateLock
}

// Very important that callers Close domain contexts they open
func (ss *stateManager) NewDomainContext(ctx context.Context, domain components.Domain, contractAddress pldtypes.EthAddress) components.DomainContext {
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

func (dc *domainContext) getUnFlushedSpends() (spending []pldtypes.HexBytes, nullifiers []*pldapi.StateNullifier, nullifierIDs []pldtypes.HexBytes, err error) {
	// Take lock and check flush state
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, nil, nil, flushErr
	}

	for _, l := range dc.txLocks {
		if l.Type.V() == pldapi.StateLockTypeSpend {
			spending = append(spending, l.StateID)
		}
	}
	nullifiers = append(nullifiers, dc.unFlushed.stateNullifiers...)
	if dc.flushing != nil {
		nullifiers = append(nullifiers, dc.flushing.stateNullifiers...)
	}
	nullifierIDs = make([]pldtypes.HexBytes, len(nullifiers))
	for i, nullifier := range nullifiers {
		nullifierIDs[i] = nullifier.ID
	}
	return spending, nullifiers, nullifierIDs, nil
}

func (dc *domainContext) mergeUnFlushedApplyLocks(schema components.Schema, dbStates []*pldapi.State, query *query.QueryJSON, excludeSpent, requireNullifier bool) (_ []*pldapi.State, err error) {
	log.L(dc).Debugf("domainContext:mergeUnFlushedApplyLocks dc.txLocks: %d creatingStates: %d", len(dc.txLocks), len(dc.creatingStates))
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, flushErr
	}

	retStates := dbStates
	matches, err := dc.mergeUnFlushed(schema, dbStates, query, excludeSpent, requireNullifier)
	if err != nil {
		return nil, err
	}
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

func (dc *domainContext) mergeUnFlushed(schema components.Schema, dbStates []*pldapi.State, query *query.QueryJSON, excludeSpent, requireNullifier bool) (_ []*components.StateWithLabels, err error) {

	// Get the list of new un-flushed states, which are not already locked for spend
	matches := make([]*components.StateWithLabels, 0, len(dc.creatingStates))
	schemaId := schema.Persisted().ID
	for _, state := range dc.creatingStates {
		if !state.Schema.Equals(&schemaId) {
			continue
		}
		if excludeSpent {
			spent := false
			for _, lock := range dc.txLocks {
				if lock.StateID.Equals(state.ID) && lock.Type.V() == pldapi.StateLockTypeSpend {
					spent = true
					break
				}
			}
			// Cannot return it if it's spent or locked for spending
			if spent {
				continue
			}
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

	return matches, nil
}

func (dc *domainContext) Info() components.DomainContextInfo {
	return components.DomainContextInfo{
		ID:              dc.id,
		DomainName:      dc.domainName,
		ContractAddress: dc.contractAddress,
	}
}

func (dc *domainContext) mergeInMemoryMatches(schema components.Schema, states []*pldapi.State, extras []*components.StateWithLabels, query *query.QueryJSON) (_ []*pldapi.State, err error) {

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
	retList := make([]*pldapi.State, len)
	for i := 0; i < len; i++ {
		retList[i] = fullList[i].State
	}
	return retList, nil

}

func (dc *domainContext) GetStatesByID(dbTX persistence.DBTX, schemaID pldtypes.Bytes32, ids []string) (components.Schema, []*pldapi.State, error) {
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}
	query := query.NewQueryBuilder().In(".id", idsAny).Sort(".created").Query()
	schema, matches, err := dc.ss.findStates(dc, dbTX, dc.domainName, &dc.contractAddress, schemaID, query, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAll,
	})
	if err == nil {
		var memMatches []*components.StateWithLabels
		memMatches, err = dc.mergeUnFlushed(schema, matches, query, false /* locked states are fine */, false /* nullifiers not required */)
		if err == nil && len(memMatches) > 0 {
			matches, err = dc.mergeInMemoryMatches(schema, matches, memMatches, query)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	return schema, matches, err
}

func (dc *domainContext) FindAvailableStates(dbTX persistence.DBTX, schemaID pldtypes.Bytes32, query *query.QueryJSON) (components.Schema, []*pldapi.State, error) {
	log.L(dc.Context).Debug("domainContext:FindAvailableStates")
	// Build a list of spending states
	spending, _, _, err := dc.getUnFlushedSpends()
	if err != nil {
		return nil, nil, err
	}

	// Run the query against the DB
	schema, states, err := dc.ss.findStates(dc, dbTX, dc.domainName, &dc.contractAddress, schemaID, query, &components.StateQueryOptions{
		StatusQualifier: pldapi.StateStatusAvailable,
		ExcludedIDs:     spending,
	})
	if err != nil {
		return nil, nil, err
	}
	log.L(dc.Context).Debugf("domainContext:FindAvailableStates read %d states from DB", len(states))

	// Merge in un-flushed states to results
	states, err = dc.mergeUnFlushedApplyLocks(schema, states, query, true /* exclude spent states */, false)
	log.L(dc.Context).Debugf("domainContext:FindAvailableStates mergeUnFlushedApplyLocks %d", len(states))

	return schema, states, err
}

func (dc *domainContext) FindAvailableNullifiers(dbTX persistence.DBTX, schemaID pldtypes.Bytes32, query *query.QueryJSON) (components.Schema, []*pldapi.State, error) {

	// Build a list of unflushed and spending nullifiers
	spending, nullifiers, nullifierIDs, err := dc.getUnFlushedSpends()
	if err != nil {
		return nil, nil, err
	}
	statesWithNullifiers := make([]pldtypes.HexBytes, len(nullifiers))
	for i, n := range nullifiers {
		statesWithNullifiers[i] = n.State
	}

	// Run the query against the DB
	schema, states, err := dc.ss.findNullifiers(dc, dbTX, dc.domainName, &dc.contractAddress, schemaID, query, pldapi.StateStatusAvailable, spending, nullifierIDs)
	if err != nil {
		return nil, nil, err
	}

	// Merge in un-flushed states to results
	states, err = dc.mergeUnFlushedApplyLocks(schema, states, query, true /* exclude spent states */, true)
	return schema, states, err
}

func (dc *domainContext) UpsertStates(dbTX persistence.DBTX, stateUpserts ...*components.StateUpsert) (states []*pldapi.State, err error) {
	return dc.upsertStates(dbTX, false, stateUpserts...)
}

func (dc *domainContext) upsertStates(dbTX persistence.DBTX, holdingLock bool, stateUpserts ...*components.StateUpsert) (states []*pldapi.State, err error) {

	states = make([]*pldapi.State, len(stateUpserts))
	stateLocks := make([]*pldapi.StateLock, 0, len(stateUpserts))
	withValues := make([]*components.StateWithLabels, len(stateUpserts))
	toMakeAvailable := make([]*components.StateWithLabels, 0, len(stateUpserts))
	for i, ns := range stateUpserts {
		schema, err := dc.ss.getSchemaByID(dc, dbTX, dc.domainName, ns.Schema, true)
		if err != nil {
			return nil, err
		}

		vs, err := schema.ProcessState(dc, &dc.contractAddress, ns.Data, ns.ID, dc.customHashFunction)
		if err != nil {
			return nil, err
		}
		withValues[i] = vs
		states[i] = withValues[i].State
		if ns.CreatedBy != nil {
			createLock := &pldapi.StateLock{
				Type:        pldapi.StateLockTypeCreate.Enum(),
				Transaction: *ns.CreatedBy,
				StateID:     withValues[i].ID,
			}
			stateLocks = append(stateLocks, createLock)
			toMakeAvailable = append(toMakeAvailable, vs)
			log.L(dc).Infof("Upserting state %s with create lock tx=%s", states[i].ID, ns.CreatedBy)
		} else {
			log.L(dc).Infof("Upserting state %s (no create lock)", states[i].ID)
		}
	}

	// Take lock and check flush state
	if !holdingLock {
		dc.stateLock.Lock()
		defer dc.stateLock.Unlock()
	}
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
		nullifier := &pldapi.StateNullifier{
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

func (dc *domainContext) addStateLocks(locks ...*pldapi.StateLock) error {
	for _, l := range locks {
		lockType, err := l.Type.Validate()
		if err != nil {
			return err
		}

		if l.Transaction == (uuid.UUID{}) {
			return i18n.NewError(dc, msgs.MsgStateLockNoTransaction)
		} else if len(l.StateID) == 0 {
			return i18n.NewError(dc, msgs.MsgStateLockNoState)
		}

		// For creating the state must be in our map (via Upsert) or we will fail to return it
		creatingState := dc.creatingStates[l.StateID.String()]
		if lockType == pldapi.StateLockTypeCreate && creatingState == nil {
			return i18n.NewError(dc, msgs.MsgStateLockCreateNotInContext, l.StateID)
		}

		// Note we do NOT check for conflicts on existing state locks
		log.L(dc).Debugf("state %s adding %s lock tx=%s)", l.StateID, lockType, l.Transaction)
		dc.txLocks = append(dc.txLocks, l)
	}
	return nil
}

func (dc *domainContext) applyLocks(states []*pldapi.State) []*pldapi.State {
	for _, s := range states {
		s.Locks = []*pldapi.StateLock{}
		for _, l := range dc.txLocks {
			if l.StateID.Equals(s.ID) {
				s.Locks = append(s.Locks, l)
			}
		}
	}
	return states
}

func (dc *domainContext) AddStateLocks(locks ...*pldapi.StateLock) (err error) {
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

	newLocks := make([]*pldapi.StateLock, 0)
	for _, lock := range dc.txLocks {
		skip := false
		for _, tx := range transactions {
			if lock.Transaction == tx {
				if lock.Type.V() == pldapi.StateLockTypeCreate {
					// Clean up the creating record
					delete(dc.creatingStates, lock.StateID.String())
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

func (dc *domainContext) StateLocksByTransaction() map[uuid.UUID][]pldapi.StateLock {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	txLocksCopy := make(map[uuid.UUID][]pldapi.StateLock)
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

func (dc *domainContext) Flush(dbTX persistence.DBTX) error {
	ctx := dc.Ctx()
	log.L(ctx).Infof("Flushing context domain=%s", dc.domainName)

	// We hold the lock while we are doing the synchronous part of flushing
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	if dc.flushing != nil {
		if dc.flushing.flushResult != nil {
			// we return the original error if the last flush error was not cleared
			return dc.flushing.flushResult
		}
		// It is an error if we are called a second time in this function before the callback
		// from the first call is completed/failed.
		return i18n.NewError(ctx, msgs.MsgStateFlushInProgress)
	}

	// Sync check if there's already an error
	// Ok we're good to go async
	dc.flushing = dc.unFlushed
	dc.unFlushed = nil

	// If there's nothing to do, return a nil result
	if dc.flushing == nil {
		log.L(ctx).Debugf("nothing pending to flush in domain context")
		return nil
	}

	// Need to make sure we clean up after ourselves if we fail synchronously
	var syncFlushError error
	defer func() {
		if syncFlushError != nil {
			dc.flushing.setError(syncFlushError)
		}
	}()
	syncFlushError = dc.flushing.exec(ctx, dbTX)
	if syncFlushError != nil {
		return syncFlushError
	}

	// Return a callback to the owner of the DB Transaction, so they can tell us if the commit succeeded
	dbTX.AddFinalizer(dc.finalizer)
	return nil
}

func (dc *domainContext) finalizer(ctx context.Context, commitError error) {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()

	if dc.flushing != nil && commitError != nil {
		// The error sits on the context until a Reset() is called
		dc.flushing.setError(commitError)
	} else {
		// We're ready for the next flush
		dc.flushing = nil
	}
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
				log.L(dc).Errorf("flush failed - domain context must be reset")
				return i18n.WrapError(dc, dc.flushing.flushResult, msgs.MsgStateFlushFailedDomainReset, dc.domainName, dc.contractAddress)
			}
		default:
		}
	}
	if dc.unFlushed == nil {
		dc.unFlushed = dc.newPendingStateWrites()
	}
	return nil
}

type exportSnapshot struct {
	States []*components.StateUpsert `json:"states"`
	Locks  []*exportableStateLock    `json:"locks"`
}

// pldapi.StateLocks do not include the stateID in the serialized JSON so we need to define a new struct to include it
type exportableStateLock struct {
	State       pldtypes.HexBytes                   `json:"stateId"`
	Transaction uuid.UUID                           `json:"transaction"`
	Type        pldtypes.Enum[pldapi.StateLockType] `json:"type"`
}

// Return a snapshot of all currently known state locks as serialized JSON
func (dc *domainContext) ExportSnapshot() ([]byte, error) {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return nil, flushErr
	}
	locks := make([]*exportableStateLock, 0, len(dc.txLocks))
	for _, l := range dc.txLocks {
		locks = append(locks, &exportableStateLock{
			State:       l.StateID,
			Transaction: l.Transaction,
			Type:        l.Type,
		})
	}
	states := make([]*components.StateUpsert, 0, len(dc.creatingStates))
	for _, s := range dc.creatingStates {
		states = append(states, &components.StateUpsert{
			ID:     s.ID,
			Schema: s.Schema,
			Data:   s.Data,
		})
	}
	return json.Marshal(&exportSnapshot{
		States: states,
		Locks:  locks,
	})
}

// ImportSnapshot is used to restore the state of the domain context, by adding a set of locks
func (dc *domainContext) ImportSnapshot(stateLocksJSON []byte) error {
	dc.stateLock.Lock()
	defer dc.stateLock.Unlock()
	if flushErr := dc.checkResetInitUnFlushed(); flushErr != nil {
		return flushErr
	}
	var snapshot exportSnapshot
	err := json.Unmarshal(stateLocksJSON, &snapshot)
	if err != nil {
		return i18n.WrapError(dc, err, msgs.MsgDomainContextImportInvalidJSON)
	}
	dc.creatingStates = make(map[string]*components.StateWithLabels)
	dc.txLocks = make([]*pldapi.StateLock, 0, len(snapshot.Locks))
	if _, err = dc.upsertStates(dc.ss.p.NOTX(), true /* already hold lock */, snapshot.States...); err != nil {
		return i18n.WrapError(dc, err, msgs.MsgDomainContextImportBadStates)
	}
	for _, l := range snapshot.Locks {
		dc.txLocks = append(dc.txLocks, &pldapi.StateLock{
			DomainName:  dc.domainName,
			StateID:     l.State,
			Transaction: l.Transaction,
			Type:        l.Type,
		})
		//if it transpires that any of the states we already know about are created by these transactions,
		// then we need to add them to the creatingStates map otherwise they will not be returned in queries
		if l.Type == pldapi.StateLockTypeCreate.Enum() {
			foundInUnflushed := false
			for _, state := range dc.unFlushed.states {
				if state.ID.String() == l.State.String() {
					dc.creatingStates[state.ID.String()] = state
					foundInUnflushed = true
				}
			}
			if !foundInUnflushed {
				// assuming this function is being used to copy a coordinators context to a delegate assembler's context
				// this this if branch could mean one of two things:
				// 1. the state distribution message hasn't' arrived yet but will arrive soon
				// 2. the state distribution message is never going to arrive because we are not on the distribution list
				// We can't tell the difference between these two cases so can't really fail here
				// It is up to the domain to ensure that they ask for the transaction to be `Park`ed temporarily if they suspect `1`
				log.L(dc).Infof("ImportSnapshot: state %s not found in unflushed states", l.State)
			}
		}
	}

	return nil
}
