/*
 * Copyright contributors to Paladin, an LFDT project
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 */

package io.kaleido.paladin.pente;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmrunner.EVMVersion;
import io.kaleido.paladin.toolkit.JsonHex;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.evm.EVM;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.gascalculator.GasCalculator;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;
import org.junit.jupiter.api.Test;
import org.web3j.abi.TypeReference;
import org.web3j.abi.datatypes.Bool;
import org.web3j.abi.datatypes.DynamicBytes;
import org.web3j.abi.datatypes.Type;
import org.web3j.abi.datatypes.generated.Bytes32;
import org.web3j.abi.datatypes.generated.Uint256;

import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.LinkedList;
import java.util.List;
import java.util.Optional;
import java.util.Random;
import java.util.Set;
import java.util.concurrent.Callable;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertSame;

public class EVMVersionConcurrencyTest {

    private static final int THREADS = 8;

    private static final int EXECUTIONS_PER_THREAD = 4;

    /**
     * Each test uses a configuration of its own so that the memoized entry it exercises is
     * cold whatever order the tests run in, and is not shared with any other test.
     */
    private static EvmConfiguration distinctConfiguration(long jumpDestCacheWeightKB) {
        return new EvmConfiguration(jumpDestCacheWeightKB, EvmConfiguration.WorldUpdaterMode.STACKED);
    }

    /**
     * Runs the task on every thread at once, releasing them from a barrier so that the calls
     * overlap, and returns each thread's result once they have all completed.
     */
    private static <T> List<T> onAllThreadsAtOnce(Callable<T> task) throws Exception {
        ExecutorService pool = Executors.newFixedThreadPool(THREADS);
        try {
            var startTogether = new CyclicBarrier(THREADS);
            var pending = new ArrayList<Future<T>>();
            for (int i = 0; i < THREADS; i++) {
                pending.add(pool.submit(() -> {
                    startTogether.await(30, TimeUnit.SECONDS);
                    return task.call();
                }));
            }
            var results = new ArrayList<T>();
            for (var future : pending) {
                results.add(future.get(180, TimeUnit.SECONDS));
            }
            return results;
        } finally {
            pool.shutdownNow();
        }
    }

    private static <T> Set<T> byIdentity(List<T> values) {
        Set<T> distinct = Collections.newSetFromMap(new IdentityHashMap<T, Boolean>());
        distinct.addAll(values);
        return distinct;
    }

    private static Bytes bytecodeOf(String resourcePath) throws IOException {
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream(resourcePath)) {
            assertNotNull(is);
            JsonNode node = new ObjectMapper().readTree(is);
            return Bytes.fromHexString(node.get("bytecode").asText());
        }
    }

    @Test
    void concurrentFirstCallsAllReceiveTheSameSharedContracts() throws Exception {
        var configuration = distinctConfiguration(4101);

        // The chainId differs per call, so a shared result also shows chainId is not part of the key
        var versions = onAllThreadsAtOnce(() -> EVMVersion.Shanghai(new Random().nextLong(), configuration));

        assertEquals(THREADS, versions.size());
        assertEquals(1, byIdentity(versions.stream().map(EVMVersion::precompileContractRegistry).toList()).size());
        assertEquals(1, byIdentity(versions.stream().map(EVMVersion::gasCalculator).toList()).size());
    }

    @Test
    void everyCallBuildsItsOwnEvmSoTheCodeCacheIsNeverShared() {
        var configuration = distinctConfiguration(4102);

        var first = EVMVersion.Shanghai(1L, configuration);
        var second = EVMVersion.Shanghai(1L, configuration);

        assertNotSame(first.evm(), second.evm());
        assertSame(first.precompileContractRegistry(), second.precompileContractRegistry());
        assertSame(first.gasCalculator(), second.gasCalculator());
    }

    @Test
    void concurrentCallsEachBuildTheirOwnEvm() throws Exception {
        var configuration = distinctConfiguration(4103);

        var versions = onAllThreadsAtOnce(() -> EVMVersion.Shanghai(1L, configuration));

        Set<EVM> evms = byIdentity(versions.stream().map(EVMVersion::evm).toList());
        assertEquals(THREADS, evms.size());
    }

    @Test
    void concurrentExecutionsThroughTheSharedPrecompileRegistryRecoverCorrectly() throws Exception {
        var configuration = distinctConfiguration(4104);
        var bytecode = bytecodeOf("contracts/testcontracts/Recover.sol/Recover.json");
        var signedMessage = JsonHex.from("0xacaf3289d7b601cbd114fb36c4d29c85bbfd5e133f14cb355c3fd8d99367964f");
        var signature = JsonHex.from("0xe76d4a6f194440ca1b19695e41538b960afc9c27c69722ef93cbf0134cbc6fd317481bff0ca56883b81fd37dcf009d7b9c98c793a67826602b8d8eb83a8b94c51b");
        var expectedAddress = "0x78826125b6be403ea159876f5a32a3eac7cd0fe5";

        var registries = onAllThreadsAtOnce(() -> {
            PrecompileContractRegistry registry = null;
            for (int i = 0; i < EXECUTIONS_PER_THREAD; i++) {
                var evmVersion = EVMVersion.Shanghai(new Random().nextLong(), configuration);
                registry = evmVersion.precompileContractRegistry();
                var evmRunner = new EVMRunner(evmVersion, address -> Optional.empty(), 0, 0);
                var contractAddress = EVMRunner.randomAddress();
                var sender = EVMRunner.randomAddress();
                var logs = new LinkedList<EVMRunner.JsonEVMLog>();

                var deployFrame = evmRunner.runContractDeployment(
                        sender, contractAddress, bytecode, Long.MAX_VALUE, logs);
                assertEquals(MessageFrame.State.COMPLETED_SUCCESS, deployFrame.getState());

                var verifyFrame = evmRunner.runContractInvoke(
                        sender,
                        contractAddress,
                        "verifySignature",
                        Long.MAX_VALUE,
                        logs,
                        new Bytes32(signedMessage.getBytes()),
                        new DynamicBytes(signature.getBytes()),
                        new org.web3j.abi.datatypes.Address(expectedAddress)
                );
                assertEquals(MessageFrame.State.COMPLETED_SUCCESS, verifyFrame.getState());

                List<Type<?>> returns = evmRunner.decodeReturn(verifyFrame, List.of(
                        new TypeReference<Bool>() {},
                        new TypeReference<org.web3j.abi.datatypes.Address>() {}
                ));
                assertEquals("true", returns.get(0).getValue().toString());
                assertEquals(expectedAddress, returns.get(1).getValue().toString());
            }
            return registry;
        });

        assertEquals(1, byIdentity(registries).size());
    }

    @Test
    void concurrentExecutionsAcrossContractToContractCallsReadBackTheirOwnValue() throws Exception {
        var configuration = distinctConfiguration(4105);
        var bytecode = bytecodeOf("contracts/testcontracts/SimpleStorageWrapped.sol/SimpleStorageWrapped.json");

        var registries = onAllThreadsAtOnce(() -> {
            PrecompileContractRegistry registry = null;
            for (int i = 0; i < EXECUTIONS_PER_THREAD; i++) {
                var stored = new Random().nextInt(1, Integer.MAX_VALUE);
                var evmVersion = EVMVersion.Shanghai(new Random().nextLong(), configuration);
                registry = evmVersion.precompileContractRegistry();
                var evmRunner = new EVMRunner(evmVersion, address -> Optional.empty(), 0, 0);
                var contractAddress = EVMRunner.randomAddress();
                var sender = EVMRunner.randomAddress();
                var logs = new LinkedList<EVMRunner.JsonEVMLog>();

                var deployFrame = evmRunner.runContractDeployment(
                        sender, contractAddress, bytecode, Long.MAX_VALUE, logs, new Uint256(12345));
                assertEquals(MessageFrame.State.COMPLETED_SUCCESS, deployFrame.getState());

                var setFrame = evmRunner.runContractInvoke(
                        sender, contractAddress, "set", Long.MAX_VALUE, logs, new Uint256(stored));
                assertEquals(MessageFrame.State.COMPLETED_SUCCESS, setFrame.getState());

                var getFrame = evmRunner.runContractInvoke(
                        sender, contractAddress, "get", Long.MAX_VALUE, logs);
                assertEquals(MessageFrame.State.COMPLETED_SUCCESS, getFrame.getState());

                List<Type<?>> returns = evmRunner.decodeReturn(getFrame, List.of(new TypeReference<Uint256>() {}));
                assertEquals(stored, ((Uint256) (returns.getFirst())).getValue().intValue());
            }
            return registry;
        });

        assertEquals(1, byIdentity(registries).size());
    }
}
