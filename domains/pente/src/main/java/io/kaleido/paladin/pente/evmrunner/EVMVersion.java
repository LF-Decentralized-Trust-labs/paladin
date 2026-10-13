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

package io.kaleido.paladin.pente.evmrunner;

import org.hyperledger.besu.evm.EVM;
import org.hyperledger.besu.evm.MainnetEVMs;
import org.hyperledger.besu.evm.gascalculator.CancunGasCalculator;
import org.hyperledger.besu.evm.gascalculator.GasCalculator;
import org.hyperledger.besu.evm.gascalculator.LondonGasCalculator;
import org.hyperledger.besu.evm.gascalculator.ShanghaiGasCalculator;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.precompile.MainnetPrecompiledContracts;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;

import java.math.BigInteger;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Supplier;

public record EVMVersion(GasCalculator gasCalculator, EvmConfiguration evmConfiguration, PrecompileContractRegistry precompileContractRegistry, EVM evm) {

    private record CacheKey(String version, EvmConfiguration evmConfiguration) {}

    private record SharedContracts(GasCalculator gasCalculator, PrecompileContractRegistry precompileContractRegistry) {}

    // Gas calculators and precompile registries are immutable once built, so a single instance
    // per configuration is shared by all concurrent executions on that configuration
    private static final Map<CacheKey, SharedContracts> sharedContracts = new ConcurrentHashMap<>();

    // Nothing is locked while building, so a race between two first calls for the same
    // configuration builds twice and discards one of the two equivalent results
    private static SharedContracts shared(String version, EvmConfiguration evmConfiguration, Supplier<SharedContracts> builder) {
        var key = new CacheKey(version, evmConfiguration);
        var existing = sharedContracts.get(key);
        if (existing != null) {
            return existing;
        }
        var built = builder.get();
        var prior = sharedContracts.putIfAbsent(key, built);
        return prior != null ? prior : built;
    }

    public static EVMVersion London(long chainId, EvmConfiguration evmConfiguration) {
        var contracts = shared("london", evmConfiguration, () -> {
            var gasCalculator = new LondonGasCalculator();
            return new SharedContracts(gasCalculator, MainnetPrecompiledContracts.istanbul(gasCalculator));
        });
        var evm = MainnetEVMs.london(BigInteger.valueOf(chainId), evmConfiguration);
        return new EVMVersion(contracts.gasCalculator(), evmConfiguration, contracts.precompileContractRegistry(), evm);
    }

    public static EVMVersion Paris(long chainId, EvmConfiguration evmConfiguration) {
        var contracts = shared("paris", evmConfiguration, () -> {
            var gasCalculator = new LondonGasCalculator();
            return new SharedContracts(gasCalculator, MainnetPrecompiledContracts.istanbul(gasCalculator));
        });
        var evm = MainnetEVMs.paris(BigInteger.valueOf(chainId), evmConfiguration);
        return new EVMVersion(contracts.gasCalculator(), evmConfiguration, contracts.precompileContractRegistry(), evm);
    }

    public static EVMVersion Shanghai(long chainId, EvmConfiguration evmConfiguration) {
        var contracts = shared("shanghai", evmConfiguration, () -> {
            var gasCalculator = new ShanghaiGasCalculator();
            return new SharedContracts(gasCalculator, MainnetPrecompiledContracts.istanbul(gasCalculator));
        });
        var evm = MainnetEVMs.shanghai(BigInteger.valueOf(chainId), evmConfiguration);
        return new EVMVersion(contracts.gasCalculator(), evmConfiguration, contracts.precompileContractRegistry(), evm);
    }

    public static EVMVersion Cancun(long chainId, EvmConfiguration evmConfiguration) {
        var contracts = shared("cancun", evmConfiguration, () -> {
            var gasCalculator = new CancunGasCalculator();
            return new SharedContracts(gasCalculator, MainnetPrecompiledContracts.cancun(gasCalculator));
        });
        var evm = MainnetEVMs.cancun(BigInteger.valueOf(chainId), evmConfiguration);
        return new EVMVersion(contracts.gasCalculator(), evmConfiguration, contracts.precompileContractRegistry(), evm);
    }
}
