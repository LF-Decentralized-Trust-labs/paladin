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

package io.kaleido.pente.evmstate;

import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.evm.account.Account;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.worldstate.AbstractWorldUpdater;
import org.hyperledger.besu.evm.worldstate.UpdateTrackingAccount;
import org.apache.tuweni.bytes.Bytes32;
import org.hyperledger.besu.evm.worldstate.WorldUpdater;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.util.*;
import java.util.stream.Stream;

public class DynamicLoadWorldState implements org.hyperledger.besu.evm.worldstate.WorldState {

    private final Logger logger = LoggerFactory.getLogger(io.kaleido.pente.evmstate.DynamicLoadWorldState.class);

    private final AccountLoader accountLoader;

    private final Updater updater;

    public DynamicLoadWorldState(AccountLoader accountLoader, EvmConfiguration evmConfiguration) {
        this.accountLoader = accountLoader;
        this.updater = new Updater(evmConfiguration);
    }

    final Map<Address, UpdateTrackingAccount<Account>> accounts = new HashMap<>();

    private final Set<Address> queriedAccounts = new HashSet<>();

    @Override
    public Hash rootHash() {
        return null;
    }

    @Override
    public Hash frontierRootHash() {
        return null;
    }

    @Override
    public Stream<StreamableAccount> streamAccounts(Bytes32 bytes32, int i) {
        return null;
    }

    @Override
    public UpdateTrackingAccount<Account> get(Address address) {
        queriedAccounts.add(address);
        UpdateTrackingAccount<Account> account = accounts.get(address);
        if (account == null) {
            try {
                Optional<Account> loadedAccount = accountLoader.load(address);
                if (loadedAccount.isPresent()) {
                    account = new UpdateTrackingAccount<>(loadedAccount.get());
                    accounts.put(address, account);
                }
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        }
        return account;
    }

    public WorldUpdater getUpdater() {
        return updater;
    }

    public Collection<Address> getQueriedAccounts() {
        return Collections.unmodifiableSet(queriedAccounts);
    }

    private void setAccount(UpdateTrackingAccount<Account> account) {
        this.accounts.put(account.getAddress(), account);
    }

    private void deleteAccount(Address account) {
        this.accounts.remove(account);
    }

    private class Updater extends AbstractWorldUpdater<DynamicLoadWorldState, Account> {

        public Updater(EvmConfiguration evmConfiguration) {
            super(DynamicLoadWorldState.this, evmConfiguration);
        }

        @Override
        protected UpdateTrackingAccount<Account> getForMutation(Address address) {
            return  DynamicLoadWorldState.this.get(address);
        }

        @Override
        public Collection<? extends Account> getTouchedAccounts() {
            return getUpdatedAccounts();
        }

        @Override
        public Collection<Address> getDeletedAccountAddresses() {
            return deletedAccounts;
        }

        @Override
        public void revert() {
            logger.debug("reverted");
            super.reset();
        }

        @Override
        public void commit() {
            // This gets called on the COMPLETED_SUCCESS boundary of every frame.
            // So within a single transaction it might be called multiple times.
            //
            // We simply use it to propagate the changes to the world, and the world is responsible
            // for tracking the full list of accessed accounts.
            for (Account account : getTouchedAccounts()) {
                logger.debug("updated account: {}", account);
                DynamicLoadWorldState.this.setAccount(this.updatedAccounts.get(account.getAddress()));
            }
            for (Address account : getDeletedAccounts()) {
                logger.debug("deleted account: {}", account);
                DynamicLoadWorldState.this.deleteAccount(account);
            }
        }
    }
}
