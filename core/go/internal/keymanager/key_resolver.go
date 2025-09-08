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

package keymanager

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"gorm.io/gorm/clause"
)

type resolvedDBPath struct {
	segment   string
	index     int64
	path      string
	nextIndex *int64
	parent    *resolvedDBPath
}

type keyResolverDBTXKey struct{}

type keyResolver struct {
	km                  *keyManager
	dbTX                persistence.DBTX
	id                  string
	l                   sync.Mutex
	rootPath            *resolvedDBPath
	resolvedPaths       map[string]*resolvedDBPath
	allocationLockTaken bool
	newMappings         []*pldapi.KeyMappingWithPath
	newVerifiers        []*pldapi.KeyVerifierWithKeyRef
	done                chan struct{}
}

func (km *keyManager) KeyResolverForDBTX(dbTX persistence.DBTX) components.KeyResolver {
	// We rely on there only being one key resolver in each DB transaction
	iKrc := dbTX.Singleton(keyResolverDBTXKey{}, func(txCtx context.Context) any {
		return km.newKeyResolver(dbTX, true)
	})
	return iKrc.(components.KeyResolver)
}

func (km *keyManager) newKeyResolver(dbTX persistence.DBTX, registerCBs bool) components.KeyResolver {
	kr := &keyResolver{
		km:            km,
		dbTX:          dbTX,
		id:            pldtypes.ShortID(),
		rootPath:      &resolvedDBPath{},
		resolvedPaths: make(map[string]*resolvedDBPath),
		done:          make(chan struct{}),
	}
	if registerCBs {
		dbTX.AddPreCommit(kr.preCommit)
		dbTX.AddFinalizer(kr.finalizer)
	}
	return kr
}

func (kr *keyResolver) getOrCreateIdentifierPath(ctx context.Context, identifier string, allowCreate bool) (resolved *resolvedDBPath, err error) {

	// We split it into segments by "." and create-or-update the index at each level
	segments := append([]string{"" /* root path */}, strings.Split(identifier, ".")...)
	parent := kr.rootPath
	for _, segment := range segments {
		if parent != kr.rootPath && len(segment) == 0 {
			return nil, i18n.NewError(ctx, msgs.MsgKeyManagerInvalidIdentifier, identifier)
		}
		resolved, err = kr.resolvePathSegment(ctx, parent, segment, allowCreate)
		if err != nil {
			return nil, err
		}
		parent = resolved
	}
	return resolved, nil
}

func (kr *keyResolver) resolvePathSegment(ctx context.Context, parent *resolvedDBPath, segment string, allowCreate bool) (*resolvedDBPath, error) {

	db := kr.dbTX.DB()

	path := segment
	if parent.path != "" {
		path = fmt.Sprintf("%s.%s", parent.path, segment)
	}

	// We might have resolved this before in our context, in which case we use that
	// Note the empty string is the root path here
	if resolved := kr.resolvedPaths[path]; resolved != nil {
		return resolved, nil
	}

	for {
		// Check for an existing entry in the DB
		var pathList []*DBKeyPath
		err := db.WithContext(ctx).
			Where("path = ?", path).
			Find(&pathList).Error
		if err != nil {
			return nil, err
		}
		if len(pathList) > 0 {
			resolved := &resolvedDBPath{segment: segment, index: pathList[0].Index, path: path, parent: parent}
			kr.resolvedPaths[path] = resolved
			return resolved, nil
		} else if !allowCreate {
			// In reverse lookup we get called in read-only mode
			return nil, i18n.NewError(ctx, msgs.MsgKeyManagerIdentifierPathNotFound, path)
		}

		// Note: This is a single course grain lock on allocation, for the reasons described in TestE2ESigningHDWalletRealDB
		// If performance shows DB locks coupling to new key allocation is a bottleneck, then the proposed solution would be:
		// - Create a resolve-intent structure that is used before opening the DB transaction to list all identifiers that will be resolved
		// - When we get to this point the lock we take is at the level of the tree that is the highest common root of all intents
		// - We reject any attempt to allocate an identifier not in the intent list
		if !kr.allocationLockTaken {
			if err := kr.km.takeAllocationLock(ctx, kr); err != nil {
				return nil, err // context cancelled while waiting
			}
			kr.allocationLockTaken = true
		}

		// Find or create in the DB
		var dbPath *DBKeyPath

		nextIndex := int64(0)
		if parent.nextIndex == nil {
			// Get the highest index on the parent so far written to the DB
			err = db.WithContext(ctx).
				Where("parent = ?", parent.path).
				Order(`"index" DESC`).
				Limit(1).
				Find(&pathList).
				Error
			if err != nil {
				return nil, err
			}
			if len(pathList) > 0 {
				nextIndex = pathList[0].Index + 1
			}
		}

		// We think we've got ourselves a nice new index - try to allocate it
		dbPath = &DBKeyPath{
			Parent: parent.path,
			Path:   path,
			Index:  nextIndex,
		}

		// We might get a conflict because we did a dirty read before we took the lock.
		log.L(ctx).Infof("allocating index %d on parent %s to key-path %s", nextIndex, parent.path, path)
		result := db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(dbPath)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			// note this is not an optimized path - lots of thread clashing to create the same
			// key concurrently. Separate re-use of lots of keys is the more optimized path.
			log.L(ctx).Infof("re-checking with lock after losing optimistic race: %s", err)
			parent.nextIndex = nil
			continue
		}

		// Ok - we took the index - increment in the parent entry as we won't query again
		nextIndex++
		parent.nextIndex = &nextIndex

		// Store the resolved path, and return
		resolved := &resolvedDBPath{segment: segment, index: dbPath.Index, path: path, parent: parent}
		kr.resolvedPaths[path] = resolved

		return resolved, nil
	}
}

func (kr *keyResolver) getStoredVerifier(ctx context.Context, identifier, algorithm, verifierType string) (*pldapi.KeyVerifier, error) {
	vKey := verifierForwardCacheKey(identifier, algorithm, verifierType)
	verifier, _ := kr.km.verifierByIdentityCache.Get(vKey)
	if verifier != nil {
		return verifier, nil
	}
	var verifiers []*DBKeyVerifier
	err := kr.dbTX.DB().WithContext(ctx).
		Where(`"identifier" = ?`, identifier).
		Where(`"algorithm" = ?`, algorithm).
		Where(`"type" = ?`, verifierType).
		Limit(1).
		Find(&verifiers).
		Error
	if err != nil {
		return nil, err
	}
	if len(verifiers) == 0 {
		return nil, nil
	}
	verifier = &pldapi.KeyVerifier{
		Algorithm: verifiers[0].Algorithm,
		Type:      verifiers[0].Type,
		Verifier:  verifiers[0].Verifier,
	}
	kr.km.verifierByIdentityCache.Set(vKey, verifier)
	return verifier, nil
}

func (kr *keyResolver) ResolveKey(ctx context.Context, identifier, algorithm, verifierType string) (_ *pldapi.KeyMappingAndVerifier, err error) {
	return kr.resolveKey(ctx, identifier, algorithm, verifierType, false /* allow creation */)
}

func (kr *keyResolver) resolveKey(ctx context.Context, identifier, algorithm, verifierType string, requireExistingMapping bool) (_ *pldapi.KeyMappingAndVerifier, err error) {
	kr.l.Lock()
	defer kr.l.Unlock()

	// Identifier must be a valid
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, identifier, pldtypes.DefaultNameMaxLen, "identifier"); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgKeyManagerInvalidIdentifier, identifier)
	}

	// Now check the mappings we've already generated in this context
	var mapping *pldapi.KeyMappingWithPath
	for _, m := range kr.newMappings {
		if m.Identifier == identifier {
			mapping = m
		}
	}

	if mapping == nil {
		// Next use the cache to resolve it - if this is good, then we don't need to do anything
		// persistent in this key resolution context, or block anyone else.
		mapping, _ = kr.km.identifierCache.Get(identifier)
	}

	var isNewMapping = false
	var dbPath *resolvedDBPath
	db := kr.dbTX.DB()
	if mapping == nil {
		// We go look up the hierarchical path of this identifier, to see if it's existing or new.
		// Lots of optimistic locking complexity inside this function to efficiently race threads to ensure one wins
		// each index allocation race.
		dbPath, err = kr.getOrCreateIdentifierPath(ctx, identifier, !requireExistingMapping)
		if err != nil {
			return nil, err
		}

		// NOTE: If we fail after this point, but for some reason don't rollback the DB transaction it is possible
		// for us to have an un-bound path in our DB table. This is considered fine - we'll pick the same index
		// later if the path gets resolved again, and all code that relies on indexes MUST accept gaps are a possibility.

		// Now we've worked out an existing or new path to assign to the key, but we don't actually
		// know if the key has already been allocated (it's possible previously this entry was just a path even
		// if it already existed) ... so do a query.
		var mappings []*DBKeyMapping
		err = db.WithContext(ctx).
			Where(`"identifier" = ?`, identifier).
			Limit(1).
			Find(&mappings).
			Error
		if err != nil {
			return nil, err
		}

		// Now we know if we're creating a new DB, or we have an existing one
		if len(mappings) > 0 {
			mapping = &pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{
					Identifier: mappings[0].Identifier,
					Wallet:     mappings[0].Wallet,
					KeyHandle:  mappings[0].KeyHandle,
				},
				Path: dbPath.pathSegments(),
			}
		} else {
			if requireExistingMapping {
				return nil, i18n.NewError(ctx, msgs.MsgKeyManagerExistingIdentifierNotFound, identifier)
			}
			isNewMapping = true
			mapping = &pldapi.KeyMappingWithPath{
				KeyMapping: &pldapi.KeyMapping{
					Identifier: identifier,
				},
				Path: dbPath.pathSegments(),
			}
		}
	}

	return kr.resolveMapping(ctx, mapping, isNewMapping, identifier, algorithm, verifierType, requireExistingMapping)
}

func (kr *keyResolver) resolveMapping(ctx context.Context, mapping *pldapi.KeyMappingWithPath, isNewMapping bool, identifier, algorithm, verifierType string, requireExistingMapping bool) (_ *pldapi.KeyMappingAndVerifier, err error) {
	var w *wallet
	if isNewMapping {
		// Match it to a wallet (or fail)
		w, err = kr.km.selectWallet(ctx, identifier)
		if err != nil {
			return nil, err
		}
		mapping.Wallet = w.name
	} else {
		// Get the wallet runtime that's already been bound - possible still to fail
		// due to re-configuration of the node.
		w, err = kr.km.getWalletByName(ctx, mapping.Wallet)
		if err != nil {
			return nil, err
		}

		// Check if the verifier is being created in this context
		for _, v := range kr.newVerifiers {
			if v.KeyIdentifier == identifier && v.Algorithm == algorithm && v.Type == verifierType {
				log.L(ctx).Infof("Resolved key (created earlier in context): identifier=%s algorithm=%s verifierType=%s keyHandle=%s verifier=%s",
					identifier, algorithm, verifierType, mapping.KeyHandle, v.Verifier)
				// We have everything we need - no need to bother the signing module
				return &pldapi.KeyMappingAndVerifier{
					KeyMappingWithPath: mapping,
					Verifier:           v.KeyVerifier,
				}, nil
			}
		}

		// Check the DB for a verifier for this existing mapping.
		v, err := kr.getStoredVerifier(ctx, identifier, algorithm, verifierType)
		if err != nil {
			return nil, err
		}
		if v != nil {
			log.L(ctx).Infof("Resolved key (cached): identifier=%s algorithm=%s verifierType=%s keyHandle=%s verifier=%s",
				identifier, algorithm, verifierType, mapping.KeyHandle, v.Verifier)
			// populate the reverse lookup cache
			kr.km.verifierReverseCache.Set(verifierReverseCacheKey(v.Type, v.Algorithm, v.Verifier), &pldapi.KeyMappingAndVerifier{
				KeyMappingWithPath: mapping,
				Verifier:           v,
			})
			// We have everything we need - no need to bother the signing module
			return &pldapi.KeyMappingAndVerifier{
				KeyMappingWithPath: mapping,
				Verifier:           v,
			}, nil
		}

	}

	// We shouldn't get here for an existing mapping
	if requireExistingMapping {
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerExistingIdentifierNotFound, identifier)
	}

	// Ok - we are ready to talk to the wallet signing module to resolve the
	// key handle and verifier.
	result, err := w.resolveKeyAndVerifier(ctx, mapping, algorithm, verifierType)
	if err != nil {
		return nil, err
	}

	// We have a verifier and possibly a new mapping to write in our pre-commit
	if isNewMapping {
		kr.newMappings = append(kr.newMappings, result.KeyMappingWithPath)
	}
	// We add the verifier to our list to create here - but there is one small edge case where
	// this might be a duplicate. If multiple threads race to create a second verifier for
	// an existing key. Because there's no locking needed on the mapping to do that.
	// This is fine because it's deterministic, and we just do an ON CONFLICT DO NOTHING below.
	kr.newVerifiers = append(kr.newVerifiers, &pldapi.KeyVerifierWithKeyRef{
		KeyIdentifier: identifier,
		KeyVerifier:   result.Verifier,
	})

	log.L(ctx).Infof("Resolved key: identifier=%s algorithm=%s verifierType=%s keyHandle=%s verifier=%s",
		identifier, algorithm, verifierType, result.KeyHandle, result.Verifier.Verifier)
	return result, nil

}

func verifierForwardCacheKey(keyIdentifier, algorithm, verifierType string) string {
	return fmt.Sprintf("%s|%s|%s", keyIdentifier, algorithm, verifierType)
}

func verifierReverseCacheKey(algorithm, verifierType, verifier string) string {
	return fmt.Sprintf("%s|%s|%s", algorithm, verifierType, verifier)
}

func (kr *keyResolver) preCommit(ctx context.Context, dbTX persistence.DBTX) (err error) {
	kr.l.Lock()
	defer kr.l.Unlock()

	if len(kr.newMappings) > 0 {
		dbMappings := make([]*DBKeyMapping, len(kr.newMappings))
		for i, m := range kr.newMappings {
			dbMappings[i] = &DBKeyMapping{
				Identifier: m.Identifier,
				Wallet:     m.Wallet,
				KeyHandle:  m.KeyHandle,
			}
		}
		// Note we have locking to prevent us having an ON CONFLICT here, and
		// if one is added it needs careful understanding of why.
		err = dbTX.DB().WithContext(ctx).Create(dbMappings).Error
	}
	if err == nil && len(kr.newVerifiers) > 0 {
		dbVerifiers := make([]*DBKeyVerifier, len(kr.newVerifiers))
		for i, v := range kr.newVerifiers {
			dbVerifiers[i] = &DBKeyVerifier{
				Identifier: v.KeyIdentifier,
				Algorithm:  v.Algorithm,
				Type:       v.Type,
				Verifier:   v.Verifier,
			}
		}
		err = dbTX.DB().WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}). // explained where we add to kr.newVerifiers
			Create(dbVerifiers).
			Error
	}
	return err
}

func (kr *keyResolver) postCommit() {
	// This updates all the caches after we're confident the data is committed to the DB
	for _, v := range kr.newVerifiers {
		kr.km.verifierByIdentityCache.Set(verifierForwardCacheKey(v.KeyIdentifier, v.Algorithm, v.Type), v.KeyVerifier)
	}
	for _, m := range kr.newMappings {
		kr.km.identifierCache.Set(m.Identifier, m)
		for _, v := range kr.newVerifiers {
			if v.KeyIdentifier == m.Identifier {
				// populate the reverse lookup cache
				kr.km.verifierReverseCache.Set(verifierReverseCacheKey(v.Type, v.Algorithm, v.Verifier), &pldapi.KeyMappingAndVerifier{
					KeyMappingWithPath: m,
					Verifier:           v.KeyVerifier,
				})
			}
		}
	}
}

func (kr *keyResolver) finalizer(ctx context.Context, err error) {
	kr.l.Lock()
	defer kr.l.Unlock()

	if err == nil {
		kr.postCommit()
	}
	kr.cleanup(ctx)
}

func (kr *keyResolver) cleanup(ctx context.Context) {
	if kr.done != nil {
		if kr.allocationLockTaken {
			kr.km.unlockAllocation(ctx, kr)
		}
		// All other KRCs waiting on us will wake up and race to grab the lock on the parent context of their choosing
		close(kr.done)
		kr.done = nil
	}
}

func (resolved *resolvedDBPath) pathSegments() []*pldapi.KeyPathSegment {
	var reversed []*pldapi.KeyPathSegment
	p := resolved
	for p != nil {
		if p.segment == "" /* don't include the root */ {
			break
		}
		reversed = append(reversed, &pldapi.KeyPathSegment{
			Name:  p.segment,
			Index: p.index,
		})
		p = p.parent
	}
	segments := make([]*pldapi.KeyPathSegment, len(reversed))
	for i := 0; i < len(segments); i++ {
		segments[i] = reversed[len(segments)-1-i]
	}
	return segments
}
