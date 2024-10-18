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
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"gorm.io/gorm"

	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
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

	p persistence.Persistence
}

func NewKeyManager(bgCtx context.Context, conf *pldconf.KeyManagerConfig) components.KeyManager {
	return &keyManager{
		bgCtx:                   bgCtx,
		conf:                    conf,
		identifierCache:         cache.NewCache[string, *pldapi.KeyMappingWithPath](&conf.IdentifierCache, &pldconf.KeyManagerDefaults.IdentifierCache),
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

func (km *keyManager) Start() error {
	return nil
}

func (km *keyManager) Stop() {
}

func (km *keyManager) Sign(ctx context.Context, mapping *pldapi.KeyMappingAndVerifier, payloadType string, payload []byte) ([]byte, error) {
	w, err := km.getWalletByName(ctx, mapping.Wallet)
	if err != nil {
		return nil, err
	}
	return w.sign(ctx, mapping, payloadType, payload)
}

func (km *keyManager) lockAllocationOrGetOwner(krc *keyResolver) *keyResolver {
	km.allocLock.Lock()
	defer km.allocLock.Unlock()
	if km.allocLockHolder != nil {
		return km.allocLockHolder
	}
	km.allocLockHolder = krc
	return nil
}

func (km *keyManager) takeAllocationLock(krc *keyResolver) error {
	ctx := krc.ctx
	for {
		lockingKRC := km.lockAllocationOrGetOwner(krc)
		if lockingKRC == nil {
			log.L(ctx).Debugf("key resolution context %s locked allocation", krc.id)
			return nil
		}
		// There is contention on this path - wait until the lock is released, and try to get it again
		select {
		case <-lockingKRC.done:
		case <-ctx.Done():
			log.L(ctx).Debugf("key resolution context %s cancelled while waiting for allocation unlocked by %s", krc.id, lockingKRC.id)
			return i18n.NewError(ctx, msgs.MsgContextCanceled)
		}
	}
}

func (km *keyManager) unlockAllocation(krc *keyResolver) {
	km.allocLock.Lock()
	defer km.allocLock.Unlock()

	// We will have locks on all the parent paths
	if km.allocLockHolder == krc {
		log.L(krc.ctx).Debugf("key resolution context %s unlocked allocation", krc.id)
		km.allocLockHolder = nil
	} else {
		existingID := "null"
		if km.allocLockHolder != nil {
			existingID = km.allocLockHolder.id
		}
		log.L(krc.ctx).Errorf("key resolution context %s attempted to unlock allocation lock held by %s", krc.id, existingID)
	}
}

func (km *keyManager) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {
	// Called during PostInit phase by domain manager
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

func (km *keyManager) ResolveEthAddressNewDatabaseTX(ctx context.Context, identifier string) (ethAddress *tktypes.EthAddress, err error) {
	ethAddresses, err := km.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
	if err != nil {
		return nil, err
	}
	return ethAddresses[0], nil
}

func (km *keyManager) ResolveEthAddressBatchNewDatabaseTX(ctx context.Context, identifiers []string) (ethAddresses []*tktypes.EthAddress, err error) {
	ethAddresses = make([]*tktypes.EthAddress, len(identifiers))
	resolvedKeys, err := km.ResolveBatchNewDatabaseTX(ctx, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, identifiers)
	for i := 0; i < len(identifiers); i++ {
		if err == nil {
			ethAddresses[i], err = tktypes.ParseEthAddress(resolvedKeys[i].Verifier.Verifier)
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
	krc := km.NewKeyResolutionContextLazyDB(ctx)
	defer func() {
		if err != nil {
			krc.Rollback()
		} else {
			err = krc.Commit()
		}
	}()
	for i, identifier := range identifiers {
		if err == nil {
			resolvedKeys[i], err = krc.KeyResolverLazyDB().ResolveKey(identifier, algorithm, verifierType)
		}
	}
	if err != nil {
		return nil, err
	}
	return resolvedKeys, nil
}

func (km *keyManager) ReverseKeyLookup(ctx context.Context, dbTX *gorm.DB, algorithm, verifierType, verifier string) (*pldapi.KeyMappingAndVerifier, error) {
	vKey := verifierReverseCacheKey(algorithm, verifierType, verifier)
	mapping, _ := km.verifierReverseCache.Get(vKey)
	if mapping != nil {
		return mapping, nil
	}
	var dbVerifiers []*DBKeyVerifier
	err := dbTX.WithContext(ctx).
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
	// NOTE: this is an internal-only use mode of a KRC that does not follow the external convention
	krc := km.NewKeyResolutionContext(ctx)
	defer krc.Close(false) // no changes to commit
	mapping, err = krc.KeyResolver(dbTX).(*keyResolver).
		resolveKey(dbVerifiers[0].Identifier, algorithm, verifierType, true /* existing only */)
	if err != nil {
		return nil, err
	}
	km.verifierReverseCache.Set(vKey, mapping)
	return mapping, nil
}
