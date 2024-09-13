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

package io.kaleido.paladin.pente;

import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.account.Account;
import org.hyperledger.besu.evm.worldstate.UpdateTrackingAccount;
import org.junit.jupiter.api.Test;

import java.io.IOException;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

public class PersistedAccountTest {

    @Test
    void serializeDeserializeAccounts() throws IOException {

        Address address = EVMRunner.randomAddress();

        // Build an interesting looking account
        PersistedAccount account = new PersistedAccount(address);
        UpdateTrackingAccount<Account> updater = new UpdateTrackingAccount<>(account);
        updater.setNonce(12345L);
        updater.setCode(Bytes.fromHexString("0xFeeDBeeF"));
        for (int i = 0; i < 1000; i++) {
            updater.setStorageValue(TestUtils.randomUInt256(), TestUtils.randomUInt256());
        }
        account.applyChanges(updater);

        // Serialize it
        String serialized = Bytes.wrap(account.serialize()).toHexString();

        // Deserialize it
        PersistedAccount deserialized = PersistedAccount.deserialize(Bytes.fromHexString(serialized).toArray());

        // Check the values
        updater.getUpdatedStorage().forEach((k, v) -> {
            assertEquals(updater.getUpdatedStorage().get(k), deserialized.getStorageValue(k));
        });

    }

}
