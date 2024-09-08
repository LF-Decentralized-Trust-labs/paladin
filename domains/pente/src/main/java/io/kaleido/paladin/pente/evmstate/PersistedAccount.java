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

import java.util.*;

public class PersistedAccount implements Account {

    private final Address address;

    private long nonce;

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
        return Wei.ZERO;
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

    @Override
    public UInt256 getStorageValue(UInt256 k) {
        Optional<Bytes> v = this.storageTrie.get(Hash.hash(k));
        return v.map(PersistedAccount::convertRLPBytesToUInt256).orElse(null);
    }

    @Override
    public UInt256 getOriginalStorageValue(UInt256 k) {
        // This account is not used to store diffs - so there's no difference in the original
        return this.getStorageValue(k);
    }

    private static UInt256 convertRLPBytesToUInt256(final Bytes value) {
        // TODO: Optimize function for simple scalar UInt256 conversion?
        return UInt256.valueOf(((RlpString)RlpDecoder.decode(value.toArray()).getValues().getFirst()).asPositiveBigInteger());
    }

    private static Bytes convertToRLPBytes(final UInt256 value) {
        // TODO: Optimize function for simple scalar UInt256 conversion?
        return Bytes.wrap(RlpEncoder.encode(RlpString.create(value.toMinimalBytes().toArray())));
    }

    @Override
    public NavigableMap<Bytes32, AccountStorageEntry> storageEntriesFrom(
            final Bytes32 startKeyHash, final int limit) {
        // TODO: This function would require a reverse lookup table from storage key pre-images
        //       to the hashed 32b storage keys.
        throw new NotImplementedError("storage traversal not implemented");
//        final NavigableMap<Bytes32, AccountStorageEntry> storageEntries = new TreeMap<>();
//        this.storageTrie
//                .entriesFrom(startKeyHash, limit)
//                .forEach(
//                        (key, value) -> {
//                            final AccountStorageEntry entry =
//                                    AccountStorageEntry.create(
//                                            convertToUInt256(value), key, storageKeys.get(key));
//                            storageEntries.put(key, entry);
//                        });
//        return storageEntries;
    }

    public void applyChanges(UpdateTrackingAccount<? extends Account> account)  {
        if (!account.getBalance().isZero()) {
            throw new UnsupportedOperationException("balance cannot be modified in private contracts");
        }
        this.nonce = account.getNonce();
        this.code = account.getCode();

        account.getUpdatedStorage()
                .forEach(
                    (key, value) -> {
                        if (value.isZero()) {
                            this.storageTrie.remove(Hash.hash(key));
                        } else {
                            this.storageTrie.put(Hash.hash(key), convertToRLPBytes(value));
                        }
                    });
    }

    public byte[] serialize() {
        // We use RLP for serialization. This isn't strictly necessary, but it is more efficient than
        // JSON, and we're in a world of RLP serialization.
        // TODO: Performance/readability/versioning comparison with Protobuf?
        List<RlpType> values = new ArrayDeque<>();
        values.add(RlpString.create(1L /* version */));
        values.add(RlpString.create(this.address.toArray()));
        values.add(RlpString.create(this.nonce));
        Bytes codeHash = this.getCodeHash();
        if (codeHash == null) {
            values.add(RlpString.create((byte[])(null)));
            values.add(RlpString.create((byte[])(null)));
        } else {
            values.add(RlpString.create(codeHash.toArray()));
            values.add(RlpString.create(this.code.toArray()));
        }
        values.add(RlpString.create(this.storageTrie.getRootHash().toArray()));
        List<RlpType> trieLeafs = new ArrayDeque<>();
        this.storageTrie.visitLeafs((key, leafNode) -> {
            if (leafNode.getValue().isPresent()) {
                trieLeafs.add(new RlpList(
                        RlpString.create(key.toArray()),
                        RlpString.create(leafNode.getValue().get().toArray())
                ));
            }
            return TrieIterator.State.CONTINUE;
        });
        values.add(new RlpList(trieLeafs));
        return RlpEncoder.encode(new RlpList(values));
    }

    private static class RlpIterator {
        private final Iterator<RlpType> iterator;
        private RlpIterator(RlpList list) {
            this.iterator = list.getValues().iterator();
        }
        private boolean hasNext() {
            return this.iterator.hasNext();
        }
        private RlpString nextString() {
            return (RlpString)(this.iterator.next());
        }
        private RlpList nextList() {
            return (RlpList)(this.iterator.next());
        }
    }

    public static PersistedAccount deserialize(byte[] data)  {
        RlpIterator rlp = new RlpIterator(new RlpIterator(RlpDecoder.decode(data)).nextList());
        if (rlp.nextString().asPositiveBigInteger().longValue() != 1L) {
            throw new IllegalArgumentException("only version 1 encoding is supported");
        }
        PersistedAccount account = new PersistedAccount(Address.wrap(Bytes.wrap(rlp.nextString().getBytes())));
        account.nonce = rlp.nextString().asPositiveBigInteger().longValue();
        byte[] codeHash = rlp.nextString().getBytes();
        if (codeHash == null) {
            rlp.nextString();
        } else {
            account.code = Bytes.wrap(rlp.nextString().getBytes());
            if (!account.getCodeHash().equals(Bytes.wrap(codeHash))) {
                throw new IllegalArgumentException("code bytes hash mismatch");
            }
        }
        Bytes storageRootHash = Bytes.wrap(rlp.nextString().getBytes());
        RlpIterator trieLeafs = new RlpIterator(rlp.nextList());
        while (trieLeafs.hasNext()) {
            RlpIterator leafKV = new RlpIterator(trieLeafs.nextList());
            account.storageTrie.put(
                    Bytes32.wrap(leafKV.nextString().getBytes()),
                    Bytes.wrap(leafKV.nextString().getBytes()));
        }
        if (!account.storageTrie.getRootHash().equals(Bytes.wrap(storageRootHash))) {
            throw new IllegalArgumentException("storage trie root hash mismatch");
        }
        return account;
    }
}
