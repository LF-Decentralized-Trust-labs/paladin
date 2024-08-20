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

package main

import (
	"context"
	"encoding/json"
	"sync"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/components"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/blockindexer"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed abis/IPaladinContract_V0.json
var iPaladinContractBuildJSON []byte
var iPaladinContractABI = mustParseEmbeddedBuildABI(iPaladinContractBuildJSON)
var eventSig_PaladinNewSmartContract_V0 = mustParseEventSignature(iPaladinContractABI, "PaladinNewSmartContract_V0")
var eventSig_PaladinPrivateTransaction_V0 = mustParseEventSignature(iPaladinContractABI, "PaladinPrivateTransaction_V0")

func NewDomainManager(bgCtx context.Context, conf *DomainManagerConfig) components.DomainManager {
	return &domainManager{
		bgCtx:            bgCtx,
		conf:             conf,
		domainsByID:      make(map[uuid.UUID]*domain),
		domainsByName:    make(map[string]*domain),
		domainsByAddress: make(map[ethtypes.Address0xHex]*domain),
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
	chainID          int64

	domainsByID      map[uuid.UUID]*domain
	domainsByName    map[string]*domain
	domainsByAddress map[ethtypes.Address0xHex]*domain
}

type event_PaladinNewSmartContract_V0 struct {
	TXId    types.Bytes32             `json:"txId"`
	Address ethtypes.Address0xHex     `json:"address"`
	Data    ethtypes.HexBytes0xPrefix `json:"data"`
}

func (dm *domainManager) PreInit(pic components.PreInitComponents) (*components.InitInstructions, error) {
	dm.persistence = pic.Persistence()
	dm.stateStore = pic.StateStore()
	dm.chainID = pic.EthClientFactory().ChainID()
	return &components.InitInstructions{
		EventStreams: []*components.ManagerEventStream{
			{
				ABI:     iPaladinContractABI,
				Handler: dm.eventIndexer,
			},
		},
	}, nil
}

func (dm *domainManager) PostInit(c components.PostInitComponents) error {
	dm.blockIndexer = c.BlockIndexer()
	return nil
}

func (dm *domainManager) Start() error { return nil }

func (dm *domainManager) Stop() {}

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

	// Get the config for this domain
	conf := dm.conf.Domains[name]
	if conf == nil {
		// Shouldn't be possible
		return nil, i18n.NewError(dm.bgCtx, msgs.MsgDomainNotFound, name)
	}

	// Replaces any previously registered instance
	if existing := dm.domainsByID[id]; existing != nil {
		existing.close()
	}

	// Initialize
	d := dm.newDomain(id, name, conf, toDomain)
	dm.domainsByID[id] = d
	dm.domainsByName[name] = d
	go d.init()
	return d, nil
}

func (dm *domainManager) GetDomainByName(ctx context.Context, name string) (components.DomainActions, error) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	d := dm.domainsByName[name]
	if d == nil {
		return nil, i18n.NewError(ctx, msgs.MsgDomainNotFound, name)
	}
	return d, nil
}

func (dm *domainManager) setDomainAddress(d *domain) {
	dm.mux.Lock()
	defer dm.mux.Unlock()
	dm.domainsByAddress[*d.factoryContractAddress] = d
}

func (dm *domainManager) getDomainByAddress(ctx context.Context, addr *ethtypes.Address0xHex) (d *domain, _ error) {
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

func (dm *domainManager) eventIndexer(ctx context.Context, tx *gorm.DB, batch *blockindexer.EventDeliveryBatch) error {

	var contracts []*OnchainDomain

	for _, ev := range batch.Events {
		switch {
		case ev.Signature.Equals(eventSig_PaladinNewSmartContract_V0):
			var contract OnchainDomain
			parseErr := json.Unmarshal(ev.Data, &contract)
			if parseErr != nil {
				log.L(ctx).Errorf("Failed to parse domain event: %s", types.JSONString(ev))
			} else {
				contracts = append(contracts, &contract)
			}
		}
	}

	// We have some contracts to persist
	if len(contracts) > 0 {
		err := tx.
			Table("onchain_domains").
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "address"}},
				DoNothing: true, // immutable
			}).
			Create(contracts).
			Error
		if err != nil {
			return err
		}
	}

	return nil
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

func mustParseEventSignature(a abi.ABI, eventName string) (v *types.Bytes32) {
	event := a.Events()[eventName]
	if event == nil {
		panic("ABI missing " + eventName)
	}
	sigBytes, err := event.SignatureHash()
	if err != nil {
		panic(err)
	}
	return types.NewBytes32FromSlice(sigBytes)
}
