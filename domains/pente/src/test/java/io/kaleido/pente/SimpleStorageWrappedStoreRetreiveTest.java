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

package io.kaleido.pente;

import io.kaleido.pente.evmrunner.EVMRunner;
import io.kaleido.pente.evmrunner.EVMVersion;
import io.kaleido.pente.evmstate.PersistedAccount;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.junit.jupiter.api.Test;
import org.web3j.abi.TypeReference;
import org.web3j.abi.datatypes.Type;
import org.web3j.abi.datatypes.generated.Uint256;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.io.InputStream;
import java.util.*;

import static io.kaleido.pente.TestUtils.*;
import static org.junit.jupiter.api.Assertions.*;

public class SimpleStorageWrappedStoreRetreiveTest {

    @Test
    void runAnEVM() throws IOException {

        // Generate a shiny new EVM
        EVMVersion evmVersion = EVMVersion.Shanghai(new Random().nextLong(), EvmConfiguration.DEFAULT);
        EVMRunner evmRunnerFresh = new EVMRunner(evmVersion, address -> Optional.empty(), 0);

        // Load some bytecode for our first contract deploy
        String resourcePath = "contracts/testcontracts/SimpleStorageWrapped.sol/SimpleStorageWrapped.json";
        String hexByteCode;
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream(resourcePath)) {
            assertNotNull(is);
            JsonNode node = new ObjectMapper().readTree(is);
            hexByteCode = node.get("bytecode").asText();
        }
        final Address smartContractAddress = EVMRunner.randomAddress();
        final Address sender = EVMRunner.randomAddress();
        MessageFrame deployFrame = evmRunnerFresh.runContractDeployment(
                sender,
                smartContractAddress,
                Bytes.fromHexString(hexByteCode),
                new Uint256(12345)
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, deployFrame.getState());
        MessageFrame setFrame = evmRunnerFresh.runContractInvoke(
                sender,
                smartContractAddress,
                "set",
                new Uint256(23456)
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, setFrame.getState());

        // Persist the world we've just built into bytes
        final Map<Address, byte[]> accountBytes = new HashMap<>();
        evmRunnerFresh.getWorld().getQueriedAccounts().forEach(address -> {
            accountBytes.put(address, evmRunnerFresh.getWorld().get(address).serialize());
        });

        // Create a new world that dynamically loads those bytes
        EVMRunner evmRunnerWithLoad = new EVMRunner(evmVersion, address ->
            accountBytes.containsKey(address) ? Optional.of(PersistedAccount.deserialize(accountBytes.get(address))) : Optional.empty()
        , 0);

        // Run the get against that world
        MessageFrame getFrame = evmRunnerWithLoad.runContractInvoke(
                sender,
                smartContractAddress,
                "get"
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, getFrame.getState());
        List<Type<?>> returns = evmRunnerWithLoad.decodeReturn(getFrame, List.of(new TypeReference<Uint256>() {}));
        assertEquals(23456, ((Uint256)(returns.getFirst())).getValue().intValue());

        // We should see query against all three accounts that were in the storage
        assertEquals(
                sortedAddressList(evmRunnerWithLoad.getWorld().getQueriedAccounts()),
                sortedAddressList(accountBytes.keySet())
        );
    }
}
