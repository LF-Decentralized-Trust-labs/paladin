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

package domainmgr

import (
	"context"
	"encoding/json"
	"sync"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/cache"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/inflight"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"gorm.io/gorm"
)

//go:embed abis/IPaladinContract_V0.json
var iPaladinContractBuildJSON []byte
var iPaladinContractABI = mustParseEmbeddedBuildABI(iPaladinContractBuildJSON)

var eventSig_PaladinNewSmartContract_V0 = mustParseEventSignatureHash(iPaladinContractABI, "PaladinNewSmartContract_V0")
var eventSolSig_PaladinNewSmartContract_V0 = mustParseEventSoliditySignature(iPaladinContractABI, "PaladinNewSmartContract_V0")

// var eventSig_PaladinPrivateTransaction_V0 = mustParseEventSignature(iPaladinContractABI, "PaladinPrivateTransaction_V0")

func NewDomainManager(bgCtx context.Context, conf *DomainManagerConfig) components.DomainManager {
	return &domainManager{
		bgCtx:            bgCtx,
		conf:             conf,
		domainsByID:      make(map[uuid.UUID]*domain),
		domainsByName:    make(map[string]*domain),
		domainsByAddress: make(map[types.EthAddress]*domain),
		contractWaiter:   inflight.NewInflightManager[uuid.UUID, *PrivateSmartContract](uuid.Parse),
		contractCache:    cache.NewCache[types.EthAddress, *domainContract](&conf.DomainManager.ContractCache, ContractCacheDefaults),
	}
}

type domainManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf             *DomainManagerConfig
	persistence      persistence.Persistence
	stateStore       statestore.StateStore
	blockIndexer     blockindexer.BlockIndexer
	ethClientFactory ethclient.EthClientFactory

	domainsByID      map[uuid.UUID]*domain
	domainsByName    map[string]*domain
	domainsByAddress map[types.EthAddress]*domain

	contractWaiter *inflight.InflightManager[uuid.UUID, *PrivateSmartContract]
	contractCache  cache.Cache[types.EthAddress, *domainContract]
}

type event_PaladinNewSmartContract_V0 struct {
	TXId   types.Bytes32    `json:"txId"`
	Domain types.EthAddress `json:"domain"`
	Data   types.HexBytes   `json:"data"`
}

func (dm *domainManager) Init(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	dm.persistence = pic.Persistence()
	dm.stateStore = pic.StateStore()
	dm.ethClientFactory = pic.EthClientFactory()
	dm.blockIndexer = pic.BlockIndexer()
	return &components.ManagerInitResult{
		EventStreams: []*components.ManagerEventStream{
			{
				ABI:     iPaladinContractABI,
				Handler: dm.eventIndexer,
			},
		},
	}, nil
}

func (dm *domainManager) Start() error { return nil }

func (dm *domainManager) Stop() {
	dm.mux.Lock()
	var allDomains []*domain
	for _, d := range dm.domainsByID {
		allDomains = append(allDomains, d)
	}
	dm.mux.Unlock()
	for _, d := range allDomains {
		dm.cleanupDomain(d)
	}

}

func (dm *domainManager) cleanupDomain(d *domain) {
	// must not hold the domain lock when running this
	d.close()
	delete(dm.domainsByID, d.id)
	delete(dm.domainsByName, d.name)
	if d.factoryContractAddress != nil {
		delete(dm.domainsByAddress, *d.factoryContractAddress)
	}
}

func (dm *domainManager) ConfiguredDomains() map[string]*plugins.PluginConfig {
	pluginConf := make(map[string]*plugins.PluginConfig)
	for name, conf := range dm.conf.Domains {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (dm *domainManager) DomainRegistered(name string, id uuid.UUID, toDomain plugins.DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()

	// Replaces any previously registered instance
	existing := dm.domainsByName[name]
	for existing != nil {
		// Can't hold the lock in cleanup, hence the loop
		dm.mux.Unlock()
		dm.cleanupDomain(existing)
		dm.mux.Lock()
		existing = dm.domainsByName[name]
	}

	// Get the config for this domain
	conf := dm.conf.Domains[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(dm.bgCtx, msgs.MsgDomainNotFound, name)
	}

	// Initialize
	d := dm.newDomain(id, name, conf, toDomain)
	dm.domainsByID[id] = d
	dm.domainsByName[name] = d
	go d.init()
	return d, nil
}

func (dm *domainManager) GetDomainByName(ctx context.Context, name string) (components.Domain, error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	d := dm.domainsByName[name]
	if d == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, name)
	}
	return d, nil
}

func (dm *domainManager) WaitForDeploy(ctx context.Context, txID uuid.UUID) (components.DomainSmartContract, error) {
	// Waits for the event that confirms a smart contract has been deployed (or a context timeout)
	// using the transaction ID of the deploy transaction
	req := dm.contractWaiter.AddInflight(ctx, txID)
	defer req.Cancel()

	dc, err := dm.dbGetSmartContract(ctx, func(db *gorm.DB) *gorm.DB { return db.Where("deploy_tx = ?", txID) })
	if err != nil {
		return nil, err
	}
	if dc != nil {
		// contract was already indexed
		return dc, nil
	}

	// wait until the event gets indexed (or the context expires)
	def, err := req.Wait()
	if err != nil {
		return nil, err
	}
	return dm.enrichContractWithDomain(ctx, def)

}

func (dm *domainManager) setDomainAddress(d *domain) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	dm.domainsByAddress[*d.factoryContractAddress] = d
}

func (dm *domainManager) getDomainByAddress(ctx context.Context, addr *types.EthAddress) (d *domain, _ error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	if addr != nil {
		d = dm.domainsByAddress[*addr]
	}
	if d == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, addr)
	}
	return d, nil
}

func (dm *domainManager) GetSmartContractByAddress(ctx context.Context, addr types.EthAddress) (components.DomainSmartContract, error) {
	dc, isCached := dm.contractCache.Get(addr)
	if isCached {
		return dc, nil
	}
	// Updating the cache deferred down to newSmartContract (under enrichContractWithDomain)
	dc, err := dm.dbGetSmartContract(ctx, func(db *gorm.DB) *gorm.DB { return db.Where("address = ?", addr) })
	if err != nil {
		return nil, err
	}
	if dc == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainContractNotFoundByAddr, addr)
	}
	return dc, nil
}

func (dm *domainManager) dbGetSmartContract(ctx context.Context, setWhere func(db *gorm.DB) *gorm.DB) (*domainContract, error) {
	var contracts []*PrivateSmartContract
	query := dm.persistence.DB().Table("private_smart_contracts")
	query = setWhere(query)
	err := query.
		WithContext(ctx).
		Limit(1).
		Find(&contracts).
		Error
	if err != nil || len(contracts) == 0 {
		return nil, err
	}
	return dm.enrichContractWithDomain(ctx, contracts[0])

}

func (dm *domainManager) enrichContractWithDomain(ctx context.Context, def *PrivateSmartContract) (*domainContract, error) {

	// Get the domain by address
	d, err := dm.getDomainByAddress(ctx, &def.DomainAddress)
	if err != nil {
		return nil, err
	}

	return d.newSmartContract(def), nil
}

// If an embedded ABI is broken, we don't even run the tests / start the runtime
func mustParseEmbeddedBuildABI(abiJSON []byte) abi.ABI {
	type buildABI struct {
		ABI abi.ABI `json:"abi"`
	}
	var build buildABI
	err := json.Unmarshal([]byte(abiJSON), &build)
	if err != nil {
		panic(err)
	}
	return build.ABI
}

func mustParseEventSoliditySignature(a abi.ABI, eventName string) string {
	event := a.Events()[eventName]
	if event == nil {
		panic("ABI missing " + eventName)
	}
	solString, err := event.SolidityStringCtx(context.Background())
	if err != nil {
		panic(err)
	}
	return solString
}

func mustParseEventSignatureHash(a abi.ABI, eventName string) types.Bytes32 {
	event := a.Events()[eventName]
	if event == nil {
		panic("ABI missing " + eventName)
	}
	sig, err := event.SignatureHash()
	if err != nil {
		panic(err)
	}
	return types.NewBytes32FromSlice(sig)
}
