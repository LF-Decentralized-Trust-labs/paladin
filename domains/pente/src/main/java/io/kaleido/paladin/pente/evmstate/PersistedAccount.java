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

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.toolkit.JsonHex;
import kotlin.NotImplementedError;
import kotlin.collections.ArrayDeque;
import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.Bytes32;
import org.apache.tuweni.units.bigints.UInt256;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.ethereum.trie.MerkleTrie;
import org.hyperledger.besu.ethereum.trie.TrieIterator;
import org.hyperledger.besu.ethereum.trie.patricia.SimpleMerklePatriciaTrie;
import org.hyperledger.besu.evm.account.Account;
import org.hyperledger.besu.evm.account.AccountStorageEntry;
import org.hyperledger.besu.evm.worldstate.UpdateTrackingAccount;
import org.web3j.rlp.*;

import java.math.BigInteger;
import java.util.*;

public class PersistedAccount implements Account {

    private final Address address;

    private long nonce;

    private Wei balance = Wei.ZERO;

    private Bytes code;

    private final MerkleTrie<Bytes32, Bytes> storageTrie =  new SimpleMerklePatriciaTrie<>(b -> b);;

    private final HashMap<Bytes32, UInt256> storageKeys = new HashMap<>();

    public PersistedAccount(Address address) {
        this.address = address;
    }


    @Override
    public Address getAddress() {
        return this.address;
    }

    @Override
    public boolean isStorageEmpty() {
        return this.storageTrie.getRootHash().equals(MerkleTrie.EMPTY_TRIE_NODE);
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
        return balance;
    }

    @Override
    public Bytes getCode() {
        return this.code;
    }

    @Override
    public Hash getCodeHash() {
        if (this.code == null) {
            return null;
        }
        return Hash.hash(this.code);
    }

    public Hash getCodeHashOrZero() {
        if (this.code == null) {
            return Hash.ZERO;
        }
        return Hash.hash(this.code);
    }

    @Override
    public UInt256 getStorageValue(UInt256 k) {
        Optional<Bytes> v = this.storageTrie.get(Hash.hash(k));
        return v.map(PersistedAccount::convertBytes32ToUInt256).orElse(null);
    }

    @Override
    public UInt256 getOriginalStorageValue(UInt256 k) {
        // This account is not used to store diffs - so there's no difference in the original
        return this.getStorageValue(k);
    }

    private static UInt256 convertBytes32ToUInt256(final Bytes value) {
        return UInt256.fromBytes(value);
    }

    private static Bytes convertToBytes32(final UInt256 value) {
        return Bytes.wrap(value.toBytes());
    }

    @Override
    public NavigableMap<Bytes32, AccountStorageEntry> storageEntriesFrom(
            final Bytes32 startKeyHash, final int limit) {
        // TODO: This function would require a reverse lookup table from storage key pre-images
        //       to the hashed 32b storage keys.
        throw new NotImplementedError("storage traversal not implemented");
    }

    @Override
    public boolean equals(Object otherObj) {
        if (!(otherObj instanceof PersistedAccount)) {
            return false;
        }
        var other = (PersistedAccount)(otherObj);
        return this.getAddress().equals(other.getAddress()) &&
                this.getNonce() == other.getNonce() &&
                this.getBalance().equals(other.getBalance()) &&
                this.getCodeHashOrZero().equals(other.getCodeHashOrZero()) &&
                this.storageTrie.getRootHash().equals(other.storageTrie.getRootHash());
    }

    public void applyChanges(UpdateTrackingAccount<? extends Account> account)  {
        this.nonce = account.getNonce();
        this.balance = account.getBalance();
        this.code = account.getCode();
        account.getUpdatedStorage()
                .forEach(
                    (key, value) -> {
                        if (value.isZero()) {
                            this.storageTrie.remove(Hash.hash(key));
                        } else {
                            this.storageTrie.put(Hash.hash(key), convertToBytes32(value));
                        }
                    });
    }

    public String toString() {
        return this.getAddress().toString() +
                '[' +
                "nonce=" +
                this.getNonce() +
                "balance=" +
                this.getBalance() +
                "codehash=" +
                this.getCodeHashOrZero().toString() +
                "storagehash=" +
                this.storageTrie.getRootHash().toString() +
                ']';
    }

    public record PersistedAccountJson(
            @JsonProperty
            String version,
            @JsonProperty
            JsonHex.Address address,
            @JsonProperty
            long nonce,
            @JsonProperty
            BigInteger balance,
            @JsonProperty
            JsonHex.Bytes32 codeHash,
            @JsonProperty
            JsonHex.Bytes code,
            @JsonProperty
            JsonHex.Bytes32 storageRoot,
            @JsonProperty
            List<JsonHex.Bytes32[]> storage
    ) {}

    public byte[] serialize() {
        try {
            JsonHex.Bytes32 jsonCodeHash = JsonHex.Bytes32.ZERO;
            JsonHex.Bytes jsonCode = new JsonHex.Bytes(new byte[0]);
            var codehash = this.getCodeHash();
            if (codehash != null) {
                jsonCodeHash = new JsonHex.Bytes32(this.getCodeHash().toArray());
                jsonCode = new JsonHex.Bytes(this.code.toArray());
            }
            var storageTrieLeafs = new ArrayDeque<JsonHex.Bytes32[]>();
            var jsonAccount = new PersistedAccountJson(
                    "v24.9.0",
                    new JsonHex.Address(this.address.toArray()),
                    this.nonce,
                    this.balance.getAsBigInteger(),
                    jsonCodeHash,
                    jsonCode,
                    new JsonHex.Bytes32(this.storageTrie.getRootHash().toArray()),
                    storageTrieLeafs
            );
            this.storageTrie.visitLeafs((key, leafNode) -> {
                if (leafNode.getValue().isPresent()) {
                    storageTrieLeafs.add(new JsonHex.Bytes32[]{
                            new JsonHex.Bytes32(key.toArray()),
                            new JsonHex.Bytes32(leafNode.getValue().get().toArray())
                    });
                }
                return TrieIterator.State.CONTINUE;
            });
            return new ObjectMapper().writeValueAsBytes(jsonAccount);
        } catch(Exception e) {
            throw new IllegalArgumentException(e);
        }
    }

    public static PersistedAccount deserialize(byte[] data)  {
        try {
            var jsonAccount = new ObjectMapper().readValue(data, PersistedAccountJson.class);
            if (!jsonAccount.version().equals("v24.9.0")) {
                throw new IllegalArgumentException("unsupported version: %s".formatted(jsonAccount.version()));
            }
            PersistedAccount account = new PersistedAccount(Address.wrap(Bytes.wrap(jsonAccount.address().getBytes())));
            account.nonce = jsonAccount.nonce;
            account.balance = Wei.of(jsonAccount.balance);
            if (!jsonAccount.codeHash.equals(JsonHex.Bytes32.ZERO)) {
                account.code = Bytes.wrap(jsonAccount.code.getBytes());
                if (!account.getCodeHash().equals(Bytes.wrap(jsonAccount.codeHash.getBytes()))) {
                    throw new IllegalArgumentException("code bytes hash mismatch");
                }
            }
            Bytes storageRootHash = Bytes.wrap(jsonAccount.storageRoot().getBytes());
            for (var leafKV : jsonAccount.storage) {
                account.storageTrie.put(Bytes32.wrap(leafKV[0].getBytes()), Bytes.wrap(leafKV[1].getBytes()));
            }
            if (!account.storageTrie.getRootHash().equals(Bytes.wrap(storageRootHash))) {
                throw new IllegalArgumentException("storage trie root hash mismatch");
            }
            return account;
        } catch (Exception e) {
            throw new IllegalArgumentException(e);
        }
    }
}
