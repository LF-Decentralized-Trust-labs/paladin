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

package io.kaleido.pente.domain;

import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonRpcClient;
import io.kaleido.paladin.toolkit.ResourceLoader;
import io.kaleido.paladin.toolkit.Testbed;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

public class PenteDomainTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup("../../kata/db/migrations/sqlite", 5000);

    String deployFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            deployBed.start();
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/pente/PenteFactory.sol/PenteFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/pente/PenteFactory.sol/PenteFactory.json",
                    "abi"
            );
            try (JsonRpcClient testbedClient = new JsonRpcClient(deployBed.getRPCUrl())) {
                return testbedClient.request("testbed_deployBytecode",
                        "deployer",
                        factoryABI,
                        factoryBytecode,
                        "{}");
            }
        }
    }

    @Test
    void testSimpleStorage() throws Exception {
        String address = deployFactory();
        Assertions.assertNotEquals("", address);
    }
}
