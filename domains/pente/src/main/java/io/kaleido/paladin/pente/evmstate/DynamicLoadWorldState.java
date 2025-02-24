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

 package io.kaleido.paladin.pente.evmstate;

 import io.kaleido.paladin.logging.PaladinLogging;
 import org.apache.logging.log4j.Logger;
 import org.apache.tuweni.bytes.Bytes32;
 import org.hyperledger.besu.datatypes.Address;
 import org.hyperledger.besu.datatypes.Hash;
 import org.hyperledger.besu.evm.account.Account;
 import org.hyperledger.besu.evm.internal.EvmConfiguration;
 import org.hyperledger.besu.evm.worldstate.AbstractWorldUpdater;
 import org.hyperledger.besu.evm.worldstate.UpdateTrackingAccount;
 import org.hyperledger.besu.evm.worldstate.WorldUpdater;
 
 import java.io.IOException;
 import java.util.*;
 import java.util.stream.Stream;
 
 public class DynamicLoadWorldState implements org.hyperledger.besu.evm.worldstate.WorldState {
 
     private static final Logger logger = PaladinLogging.getLogger(DynamicLoadWorldState.class);
 
     private final AccountLoader accountLoader;
 
     private final Updater updater;
 
     public DynamicLoadWorldState(AccountLoader accountLoader, EvmConfiguration evmConfiguration) {
         this.accountLoader = accountLoader;
         this.updater = new Updater(evmConfiguration);
     }
 
     final Map<Address, PersistedAccount> accounts = new HashMap<>();
 
     private final Set<Address> queriedAccounts = new HashSet<>();
 
     public enum LastOpType {
         UPDATED, DELETED
     }
 
     private final Map<Address, LastOpType> committedAccountUpdates = new HashMap<>();
 
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
     public PersistedAccount get(Address address) {
         queriedAccounts.add(address);
         PersistedAccount account = accounts.get(address);
         if (account == null) {
             try {
                 Optional<PersistedAccount> loadedAccount = accountLoader.load(address);
                 if (loadedAccount.isPresent()) {
                     account = loadedAccount.get();
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
 
     public Map<Address, LastOpType> getCommittedAccountUpdates() {
         return Collections.unmodifiableMap(committedAccountUpdates);
     }
 
     private void setAccount(PersistedAccount account) {
         this.accounts.put(account.getAddress(), account);
     }
 
     private void deleteAccount(Address account) {
         this.accounts.remove(account);
     }
 
     private class Updater extends AbstractWorldUpdater<DynamicLoadWorldState, PersistedAccount> {
 
         public Updater(EvmConfiguration evmConfiguration) {
             super(DynamicLoadWorldState.this, evmConfiguration);
         }
 
         @Override
         protected PersistedAccount getForMutation(Address address) {
             return DynamicLoadWorldState.this.get(address);
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
             // We propagate the changes to the world, and the world is responsible
             // for tracking the full list of accessed accounts.
             for (UpdateTrackingAccount<PersistedAccount> account : getUpdatedAccounts()) {
                 logger.debug("updated account: {}", account);
                 PersistedAccount baseAccount = account.getWrappedAccount();
                 if (baseAccount == null) {
                     // This is a new account generated in this commit
                     baseAccount = new PersistedAccount(account.getAddress());
                 }
                 // TODO: Consider persisting the change list
                 baseAccount.applyChanges(account);
                 DynamicLoadWorldState.this.setAccount(baseAccount);
                 committedAccountUpdates.put(account.getAddress(), LastOpType.UPDATED);
             }
             for (Address account : getDeletedAccounts()) {
                 logger.debug("deleted account: {}", account);
                 DynamicLoadWorldState.this.deleteAccount(account);
                 committedAccountUpdates.put(account, LastOpType.DELETED);
             }
         }
     }
 }
 