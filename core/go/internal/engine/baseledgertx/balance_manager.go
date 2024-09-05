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

package baseledgertx

import (
	"context"
	"math/big"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/cache"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/components"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/enginespi"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/ethclient"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// Balance manager is a component that provides the following services
// - retrieve the balance of a given address either from the node or from the cache
// - handle auto fueling requests when the feature is turned on

// configurations
const (
	BalanceManagerSection = "balance"

	// Balance cache config
	BalanceManagerCacheEnabled           = "cache.enabled"
	BalanceManagerCacheSizeByteString    = "cache.size"
	BalanceManagerCacheTTLDurationString = "cache.ttl"

	// Auto-fueling config
	BalanceManagerAutoFuelingSection = "autoFueling"

	BalanceManagerAutoFuelingSourceAddressString                     = "sourceAddress"
	BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString = "minSourceBalance"
	BalanceManagerAutoFuelingMinDestBalanceBigIntString              = "minDestBalance"
	BalanceManagerAutoFuelingMaxDestBalanceBigIntString              = "maxDestBalance"

	BalanceManagerAutoFuelingProactiveFuelingTransactionTotalInt = "proactiveFuelingTransactionTotal"
	BalanceManagerAutoFuelingProactiveCostEstimationMethodString = "proactiveCostEstimationMethod"

	BalanceManagerAutoFuelingMinThresholdBigIntString = "minThreshold"
)

type BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod string

const (
	BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMin     BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod = "min"
	BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodAverage BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod = "avg"
	BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMax     BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod = "max"
)

const (
	defaultBalanceManagerAutoFuelingProactiveFuelingTransactionTotal = 1
)

func InitBalanceManagerConfig(conf config.Section) {
	bmConfig := conf.SubSection(BalanceManagerSection)

	// Balance cache config
	bmConfig.AddKnownKey(BalanceManagerCacheEnabled, true)
	bmConfig.AddKnownKey(BalanceManagerCacheSizeByteString, "5m")
	bmConfig.AddKnownKey(BalanceManagerCacheTTLDurationString, "30s")

	// Auto-fueling config
	afConfig := bmConfig.SubSection(BalanceManagerAutoFuelingSection)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingSourceAddressString)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingProactiveFuelingTransactionTotalInt, defaultBalanceManagerAutoFuelingProactiveFuelingTransactionTotal)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingProactiveCostEstimationMethodString, string(BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMax))
	afConfig.AddKnownKey(BalanceManagerAutoFuelingMinDestBalanceBigIntString)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingMaxDestBalanceBigIntString)
	afConfig.AddKnownKey(BalanceManagerAutoFuelingMinThresholdBigIntString)

}

// for test purpose only
func ResetBalanceManagerConfig(conf config.Section) {
	bmConfig := conf.SubSection(BalanceManagerSection)

	bmConfig.Set(BalanceManagerCacheEnabled, true)
	bmConfig.Set(BalanceManagerCacheSizeByteString, "5m")
	bmConfig.Set(BalanceManagerCacheTTLDurationString, "30s")

	afConfig := bmConfig.SubSection(BalanceManagerAutoFuelingSection)
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressString, "")
	afConfig.Set(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, "")
	afConfig.Set(BalanceManagerAutoFuelingProactiveFuelingTransactionTotalInt, defaultBalanceManagerAutoFuelingProactiveFuelingTransactionTotal)
	afConfig.Set(BalanceManagerAutoFuelingProactiveCostEstimationMethodString, string(BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMax))
	afConfig.Set(BalanceManagerAutoFuelingMinDestBalanceBigIntString, "")
	afConfig.Set(BalanceManagerAutoFuelingMaxDestBalanceBigIntString, "")
	afConfig.Set(BalanceManagerAutoFuelingMinThresholdBigIntString, "")

}

type BalanceManagerWithInMemoryTracking struct {
	// ethClient APIs are used to fetch information on chain
	ethClient ethclient.EthClient

	// transaction handler is used to submit and fetch autofueling transaction status
	txEngine baseTypes.BaseLedgerTxEngine

	// balance cache is used to store cached balances of any address
	balanceCache cache.CInterface

	// if set to a valid ethereum address, autofueling is turned on
	sourceAddress string

	// reject autofueling when the source address below this balance
	minSourceBalance *big.Int

	// if number of transactions is below this number, apply multiplier to the spent to calculate the top up amount
	// to fill the extra slots
	proactiveFuelingTransactionTotal int
	// the base to be used by the multiplier
	proactiveFuelingTransactionTotalEmptySlotCostCalcMethod BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod

	// if set, the destination account will at least be topped up to this amount for any fueling tx
	minDestBalance *big.Int
	// if set, the destination account will at most be topped up to this amount for any fueling tx
	maxDestBalance *big.Int

	// if set, any top up request with amount required below this threshold won't happen
	minThreshold *big.Int

	// a map of fueling destination addresses and a mutex to indicate whether it's no longer the first
	// time the current balance manager instance is handling fueling request to this destination address.
	// When the mutex is set, balance manager will confidently use the internal trackedFuelingTransactions map
	// to query the status of existing fueling requests and replace completed tracked fueling transactions with new ones
	// When the mutex is not set, balance manager will query transaction persistence to fetch the in-flight
	// fueling transactions by search for the latest transfer type transaction to the destination address. (this involves
	//  read from database and is necessary to recover balance manager from crashes)
	destinationAddressesFuelingTracked    map[string]*sync.Mutex
	trackedFuelingTransactions            map[string]*baseTypes.ManagedTX
	destinationAddressesFuelingTrackedMux sync.Mutex

	// a map of signing addresses and a boolean to indicate whether balance manager should fetch
	// the balance of the signing address from the chain
	addressBalanceChangedMap    map[string]bool
	addressBalanceChangedMapMux sync.Mutex
}

func (af *BalanceManagerWithInMemoryTracking) TopUpAccount(ctx context.Context, addAccount *baseTypes.AddressAccount) (mtx *baseTypes.ManagedTX, err error) {
	if af.sourceAddress == "" {
		log.L(ctx).Debugf("Skip top up transaction as no fueling source configured")
		// No-op
		return nil, nil
	}

	if af.maxDestBalance != nil && af.maxDestBalance.Cmp(addAccount.Balance) < 0 {
		// account already reached maximum balance, no op
		log.L(ctx).Debugf("Skip top up transaction as target account %s, has %s balance which is higher than the configured max top up %s", addAccount.Address, addAccount.Balance.String(), af.maxDestBalance.String())
		return nil, nil
	}
	log.L(ctx).Debugf("Calculate the amount to be topped up for address %+v ; autoFueling config: %+v", addAccount, af)

	if addAccount.Spent.Sign() > 0 && addAccount.Spent.Cmp(addAccount.Balance) > 0 {
		spentCopy := new(big.Int).Set(addAccount.Spent)
		topUpAmount := spentCopy.Sub(spentCopy, addAccount.Balance)
		if af.proactiveFuelingTransactionTotal > addAccount.SpentTransactionCount {
			// when we don't have enough (minimum fuel ahead) number of transactions
			// we use the configured calculation methods to calculate the value for the empty slots to fill
			extraFillAmountInt := big.NewInt(int64(af.proactiveFuelingTransactionTotal - addAccount.SpentTransactionCount))
			switch af.proactiveFuelingTransactionTotalEmptySlotCostCalcMethod {
			case BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMin:
				topUpAmount = topUpAmount.Add(topUpAmount, addAccount.MinCost.Mul(addAccount.MinCost, extraFillAmountInt))
			case BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodMax:
				topUpAmount = topUpAmount.Add(topUpAmount, addAccount.MaxCost.Mul(addAccount.MaxCost, extraFillAmountInt))
			case BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethodAverage:
				spentTransactionCountBigInt := big.NewInt(int64(addAccount.SpentTransactionCount))
				spentCopy = new(big.Int).Set(addAccount.Spent)
				avgAmount := spentCopy.Div(spentCopy, spentTransactionCountBigInt)
				topUpAmount = topUpAmount.Add(topUpAmount, avgAmount.Mul(avgAmount, extraFillAmountInt))
			}
		}

		// after proactiveFuelingTransactionTotal check, we'll do the threshold check if set
		if af.minDestBalance != nil {
			balanceCopy := new(big.Int)
			balanceCopy.Set(addAccount.Balance)
			newBalance := balanceCopy.Add(balanceCopy, topUpAmount)
			if af.minDestBalance.Cmp(newBalance) > 0 {
				// top up value below minimum, increase it
				minDestBalanceCopy := new(big.Int)
				minDestBalanceCopy.Set(af.minDestBalance)

				topUpAmount = minDestBalanceCopy.Sub(minDestBalanceCopy, addAccount.Balance)
			}
		}

		if af.maxDestBalance != nil {
			balanceCopy := new(big.Int)
			balanceCopy.Set(addAccount.Balance)
			newBalance := balanceCopy.Add(balanceCopy, topUpAmount)
			if af.maxDestBalance.Cmp(newBalance) < 0 {
				// top up value beyond maximum, decrease it
				maxDestBalanceCopy := new(big.Int)
				maxDestBalanceCopy.Set(af.maxDestBalance)

				topUpAmount = maxDestBalanceCopy.Sub(maxDestBalanceCopy, addAccount.Balance)
			}
		}

		if af.minThreshold != nil && af.minThreshold.Cmp(topUpAmount) > 0 {
			// top up amount too low, do not submit any fueling transaction
			log.L(ctx).Debugf("Skipped top up for address %s as calculated amount: %s is below the min threshold %s", addAccount.Address, topUpAmount.String(), af.minThreshold.String())
			return nil, nil
		}
		log.L(ctx).Debugf("Requesting top up for address %s using calculated amount: %s based on spent: %s", addAccount.Address, topUpAmount.String(), addAccount.Spent.String())
		// after all the above amount tuning, do a final threshold check if there is one
		return af.TransferGasFromAutoFuelingSource(ctx, addAccount.Address, topUpAmount)
	}
	return nil, nil
}

func (af *BalanceManagerWithInMemoryTracking) NotifyAddressBalanceChanged(ctx context.Context, address string) {
	af.addressBalanceChangedMapMux.Lock()
	defer af.addressBalanceChangedMapMux.Unlock()
	af.addressBalanceChangedMap[address] = true
}

func (af *BalanceManagerWithInMemoryTracking) IsAutoFuelingEnabled(ctx context.Context) bool {
	return af.sourceAddress != ""
}

func (af *BalanceManagerWithInMemoryTracking) GetAddressBalance(ctx context.Context, address string) (*baseTypes.AddressAccount, error) {
	af.addressBalanceChangedMapMux.Lock()
	defer af.addressBalanceChangedMapMux.Unlock()
	log.L(ctx).Debugf("Retrieving balance for address %s ", address)

	cachedAddressBalance := af.balanceCache.Get(address)
	var addressBalance big.Int
	balanceChangedOnChain := af.addressBalanceChangedMap[address]
	if balanceChangedOnChain || cachedAddressBalance == nil {
		log.L(ctx).Debugf("Retrieving balance for address %s from connector", address)
		// fetch the latest balance from the chain
		addressBalancePtr, err := af.ethClient.GetBalance(ctx, address, "latest")
		if err != nil {
			log.L(ctx).Errorf("Failed retrieving balance for address %s from connector due to: %+v", address, err)
			return nil, err
		}
		addressBalance = *addressBalancePtr.BigInt()
		af.balanceCache.Set(address, addressBalance)
		// set the flag to false so that the following requests of this address
		// uses cache if there is no new balance change
		af.addressBalanceChangedMap[address] = false
	} else {
		addressBalance = cachedAddressBalance.(big.Int)
		log.L(ctx).Tracef("Retrieved balance for address %s from cache: %s", address, addressBalance.String())
	}
	log.L(ctx).Debugf("Retrieved balance for address %s: %s", address, addressBalance.String())

	return &baseTypes.AddressAccount{
		Address: address,
		Balance: &addressBalance,
		Spent:   big.NewInt(0),
		MinCost: big.NewInt(0),
		MaxCost: big.NewInt(0),
	}, nil
}

func (af *BalanceManagerWithInMemoryTracking) TransferGasFromAutoFuelingSource(ctx context.Context, destAddress string, value *big.Int) (mtx *baseTypes.ManagedTX, err error) {
	// check whether there is a pending fueling transaction already
	// check whether the current balance manager already tracking the existing in-flight fueling transactions
	log.L(ctx).Tracef("TransferGasFromAutoFuelingSource entry, source address: %s, destination address: %s, amount: %s", af.sourceAddress, destAddress, value.String())

	var fuelingTx *baseTypes.ManagedTX
	af.destinationAddressesFuelingTrackedMux.Lock()
	perAddressMux, ok := af.destinationAddressesFuelingTracked[destAddress] // there is no lock here as the map of tracked transactions is the one that is critical to get right
	if !ok {
		perAddressMux = &sync.Mutex{}
		af.destinationAddressesFuelingTracked[destAddress] = perAddressMux
	}
	perAddressMux.Lock()
	defer perAddressMux.Unlock()
	af.destinationAddressesFuelingTrackedMux.Unlock()
	fuelingTx = af.trackedFuelingTransactions[destAddress]
	if fuelingTx == nil {
		log.L(ctx).Debugf("TransferGasFromAutoFuelingSource no existing tracking fueling request for  destination address: %s", destAddress)
		// there is no tracked fueling transaction for this address, do a lookup in the db in case we've restarted or couldn't record the last one submitted
		// in the middle of tracking
		fuelingTx, err = af.txEngine.GetPendingFuelingTransaction(ctx, af.sourceAddress, destAddress)
		if err != nil {
			log.L(ctx).Errorf("TransferGasFromAutoFuelingSource error occurred when getting pending fueling tx for address: %s, error: %+v", destAddress, err)
			// we don't risk the chance of having duplicate fueling transactions when we cannot fetching all the in-flight transactions
			return nil, err
		}
		af.trackedFuelingTransactions[destAddress] = fuelingTx
	}
	if fuelingTx != nil && !af.txEngine.CheckTransactionCompleted(ctx, fuelingTx) {
		log.L(ctx).Debugf("TransferGasFromAutoFuelingSource fueling request with ID: %s for  destination address: %s still not complete", fuelingTx.ID, destAddress)
		// transaction is tracked and is still pending, return the transaction as it is
		return fuelingTx, nil
	}

	// otherwise, new fueling tx is required

	// clean up the existing tracked transaction
	af.trackedFuelingTransactions[destAddress] = nil

	// 1) Check balance of source address to ensure we have enough to transfer
	sourceAccount, err := af.GetAddressBalance(ctx, af.sourceAddress)

	if err != nil {
		log.L(ctx).Errorf("TransferGasFromAutoFuelingSource failed to get balance of source: %s", af.sourceAddress)
		return nil, err
	}
	log.L(ctx).Tracef("TransferGasFromAutoFuelingSource source balance: (%v)", sourceAccount.Balance.String())

	if af.minSourceBalance != nil && sourceAccount.Balance.Cmp(af.minSourceBalance) < 0 {
		log.L(ctx).Errorf("TransferGasFromAutoFuelingSource source balance of %s: %s is below the configured minimum: %s", sourceAccount.Address, sourceAccount.Balance.String(), af.minSourceBalance.String())
		// if the balance of the source account goes below configured minimum, we return an error to the caller to decide what to do
		return nil, i18n.NewError(ctx, msgs.MsgBalanceBelowMinimum, sourceAccount.Balance.String(), sourceAccount.Address, af.minSourceBalance.String())
	}

	if sourceAccount.Balance.Cmp(value) < 0 {
		log.L(ctx).Errorf("TransferGasFromAutoFuelingSource source balance of %s: %s is below the requested amount: %s", sourceAccount.Address, sourceAccount.Balance.String(), value.String())
		// if the balance of the source account is not enough to cover the requested amount ,we return an error to the caller to decide what to do
		return nil, i18n.NewError(ctx, msgs.MsgInsufficientBalance, sourceAccount.Balance.String(), sourceAccount.Address, value.String())
	}

	// for the situation of the requested value + gas fee is greater than the balance, we only figure this out after the new transaction is executed

	// 2) Perform transaction to transfer value to the dest address

	log.L(ctx).Debugf("TransferGasFromAutoFuelingSource submitting a fueling tx for  destination address: %s ", destAddress)
	txID := uuid.New()
	mtx, _, err = af.txEngine.HandleNewTransaction(ctx, &baseTypes.RequestOptions{
		ID:       &txID,
		SignerID: af.sourceAddress,
	}, &components.EthTransfer{
		To:    *types.MustEthAddress(destAddress),
		Value: ethtypes.NewHexInteger(value),
	})

	if err != nil {
		log.L(ctx).Errorf("TransferGasFromAutoFuelingSource fueling tx submission for destination address: %s failed due to: %+v", destAddress, err)
		return nil, err
	}
	log.L(ctx).Debugf("TransferGasFromAutoFuelingSource tracking fueling tx with ID %s, for destination address: %s ", mtx.ID, destAddress)
	// start tracking the new transactions
	af.trackedFuelingTransactions[destAddress] = mtx
	return mtx, nil
}

func NewBalanceManagerWithInMemoryTracking(ctx context.Context, conf config.Section, ethClient ethclient.EthClient, txEngine baseTypes.BaseLedgerTxEngine) (baseTypes.BalanceManager, error) {
	cm, _ := cache.NewCacheManager(ctx, true).GetCache(ctx, "balance-manager", "balance", conf.GetByteSize(BalanceManagerCacheSizeByteString), conf.GetDuration(BalanceManagerCacheTTLDurationString), conf.GetBool(BalanceManagerCacheEnabled), cache.StrictExpiry, cache.TTLFromInitialAdd)
	log.L(ctx).Debugf("Balance manager cache setting. Enabled: %t , size: %d , ttl: %s", conf.GetBool(BalanceManagerCacheEnabled), conf.GetByteSize(BalanceManagerCacheSizeByteString), conf.GetDuration(BalanceManagerCacheTTLDurationString))
	afConfig := conf.SubSection(BalanceManagerAutoFuelingSection)

	var minSourceBalance *big.Int
	minSourceAddressBalanceString := afConfig.GetString(BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString)
	if minSourceAddressBalanceString != "" {
		minSourceBalance = &big.Int{}
		_, ok := minSourceBalance.SetString(minSourceAddressBalanceString, 10)
		if !ok {
			log.L(ctx).Errorf("Failed to parse %s into a bigInt", minSourceAddressBalanceString)
			return nil, i18n.NewError(ctx, msgs.MsgInvalidBigIntString, BalanceManagerAutoFuelingSourceAddressMinimumBalanceBigIntString, minSourceAddressBalanceString)
		}
		log.L(ctx).Debugf("Balance manager minSourceBalance setting: %s", minSourceBalance.String())
	}

	var minDestBalance *big.Int
	minDestBalanceString := afConfig.GetString(BalanceManagerAutoFuelingMinDestBalanceBigIntString)
	if minDestBalanceString != "" {
		minDestBalance = &big.Int{}
		_, ok := minDestBalance.SetString(minDestBalanceString, 10)
		if !ok {
			log.L(ctx).Errorf("Failed to parse %s into a bigInt", minDestBalanceString)
			return nil, i18n.NewError(ctx, msgs.MsgInvalidBigIntString, BalanceManagerAutoFuelingMinDestBalanceBigIntString, minDestBalanceString)
		}
		log.L(ctx).Debugf("Balance manager minDestBalance setting: %s", minDestBalance.String())
	}

	var maxDestBalance *big.Int
	maxDestBalanceString := afConfig.GetString(BalanceManagerAutoFuelingMaxDestBalanceBigIntString)
	if maxDestBalanceString != "" {
		maxDestBalance = &big.Int{}
		_, ok := maxDestBalance.SetString(maxDestBalanceString, 10)
		if !ok {
			log.L(ctx).Errorf("Failed to parse %s into a bigInt", maxDestBalanceString)
			return nil, i18n.NewError(ctx, msgs.MsgInvalidBigIntString, BalanceManagerAutoFuelingMaxDestBalanceBigIntString, maxDestBalanceString)
		}
		log.L(ctx).Debugf("Balance manager maxDestBalance setting: %s", maxDestBalance.String())
	}

	var minThreshold *big.Int
	minThresholdString := afConfig.GetString(BalanceManagerAutoFuelingMinThresholdBigIntString)
	if minThresholdString != "" {
		minThreshold = &big.Int{}
		_, ok := minThreshold.SetString(minThresholdString, 10)
		if !ok {
			log.L(ctx).Errorf("Failed to parse %s into a bigInt", minThresholdString)
			return nil, i18n.NewError(ctx, msgs.MsgInvalidBigIntString, BalanceManagerAutoFuelingMinThresholdBigIntString, minThresholdString)
		}
		log.L(ctx).Debugf("Balance manager minThreshold setting: %s", minThreshold.String())
	}

	if maxDestBalance != nil && minDestBalance != nil {
		if maxDestBalance.Cmp(minDestBalance) < 0 {
			log.L(ctx).Errorf("Failed initialization due to maxDestBalance is not greater than minDestBalance")
			return nil, i18n.NewError(ctx, msgs.MsgMaxBelowMin, BalanceManagerAutoFuelingMaxDestBalanceBigIntString, maxDestBalanceString, minDestBalanceString)
		}
	}

	if maxDestBalance != nil && minThreshold != nil {
		if maxDestBalance.Cmp(minThreshold) < 0 {
			log.L(ctx).Errorf("Failed initialization due to maxDestBalance is not greater than minThreshold")
			return nil, i18n.NewError(ctx, msgs.MsgMaxBelowMinThreshold, BalanceManagerAutoFuelingMaxDestBalanceBigIntString, maxDestBalanceString, minThresholdString)
		}
	}
	calcMethod := afConfig.GetString(BalanceManagerAutoFuelingProactiveCostEstimationMethodString)
	log.L(ctx).Debugf("Balance manager calcMethod setting: %s", calcMethod)
	bm := &BalanceManagerWithInMemoryTracking{
		sourceAddress:                    afConfig.GetString(BalanceManagerAutoFuelingSourceAddressString),
		ethClient:                        ethClient,
		txEngine:                         txEngine,
		balanceCache:                     cm,
		minSourceBalance:                 minSourceBalance,
		proactiveFuelingTransactionTotal: afConfig.GetInt(BalanceManagerAutoFuelingProactiveFuelingTransactionTotalInt),
		proactiveFuelingTransactionTotalEmptySlotCostCalcMethod: BalanceManagerAutoFuelingProactiveFuelingTransactionTotalEmptySlotCostCalcMethod(calcMethod),
		minDestBalance:                     minDestBalance,
		maxDestBalance:                     maxDestBalance,
		minThreshold:                       minThreshold,
		destinationAddressesFuelingTracked: make(map[string]*sync.Mutex),
		trackedFuelingTransactions:         make(map[string]*baseTypes.ManagedTX),
		addressBalanceChangedMap:           make(map[string]bool),
	}
	return bm, nil
}
