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

package publictxmgr

import (
	"context"
	"math/big"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
)

// Balance manager is a component that provides the following services
// - retrieve the balance of a given address either from the node or from the cache

type BalanceManagerWithInMemoryTracking struct {
	// transaction handler is retrieve balances from the chain
	pubTxMgr *pubTxManager

	// balance cache is used to store cached balances of any address
	balanceCache cache.Cache[pldtypes.EthAddress, *big.Int]

	// a map of signing addresses and a boolean to indicate whether balance manager should fetch
	// the balance of the signing address from the chain
	retrieveAddressBalanceMap    map[pldtypes.EthAddress]bool
	retrieveAddressBalanceMapMux sync.Mutex
}

func (af *BalanceManagerWithInMemoryTracking) NotifyRetrieveAddressBalance(ctx context.Context, address pldtypes.EthAddress) {
	af.retrieveAddressBalanceMapMux.Lock()
	defer af.retrieveAddressBalanceMapMux.Unlock()
	af.retrieveAddressBalanceMap[address] = true
}

func (af *BalanceManagerWithInMemoryTracking) GetAddressBalance(ctx context.Context, address pldtypes.EthAddress) (*AddressAccount, error) {
	af.retrieveAddressBalanceMapMux.Lock()
	defer af.retrieveAddressBalanceMapMux.Unlock()
	log.L(ctx).Debugf("Retrieving balance for address %s ", address)

	cachedAddressBalance, _ := af.balanceCache.Get(address)
	var addressBalance big.Int
	balanceChangedOnChain := af.retrieveAddressBalanceMap[address]
	if balanceChangedOnChain || cachedAddressBalance == nil {
		log.L(ctx).Debugf("Retrieving balance for address %s from connector", address)
		// fetch the latest balance from the chain
		addressBalancePtr, err := af.pubTxMgr.ethClient.GetBalance(ctx, address, "latest")
		if err != nil {
			log.L(ctx).Errorf("Failed retrieving balance for address %s from connector due to: %+v", address, err)
			return nil, err
		}
		addressBalance = *addressBalancePtr.Int()
		af.balanceCache.Set(address, addressBalancePtr.Int())
		// set the flag to false so that the following requests of this address
		// uses cache if there is no new balance change
		af.retrieveAddressBalanceMap[address] = false
	} else {
		addressBalance = *cachedAddressBalance
		log.L(ctx).Tracef("Retrieved balance for address %s from cache: %s", address, addressBalance.String())
	}
	log.L(ctx).Debugf("Retrieved balance for address %s: %s", address, addressBalance.String())

	return &AddressAccount{
		Address: address,
		Balance: &addressBalance,
		Spent:   big.NewInt(0),
		MinCost: big.NewInt(0),
		MaxCost: big.NewInt(0),
	}, nil
}

func NewBalanceManagerWithInMemoryTracking(ctx context.Context, conf *pldconf.PublicTxManagerConfig, publicTxMgr *pubTxManager) BalanceManager {
	return &BalanceManagerWithInMemoryTracking{
		pubTxMgr:                  publicTxMgr,
		balanceCache:              cache.NewCache[pldtypes.EthAddress, *big.Int](&conf.BalanceManager.Cache, &pldconf.PublicTxManagerDefaults.BalanceManager.Cache),
		retrieveAddressBalanceMap: make(map[pldtypes.EthAddress]bool),
	}
}
