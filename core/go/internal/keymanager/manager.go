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

package keymanager

import (
	"context"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/filters"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/rpcserver"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
)

type keyManager struct {
	bgCtx context.Context

	conf                    *pldconf.KeyManagerConfig
	rpcModule               *rpcserver.RPCModule
	identifierCache         cache.Cache[string, *pldapi.KeyMappingWithPath]
	verifierByIdentityCache cache.Cache[string, *pldapi.KeyVerifier]
	verifierReverseCache    cache.Cache[string, *pldapi.KeyMappingAndVerifier]
	walletsOrdered          []*wallet
	walletsByName           map[string]*wallet

	allocLock       sync.Mutex
	allocLockHolder *keyResolver

	// plugin signing modules
	mux                  sync.Mutex
	signingModulesByID   map[uuid.UUID]*signingModule
	signingModulesByName map[string]*signingModule

	p persistence.Persistence
}

func NewKeyManager(bgCtx context.Context, conf *pldconf.KeyManagerConfig) components.KeyManager {
	return &keyManager{
		bgCtx:                   bgCtx,
		conf:                    conf,
		identifierCache:         cache.NewCache[string, *pldapi.KeyMappingWithPath](&conf.IdentifierCache, &pldconf.KeyManagerDefaults.IdentifierCache),
		signingModulesByID:      make(map[uuid.UUID]*signingModule),
		signingModulesByName:    make(map[string]*signingModule),
		verifierByIdentityCache: cache.NewCache[string, *pldapi.KeyVerifier](&conf.VerifierCache, &pldconf.KeyManagerDefaults.VerifierCache),
		verifierReverseCache:    cache.NewCache[string, *pldapi.KeyMappingAndVerifier](&conf.VerifierCache, &pldconf.KeyManagerDefaults.VerifierCache),
		walletsByName:           make(map[string]*wallet),
	}
}

func (km *keyManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	km.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{km.rpcModule},
	}, nil
}

func (km *keyManager) PostInit(c components.AllComponents) error {
	km.p = c.Persistence()
	return nil
}

func (km *keyManager) Start() error {
	// Process wallets once all signing modules have been loaded
	for _, walletConf := range km.conf.Wallets {
		w, err := km.newWallet(km.bgCtx, walletConf)
		if err != nil {
			return err
		}
		if km.walletsByName[w.name] != nil {
			return i18n.NewError(km.bgCtx, msgs.MsgKeyManagerDuplicateName, w.name)
		}
		km.walletsByName[w.name] = w
		km.walletsOrdered = append(km.walletsOrdered, w)
	}

	return nil
}

func (km *keyManager) Stop() {
}

func (km *keyManager) cleanupSigningModule(sm *signingModule) {
	sm.close()
	delete(km.signingModulesByID, sm.id)
	delete(km.signingModulesByName, sm.name)
}

func (km *keyManager) ConfiguredSigningModules() map[string]*pldconf.PluginConfig {
	pluginConf := make(map[string]*pldconf.PluginConfig)
	for name, conf := range km.conf.SigningModules {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (km *keyManager) SigningModuleRegistered(name string, id uuid.UUID, toSigningModule components.KeyManagerToSigningModule) (fromSigningModule plugintk.SigningModuleCallbacks, err error) {
	// Replaces any previously registered instance
	existingSigningModule, _ := km.GetSigningModule(km.bgCtx, name)
	for existingSigningModule != nil {
		// Can't hold the lock in cleanup, hence the loop
		km.cleanupSigningModule(existingSigningModule.(*signingModule))
		existingSigningModule, _ = km.GetSigningModule(km.bgCtx, name)
	}

	km.mux.Lock()
	defer km.mux.Unlock()

	// Get the config for this signing module
	conf := km.conf.SigningModules[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(km.bgCtx, msgs.MsgKeyManagerSigningModuleNotFound, name)
	}

	// Initialize
	sm := km.newSigningModule(id, name, conf, toSigningModule).(*signingModule)
	km.signingModulesByID[id] = sm
	km.signingModulesByName[name] = sm
	go sm.init()
	return sm, nil
}

func (km *keyManager) GetSigningModule(ctx context.Context, name string) (signer.SigningModule, error) {
	km.mux.Lock()
	defer km.mux.Unlock()

	sm := km.signingModulesByName[name]
	if sm == nil {
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerSigningModuleNotFound, name)
	}
	return sm, nil
}

func (km *keyManager) Sign(ctx context.Context, mapping *pldapi.KeyMappingAndVerifier, payloadType string, payload []byte) ([]byte, error) {
	w, err := km.getWalletByName(ctx, mapping.Wallet)
	if err != nil {
		return nil, err
	}
	return w.sign(ctx, mapping, payloadType, payload)
}

func (km *keyManager) lockAllocationOrGetOwner(kr *keyResolver) *keyResolver {
	km.allocLock.Lock()
	defer km.allocLock.Unlock()
	if km.allocLockHolder != nil {
		return km.allocLockHolder
	}
	km.allocLockHolder = kr
	return nil
}

func (km *keyManager) takeAllocationLock(ctx context.Context, kr *keyResolver) error {
	for {
		lockingKRC := km.lockAllocationOrGetOwner(kr)
		if lockingKRC == nil {
			log.L(ctx).Debugf("key resolution context %s locked allocation", kr.id)
			return nil
		}
		// There is contention on this path - wait until the lock is released, and try to get it again
		select {
		case <-lockingKRC.done:
		case <-ctx.Done():
			log.L(ctx).Debugf("key resolution context %s cancelled while waiting for allocation unlocked by %s", kr.id, lockingKRC.id)
			return i18n.NewError(ctx, msgs.MsgContextCanceled)
		}
	}
}

func (km *keyManager) unlockAllocation(ctx context.Context, kr *keyResolver) {
	km.allocLock.Lock()
	defer km.allocLock.Unlock()

	// We will have locks on all the parent paths
	if km.allocLockHolder == kr {
		log.L(ctx).Debugf("key resolution context %s unlocked allocation", kr.id)
		km.allocLockHolder = nil
	} else {
		existingID := "null"
		if km.allocLockHolder != nil {
			existingID = km.allocLockHolder.id
		}
		log.L(ctx).Errorf("key resolution context %s attempted to unlock allocation lock held by %s", kr.id, existingID)
	}
}

func (km *keyManager) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {
	// Called during Start phase by domain manager
	for _, w := range km.walletsByName {
		w.signingModule.AddInMemorySigner(prefix, signer)
	}
}

// Convenience function
func (km *keyManager) ResolveKeyNewDatabaseTX(ctx context.Context, identifier, algorithm, verifierType string) (resolvedKey *pldapi.KeyMappingAndVerifier, err error) {
	resolvedKeys, err := km.ResolveBatchNewDatabaseTX(ctx, algorithm, verifierType, []string{identifier})
	if err != nil {
		return nil, err
	}
	return resolvedKeys[0], nil
}

func (km *keyManager) ResolveEthAddressNewDatabaseTX(ctx context.Context, identifier string) (ethAddress *pldtypes.EthAddress, err error) {
	ethAddresses, err := km.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return nil, err
	}
	return ethAddresses[0], nil
}

func (km *keyManager) ResolveEthAddressBatchNewDatabaseTX(ctx context.Context, identifiers []string) (ethAddresses []*pldtypes.EthAddress, err error) {
	ethAddresses = make([]*pldtypes.EthAddress, len(identifiers))
	resolvedKeys, err := km.ResolveBatchNewDatabaseTX(ctx, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, identifiers)
	for i := 0; i < len(identifiers); i++ {
		if err == nil {
			ethAddresses[i], err = pldtypes.ParseEthAddress(resolvedKeys[i].Verifier.Verifier)
		}
	}
	if err != nil {
		return nil, err
	}
	return ethAddresses, nil
}

// Convenience function
func (km *keyManager) ResolveBatchNewDatabaseTX(ctx context.Context, algorithm, verifierType string, identifiers []string) (resolvedKeys []*pldapi.KeyMappingAndVerifier, err error) {
	resolvedKeys = make([]*pldapi.KeyMappingAndVerifier, len(identifiers))
	err = km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)
		for i, identifier := range identifiers {
			if err == nil {
				resolvedKeys[i], err = kr.ResolveKey(ctx, identifier, algorithm, verifierType)
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return resolvedKeys, nil
}

func (km *keyManager) ReverseKeyLookup(ctx context.Context, dbTX persistence.DBTX, algorithm, verifierType, verifier string) (*pldapi.KeyMappingAndVerifier, error) {
	vKey := verifierReverseCacheKey(algorithm, verifierType, verifier)
	mapping, _ := km.verifierReverseCache.Get(vKey)
	if mapping != nil {
		return mapping, nil
	}
	var dbVerifiers []*DBKeyVerifier
	err := dbTX.DB().WithContext(ctx).
		Where(`"algorithm" = ?`, algorithm).
		Where(`"type" = ?`, verifierType).
		Where(`"verifier" = ?`, verifier).
		Limit(1).
		Find(&dbVerifiers).
		Error
	if err != nil {
		return nil, err
	}
	if len(dbVerifiers) == 0 {
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerVerifierLookupNotFound)
	}

	// Now we need to look up the associated mapping and rebuild it
	// NOTE: this is an internal-only use mode of a KRC that does not follow the external convention. Which means
	kr := km.newKeyResolver(dbTX, false /* allowing use with NOTX() */).(*keyResolver)
	mapping, err = kr.resolveKey(ctx, dbVerifiers[0].Identifier, algorithm, verifierType, true /* existing only */)
	if err != nil {
		return nil, err
	}
	km.verifierReverseCache.Set(vKey, mapping)
	return mapping, nil
}

func (km *keyManager) QueryKeys(ctx context.Context, dbTX *gorm.DB, jq *query.QueryJSON) (keyList []*pldapi.KeyQueryEntry, err error) {

	q := filters.BuildGORM(ctx, jq,
		dbTX.WithContext(ctx).
			Table("key_paths"), KeyEntryFilters)

	q.Select(`DISTINCT key_mappings.identifier IS NOT NULL AS "is_key",` +
		`k.p IS NOT NULL AS "has_children",` +
		`key_paths.parent AS "parent",` +
		`key_paths."index" AS "index",` +
		`key_paths.path AS "path",` +
		`key_mappings.wallet AS "wallet",` +
		`key_mappings.key_handle AS "key_handle"`,
	)

	q.Joins("LEFT OUTER JOIN key_mappings ON key_paths.path = key_mappings.identifier")
	q.Joins(`LEFT OUTER JOIN (SELECT parent AS "p" from key_paths AS p) AS k ON key_paths.path = k.p`)
	q.Where("key_paths.path != ''")

	err = q.Find(&keyList).Error
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, k := range keyList {
		ids = append(ids, k.Path)
	}

	var verifiers []*DBKeyVerifier

	err = dbTX.Table("key_verifiers").
		Where("identifier IN ?", ids).
		Scan(&verifiers).Error
	if err != nil {
		return nil, err
	}

	for _, k := range keyList {
		for _, v := range verifiers {
			if k.Path == v.Identifier {
				k.Verifiers = append(k.Verifiers, &pldapi.KeyVerifier{
					Verifier:  v.Verifier,
					Type:      v.Type,
					Algorithm: v.Algorithm,
				})
			}
		}
	}

	return keyList, nil
}
