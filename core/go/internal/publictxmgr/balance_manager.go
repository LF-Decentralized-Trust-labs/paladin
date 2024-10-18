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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

// Balance manager is a component that provides the following services
// - retrieve the balance of a given address either from the node or from the cache
// - handle auto fueling requests when the feature is turned on

type BalanceManagerWithInMemoryTracking struct {
	// transaction handler is used to submit and fetch autofueling transaction status
	pubTxMgr *pubTxManager

	// balance cache is used to store cached balances of any address
	balanceCache cache.Cache[tktypes.EthAddress, *big.Int]

	// the unresolved signer to use when submitting transactions
	source string

	// if set to a valid ethereum address, autofueling is turned on
	sourceAddress *tktypes.EthAddress

	// reject autofueling when the source address below this balance
	minSourceBalance *big.Int

	// if number of transactions is below this number, apply multiplier to the spent to calculate the top up amount
	// to fill the extra slots
	proactiveFuelingTransactionTotal int
	// the base to be used by the multiplier
	proactiveFuelingCalcMethod pldconf.ProactiveAutoFuelingCalcMethod

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
	destinationAddressesFuelingTracked    map[tktypes.EthAddress]*sync.Mutex
	trackedFuelingTransactions            map[tktypes.EthAddress]*pldapi.PublicTx
	destinationAddressesFuelingTrackedMux sync.Mutex

	// a map of signing addresses and a boolean to indicate whether balance manager should fetch
	// the balance of the signing address from the chain
	addressBalanceChangedMap    map[tktypes.EthAddress]bool
	addressBalanceChangedMapMux sync.Mutex
}

func (af *BalanceManagerWithInMemoryTracking) TopUpAccount(ctx context.Context, addAccount *AddressAccount) (mtx *pldapi.PublicTx, err error) {
	if af.sourceAddress == nil {
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
			switch af.proactiveFuelingCalcMethod {
			case pldconf.ProactiveAutoFuelingCalcMethodMin:
				topUpAmount = topUpAmount.Add(topUpAmount, addAccount.MinCost.Mul(addAccount.MinCost, extraFillAmountInt))
			case pldconf.ProactiveAutoFuelingCalcMethodMax:
				topUpAmount = topUpAmount.Add(topUpAmount, addAccount.MaxCost.Mul(addAccount.MaxCost, extraFillAmountInt))
			case pldconf.ProactiveAutoFuelingCalcMethodAverage:
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

func (af *BalanceManagerWithInMemoryTracking) NotifyAddressBalanceChanged(ctx context.Context, address tktypes.EthAddress) {
	af.addressBalanceChangedMapMux.Lock()
	defer af.addressBalanceChangedMapMux.Unlock()
	af.addressBalanceChangedMap[address] = true
}

func (af *BalanceManagerWithInMemoryTracking) IsAutoFuelingEnabled(ctx context.Context) bool {
	return af.sourceAddress != nil
}

func (af *BalanceManagerWithInMemoryTracking) GetAddressBalance(ctx context.Context, address tktypes.EthAddress) (*AddressAccount, error) {
	af.addressBalanceChangedMapMux.Lock()
	defer af.addressBalanceChangedMapMux.Unlock()
	log.L(ctx).Debugf("Retrieving balance for address %s ", address)

	cachedAddressBalance, _ := af.balanceCache.Get(address)
	var addressBalance big.Int
	balanceChangedOnChain := af.addressBalanceChangedMap[address]
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
		af.addressBalanceChangedMap[address] = false
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

func (af *BalanceManagerWithInMemoryTracking) TransferGasFromAutoFuelingSource(ctx context.Context, destAddress tktypes.EthAddress, value *big.Int) (fuelingTx *pldapi.PublicTx, err error) {
	// check whether there is a pending fueling transaction already
	// check whether the current balance manager already tracking the existing in-flight fueling transactions
	log.L(ctx).Tracef("TransferGasFromAutoFuelingSource entry, source address: %s, destination address: %s, amount: %s", af.sourceAddress, destAddress, value.String())

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
		fuelingTx, err = af.pubTxMgr.GetPendingFuelingTransaction(ctx, *af.sourceAddress, destAddress)
		if err != nil {
			log.L(ctx).Errorf("TransferGasFromAutoFuelingSource error occurred when getting pending fueling tx for address: %s, error: %+v", destAddress, err)
			// we don't risk the chance of having duplicate fueling transactions when we cannot fetching all the in-flight transactions
			return nil, err
		}
		if fuelingTx != nil {
			af.trackedFuelingTransactions[destAddress] = fuelingTx
		}
	}
	if fuelingTx != nil {
		completed, err := af.pubTxMgr.CheckTransactionCompleted(ctx, fuelingTx.From, fuelingTx.Nonce.Uint64())
		if err != nil {
			return nil, err
		}
		if !completed {
			log.L(ctx).Debugf("TransferGasFromAutoFuelingSource fueling request from=%s nonce=%d for destination address: %s still not complete", fuelingTx.From, fuelingTx.Nonce, destAddress)
			// transaction is tracked and is still pending, return the transaction as it is
			return fuelingTx, nil
		}
	}

	// otherwise, new fueling tx is required

	// clean up the existing tracked transaction
	delete(af.trackedFuelingTransactions, destAddress)

	// 1) Check balance of source address to ensure we have enough to transfer
	sourceAccount, err := af.GetAddressBalance(ctx, *af.sourceAddress)

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
	submission, err := af.pubTxMgr.SingleTransactionSubmit(ctx, &components.PublicTxSubmission{
		PublicTxInput: pldapi.PublicTxInput{
			From: af.sourceAddress,
			To:   &destAddress,
			PublicTxOptions: pldapi.PublicTxOptions{
				Value: (*tktypes.HexUint256)(value),
			},
		},
	})

	if err != nil {
		log.L(ctx).Errorf("TransferGasFromAutoFuelingSource fueling tx submission for destination address: %s failed due to: %+v", destAddress, err)
		return nil, err
	}
	fuelingTx = submission.PublicTx()
	log.L(ctx).Debugf("TransferGasFromAutoFuelingSource tracking fueling tx with from=%s nonce=%d, for destination address: %s ", fuelingTx.From, fuelingTx.Nonce, destAddress)
	// start tracking the new transactions
	af.trackedFuelingTransactions[destAddress] = fuelingTx
	return fuelingTx, nil
}

func NewBalanceManagerWithInMemoryTracking(ctx context.Context, conf *pldconf.PublicTxManagerConfig, publicTxMgr *pubTxManager) (_ BalanceManager, err error) {

	minSourceBalance := confutil.BigIntOrNil(conf.BalanceManager.AutoFueling.MinDestBalance)
	minDestBalance := confutil.BigIntOrNil(conf.BalanceManager.AutoFueling.MinDestBalance)
	maxDestBalance := confutil.BigIntOrNil(conf.BalanceManager.AutoFueling.MaxDestBalance)
	minThreshold := confutil.BigIntOrNil(conf.BalanceManager.AutoFueling.MinThreshold)

	if maxDestBalance != nil && minDestBalance != nil {
		if maxDestBalance.Cmp(minDestBalance) < 0 {
			log.L(ctx).Errorf("Failed initialization due to maxDestBalance is not greater than minDestBalance")
			return nil, i18n.NewError(ctx, msgs.MsgMaxBelowMin, "maxDestBalance")
		}
	}

	if maxDestBalance != nil && minThreshold != nil {
		if maxDestBalance.Cmp(minThreshold) < 0 {
			log.L(ctx).Errorf("Failed initialization due to maxDestBalance is not greater than minThreshold")
			return nil, i18n.NewError(ctx, msgs.MsgMaxBelowMinThreshold, "maxDestBalance")
		}
	}
	var autoFuelingSourceAddress *tktypes.EthAddress
	autoFuelingSource := confutil.StringOrEmpty(conf.BalanceManager.AutoFueling.Source, "")
	if autoFuelingSource != "" {
		// We must be able to resolve the supplied auto fueling source at startup, so we can check its balance
		resolved, err := publicTxMgr.keymgr.ResolveKeyNewDatabaseTX(ctx, autoFuelingSource, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		if err == nil {
			autoFuelingSourceAddress, err = tktypes.ParseEthAddress(resolved.Verifier.Verifier)
		}
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidAutoFuelSource, autoFuelingSource)
		}
	}
	calcMethod := confutil.StringNotEmpty(conf.BalanceManager.AutoFueling.ProactiveCostEstimationMethod, string(pldconf.ProactiveAutoFuelingCalcMethodMax))
	log.L(ctx).Debugf("Balance manager calcMethod setting: %s", calcMethod)
	bm := &BalanceManagerWithInMemoryTracking{
		source:                             autoFuelingSource,
		sourceAddress:                      autoFuelingSourceAddress,
		pubTxMgr:                           publicTxMgr,
		balanceCache:                       cache.NewCache[tktypes.EthAddress, *big.Int](&conf.BalanceManager.Cache, &pldconf.PublicTxManagerDefaults.BalanceManager.Cache),
		minSourceBalance:                   minSourceBalance,
		proactiveFuelingTransactionTotal:   confutil.IntMin(conf.BalanceManager.AutoFueling.ProactiveFuelingTransactionTotal, 0, *pldconf.PublicTxManagerDefaults.BalanceManager.AutoFueling.ProactiveFuelingTransactionTotal),
		proactiveFuelingCalcMethod:         pldconf.ProactiveAutoFuelingCalcMethod(calcMethod),
		minDestBalance:                     minDestBalance,
		maxDestBalance:                     maxDestBalance,
		minThreshold:                       minThreshold,
		destinationAddressesFuelingTracked: make(map[tktypes.EthAddress]*sync.Mutex),
		trackedFuelingTransactions:         make(map[tktypes.EthAddress]*pldapi.PublicTx),
		addressBalanceChangedMap:           make(map[tktypes.EthAddress]bool),
	}
	return bm, nil
}
