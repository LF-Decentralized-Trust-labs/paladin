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
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"

	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/inflight"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"gorm.io/gorm"
)

//go:embed abis/IPaladinContractRegistry_V0.json
var iPaladinContractRegistryBuildJSON []byte

var iPaladinContractRegistryABI = mustParseEmbeddedBuildABI(iPaladinContractRegistryBuildJSON)

var eventSig_PaladinRegisterSmartContract_V0 = mustParseEventSignatureHash(iPaladinContractRegistryABI, "PaladinRegisterSmartContract_V0")
var eventSolSig_PaladinRegisterSmartContract_V0 = mustParseEventSoliditySignature(iPaladinContractRegistryABI, "PaladinRegisterSmartContract_V0")

// var eventSig_PaladinPrivateTransaction_V0 = mustParseEventSignature(iPaladinContractABI, "PaladinPrivateTransaction_V0")

func NewDomainManager(bgCtx context.Context, conf *pldconf.DomainManagerConfig) components.DomainManager {
	allDomains := []string{}
	for name := range conf.Domains {
		allDomains = append(allDomains, name)
	}
	log.L(bgCtx).Infof("Domains configured: %v", allDomains)
	return &domainManager{
		bgCtx:            bgCtx,
		conf:             conf,
		domainsByName:    make(map[string]*domain),
		domainsByAddress: make(map[tktypes.EthAddress]*domain),
		privateTxWaiter:  inflight.NewInflightManager[uuid.UUID, *components.ReceiptInput](uuid.Parse),
		contractCache:    cache.NewCache[tktypes.EthAddress, *domainContract](&conf.DomainManager.ContractCache, pldconf.ContractCacheDefaults),
	}
}

type domainManager struct {
	bgCtx context.Context
	mux   sync.Mutex

	conf             *pldconf.DomainManagerConfig
	persistence      persistence.Persistence
	stateStore       components.StateManager
	privateTxManager components.PrivateTxManager
	txManager        components.TXManager
	blockIndexer     blockindexer.BlockIndexer
	ethClientFactory ethclient.EthClientFactory
	domainSigner     *domainSigner

	domainsByName    map[string]*domain
	domainsByAddress map[tktypes.EthAddress]*domain

	privateTxWaiter *inflight.InflightManager[uuid.UUID, *components.ReceiptInput]
	contractCache   cache.Cache[tktypes.EthAddress, *domainContract]
}

type event_PaladinRegisterSmartContract_V0 struct {
	TXId     tktypes.Bytes32    `json:"txId"`
	Domain   tktypes.EthAddress `json:"domain"`
	Instance tktypes.EthAddress `json:"instance"`
	Config   tktypes.HexBytes   `json:"config"`
}

func (dm *domainManager) PreInit(pic components.PreInitComponents) (*components.ManagerInitResult, error) {
	return &components.ManagerInitResult{}, nil
}

func (dm *domainManager) PostInit(c components.AllComponents) error {
	dm.stateStore = c.StateManager()
	dm.txManager = c.TxManager()
	dm.privateTxManager = c.PrivateTxManager()
	dm.persistence = c.Persistence()
	dm.ethClientFactory = c.EthClientFactory()
	dm.blockIndexer = c.BlockIndexer()

	// Register ourselves as a signing on the key manager
	dm.domainSigner = &domainSigner{dm: dm}
	c.KeyManager().AddInMemorySigner("domain", dm.domainSigner)

	for name, d := range dm.conf.Domains {
		if _, err := tktypes.ParseEthAddress(d.RegistryAddress); err != nil {
			return i18n.WrapError(dm.bgCtx, err, msgs.MsgDomainRegistryAddressInvalid, d.RegistryAddress, name)
		}
	}
	return nil
}

func (dm *domainManager) Start() error { return nil }

func (dm *domainManager) Stop() {
	dm.mux.Lock()
	var allDomains []*domain
	for _, d := range dm.domainsByName {
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
	delete(dm.domainsByName, d.name)
	delete(dm.domainsByAddress, *d.RegistryAddress())
}

func (dm *domainManager) ConfiguredDomains() map[string]*pldconf.PluginConfig {
	pluginConf := make(map[string]*pldconf.PluginConfig)
	for name, conf := range dm.conf.Domains {
		pluginConf[name] = &conf.Plugin
	}
	return pluginConf
}

func (dm *domainManager) DomainRegistered(name string, toDomain components.DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error) {
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
	d := dm.newDomain(name, conf, toDomain)
	dm.domainsByName[name] = d
	go d.init()
	return d, nil
}

// fails if domain is not yet initialized (note external endpoints of Paladin do not open up until all domains initialized)
func (dm *domainManager) GetDomainByName(ctx context.Context, name string) (components.Domain, error) {
	domain, err := dm.getDomainByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := domain.checkInit(ctx); err != nil {
		return nil, err
	}
	return domain, nil
}

func (dm *domainManager) getDomainByName(ctx context.Context, name string) (*domain, error) {
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
	req := dm.privateTxWaiter.AddInflight(ctx, txID)
	defer req.Cancel()

	dc, err := dm.dbGetSmartContract(ctx, dm.persistence.DB(), func(db *gorm.DB) *gorm.DB { return db.Where("deploy_tx = ?", txID) })
	if err != nil {
		return nil, err
	}
	if dc != nil {
		// contract was already indexed
		return dc, nil
	}
	return dm.waitAndEnrich(ctx, req)
}

func (dm *domainManager) GetSigner() signerapi.InMemorySigner {
	return dm.domainSigner
}

func (dm *domainManager) waitAndEnrich(ctx context.Context, req *inflight.InflightRequest[uuid.UUID, *components.ReceiptInput]) (components.DomainSmartContract, error) {
	// wait until the event gets indexed (or the context expires)
	receipt, err := req.Wait()
	if err != nil {
		return nil, err
	}

	if receipt.ContractAddress == nil {
		log.L(ctx).Errorf("Waiter expected a contract deployment: %+v", receipt)
		return nil, i18n.NewError(ctx, msgs.MsgDomainTransactionWasNotADeployment, receipt.TransactionID)
	}

	return dm.GetSmartContractByAddress(ctx, *receipt.ContractAddress)
}

func (dm *domainManager) WaitForTransaction(ctx context.Context, txID uuid.UUID) error {
	// Waits for the event that confirms a transaction has been processed (or a context timeout)
	// using the ID of the transaction
	req := dm.privateTxWaiter.AddInflight(ctx, txID)
	defer req.Cancel()
	_, err := req.Wait()
	return err
}

func (dm *domainManager) setDomainAddress(d *domain) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	dm.domainsByAddress[*d.RegistryAddress()] = d
}

func (dm *domainManager) getDomainByAddress(ctx context.Context, addr *tktypes.EthAddress, nilOnNotFound bool) (d *domain, _ error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	if addr != nil {
		d = dm.domainsByAddress[*addr]
	}
	if d == nil && !nilOnNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, addr)
	}
	return d, nil
}

func (dm *domainManager) GetSmartContractByAddress(ctx context.Context, addr tktypes.EthAddress) (components.DomainSmartContract, error) {
	dc, err := dm.getSmartContractCached(ctx, dm.persistence.DB(), addr)
	if dc != nil || err != nil {
		return dc, err
	}
	return nil, i18n.NewError(ctx, msgs.MsgDomainContractNotFoundByAddr, addr)
}

func (dm *domainManager) getSmartContractCached(ctx context.Context, tx *gorm.DB, addr tktypes.EthAddress) (*domainContract, error) {
	dc, isCached := dm.contractCache.Get(addr)
	if isCached {
		return dc, nil
	}
	// Updating the cache deferred down to newSmartContract (under enrichContractWithDomain)
	return dm.dbGetSmartContract(ctx, tx, func(db *gorm.DB) *gorm.DB { return db.Where("address = ?", addr) })
}

func (dm *domainManager) dbGetSmartContract(ctx context.Context, tx *gorm.DB, setWhere func(db *gorm.DB) *gorm.DB) (*domainContract, error) {
	var contracts []*PrivateSmartContract
	query := tx.Table("private_smart_contracts")
	query = setWhere(query)
	err := query.
		WithContext(ctx).
		Limit(1).
		Find(&contracts).
		Error
	if err != nil || len(contracts) == 0 {
		return nil, err
	}

	// At this point it's possible we have a matching smart contract in our DB, for which we
	// no longer recognize the domain registry (as it's not one that is configured an longer)
	dc, err := dm.enrichContractWithDomain(ctx, contracts[0], true)
	if err == nil && dc == nil {
		log.L(ctx).Warnf("Lookup of smart contract '%s' that is stored in the DB for domain registry '%s' that is no longer configured on this node", contracts[0].Address, contracts[0].RegistryAddress)
	}
	return dc, nil
}

func (dm *domainManager) enrichContractWithDomain(ctx context.Context, contract *PrivateSmartContract, nilOnNotFound bool) (*domainContract, error) {

	// Get the domain by address
	d, err := dm.getDomainByAddress(ctx, &contract.RegistryAddress, nilOnNotFound)
	if d == nil || err != nil {
		return nil, err
	}

	return d.newSmartContract(contract), nil
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

func mustParseEventSignatureHash(a abi.ABI, eventName string) tktypes.Bytes32 {
	event := a.Events()[eventName]
	if event == nil {
		panic("ABI missing " + eventName)
	}
	sig, err := event.SignatureHash()
	if err != nil {
		panic(err)
	}
	return tktypes.NewBytes32FromSlice(sig)
}
