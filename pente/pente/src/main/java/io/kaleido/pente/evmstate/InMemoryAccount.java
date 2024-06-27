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

import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.Bytes32;
import org.apache.tuweni.units.bigints.UInt256;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.ethereum.trie.MerkleTrie;
import org.hyperledger.besu.ethereum.trie.SimpleMerkleTrie;
import org.hyperledger.besu.ethereum.trie.patricia.SimpleMerklePatriciaTrie;
import org.hyperledger.besu.evm.ModificationNotAllowedException;
import org.hyperledger.besu.evm.account.AccountStorageEntry;
import org.hyperledger.besu.evm.account.MutableAccount;
import org.web3j.rlp.RlpDecoder;
import org.web3j.rlp.RlpString;

import java.util.*;

public class InMemoryAccount implements MutableAccount {

    private final Address address;

    private long nonce;

    private Wei balance;

    private Bytes code;

    private final Map<UInt256, UInt256> updatedValues = new HashMap<>();

    private final MerkleTrie<Bytes32, Bytes> originalStorageTrie;

    private final Map<Bytes32, Optional<UInt256>> storageKeys = new HashMap<>();

    private boolean immutable = false;

    public InMemoryAccount(Address address, MerkleTrie<Bytes32, Bytes> originalStorageTrie) {
        this.address = address;
        this.originalStorageTrie = new SimpleMerklePatriciaTrie<>(b -> b);
    }

    @Override
    public void setNonce(long l) {
        if (immutable) {
            throw new ModificationNotAllowedException();
        }
        this.nonce = l;
    }

    @Override
    public void setBalance(Wei wei) {
        if (immutable) {
            throw new ModificationNotAllowedException();
        }
        this.balance = wei;
    }

    @Override
    public void setCode(Bytes bytes) {
        if (immutable) {
            throw new ModificationNotAllowedException();
        }
        this.code = bytes;
    }

    @Override
    public void setStorageValue(UInt256 k, UInt256 v) {
        if (immutable) {
            throw new ModificationNotAllowedException();
        }
        this.updatedValues.put(k,v);
    }

    @Override
    public void clearStorage() {
        this.updatedValues.clear();
    }

    @Override
    public Map<UInt256, UInt256> getUpdatedStorage() {
        return this.updatedValues;
    }

    @Override
    public void becomeImmutable() {
        this.immutable = true;
    }

    @Override
    public Address getAddress() {
        return this.address;
    }

    @Override
    public boolean isStorageEmpty() {
        return this.updatedValues.isEmpty();
    }

    @Override
    public Hash getAddressHash() {
        return this.address.addressHash();
    }

    @Override
    public long getNonce() {
        return this.nonce;
    }

    @Override
    public Wei getBalance() {
        return this.balance;
    }

    @Override
    public Bytes getCode() {
        return this.code;
    }

    @Override
    public Hash getCodeHash() {
        return Hash.hash(this.code);
    }

    @Override
    public UInt256 getStorageValue(UInt256 k) {
        if (this.updatedValues.containsValue(k)) {
            return this.updatedValues.get(k);
        }
        Optional<Bytes> v = this.originalStorageTrie.get(Hash.hash(k));
        return v.map(InMemoryAccount::convertToUInt256).orElse(null);
    }

    @Override
    public UInt256 getOriginalStorageValue(UInt256 uInt256) {
        // TODO: Why is this needed (
        throw new UnsupportedOperationException("not yet supported");
    }

    private static UInt256 convertToUInt256(final Bytes value) {
        // TODO: Checks and balances - this'll be a hot function
        return UInt256.valueOf(((RlpString)RlpDecoder.decode(value.toArray()).getValues().getFirst()).asPositiveBigInteger());
    }

    @Override
    public NavigableMap<Bytes32, AccountStorageEntry> storageEntriesFrom(
            final Bytes32 startKeyHash, final int limit) {
        // TODO: Needs integration properly with storage
        final NavigableMap<Bytes32, AccountStorageEntry> storageEntries = new TreeMap<>();
        this.originalStorageTrie
                .entriesFrom(startKeyHash, limit)
                .forEach(
                        (key, value) -> {
                            final AccountStorageEntry entry =
                                    AccountStorageEntry.create(
                                            convertToUInt256(value), key, storageKeys.get(key));
                            storageEntries.put(key, entry);
                        });
        return storageEntries;
    }
}
