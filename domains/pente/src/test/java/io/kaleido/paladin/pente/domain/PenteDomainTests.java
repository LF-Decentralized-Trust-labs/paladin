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

package io.kaleido.paladin.pente.domain;

import io.kaleido.paladin.pente.domain.PenteConfiguration;
import io.kaleido.paladin.pente.domain.PenteDomainFactory;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertFalse;

public class PenteDomainTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup("../../core/go/db/migrations/sqlite", 5000);

    String deployFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/domains/ente/PenteFactory.sol/PenteFactory.json",
                    "abi"
            );
            return deployBed.getRpcClient().request("testbed_deployBytecode",
                    "deployer",
                    factoryABI,
                    factoryBytecode,
                    "{}");
        }
    }

    @Test
    void testSimpleStorage() throws Exception {
        String address = deployFactory();
        Assertions.assertNotEquals("", address);
        Map<String, Object> config = new HashMap<>();
        config.put("address", address);

        JsonABI.Entry simpleStorageDeployABI = JsonABI.newFunction(
                "deploy",
                JsonABI.newParameters(
                        JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                JsonABI.newParameter("salt", "bytes32"),
                                JsonABI.newParameter("members", "string[]")
                        )),
                        JsonABI.newParameter("bytecode", "bytes"),
                        JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                JsonABI.newParameter("x", "uint256")
                        ))
                ),
                JsonABI.newParameters()
        );

        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup, new Testbed.ConfigDomain(
                "pente", new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()), config
        ))) {
            PenteConfiguration.GroupTupleJSON groupInfo = new PenteConfiguration.GroupTupleJSON(
                    groupSalt,
                    new String[]{"member1", "member2"}
            );

            // Create the privacy group
            String contractAddr = testbed.getRpcClient().request("testbed_deploy",
                    "pente",
                    new PenteConfiguration.PrivacyGroupConstructorParamsJSON(
                            groupInfo,
                            "shanghai",
                            PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES
                    ));
            assertFalse(contractAddr.isBlank());

            // Deploy Simple Storage to it
            String simpleStorageBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/testcontracts/SimpleStorage.sol/SimpleStorage.json",
                    "bytecode"
            );
            Map<String, Object> deployValues = new HashMap<>() {{
                put("group", groupInfo);
                put("bytecode", simpleStorageBytecode);
                put("inputs", new HashMap<>() {{
                    put("x", "1122334455");
                }});
            }};
            testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            simpleStorageDeployABI,
                            deployValues
                    ));
            assertFalse(contractAddr.isBlank());
        }
    }
}
