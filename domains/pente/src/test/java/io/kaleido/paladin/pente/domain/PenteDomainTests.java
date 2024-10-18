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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertFalse;

public class PenteDomainTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup("../../core/go/db/migrations/sqlite", 5000);

    JsonHex.Address deployFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "abi"
            );
            String contractAddr = deployBed.getRpcClient().request("testbed_deployBytecode",
                    "deployer",
                    factoryABI,
                    factoryBytecode,
                    new HashMap<String,String>());
            return new JsonHex.Address(contractAddr);
        }
    }

    static final JsonABI.Entry simpleStorageDeployABI = JsonABI.newFunction(
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

    @JsonIgnoreProperties(ignoreUnknown = true)
    static final record SimpleStorageConstructorJSON(
            @JsonProperty
            String x
    ) {}

    static final JsonABI.Entry simpleStorageSetABI = JsonABI.newFunction(
            "set",
            JsonABI.newParameters(
                    JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                            JsonABI.newParameter("salt", "bytes32"),
                            JsonABI.newParameter("members", "string[]")
                    )),
                    JsonABI.newParameter("to", "address"),
                    JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                            JsonABI.newParameter("x", "uint256")
                    ))
            ),
            JsonABI.newParameters()
    );

    static final JsonABI.Entry simpleStorageLinkedDeployABI = JsonABI.newFunction(
            "deploy",
            JsonABI.newParameters(
                    JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                            JsonABI.newParameter("salt", "bytes32"),
                            JsonABI.newParameter("members", "string[]")
                    )),
                    JsonABI.newParameter("bytecode", "bytes"),
                    JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                            JsonABI.newParameter("linked", "address")
                    ))
            ),
            JsonABI.newParameters()
    );

    Testbed.PrivateContractTransaction getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.PrivateContractTransaction.class);
    }

    @Test
    void testSimpleStorage() throws Exception {
        JsonHex.Address address = deployFactory();
        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup, new Testbed.ConfigDomain(
                "pente", address, new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()), new HashMap<>()
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
                            PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
                            false
                    ));
            assertFalse(contractAddr.isBlank());

            // Deploy Simple Storage to the privacy group
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
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            simpleStorageDeployABI,
                            deployValues
                    ), true));
            var extraData = new ObjectMapper().readValue(tx.extraData(), PenteConfiguration.TransactionExtraData.class);
            var expectedContractAddress = extraData.contractAddress();

            // Invoke set on Simple Storage
            Map<String, Object> setValues = new HashMap<>() {{
                put("group", groupInfo);
                put("to", expectedContractAddress.toString());
                put("inputs", new HashMap<>() {{
                    put("x", "2233445566");
                }});
            }};
            testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            simpleStorageSetABI,
                            setValues
                    ), true);

            // Set again
            setValues = new HashMap<>() {{
                put("group", groupInfo);
                put("to", expectedContractAddress.toString());
                put("inputs", new HashMap<>() {{
                    put("x", "12345");
                }});
            }};
            testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            simpleStorageSetABI,
                            setValues
                    ), true);

        }
    }

    @Test
    void testSimpleStorageLinked() throws Exception {
        JsonHex.Address address = deployFactory();
        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup, new Testbed.ConfigDomain(
                "pente", address, new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()), new HashMap<>()
        ))) {
            PenteConfiguration.GroupTupleJSON groupInfo = new PenteConfiguration.GroupTupleJSON(
                    groupSalt,
                    new String[]{"member1", "member2"}
            );

            // Deploy SimpleStorage to the base ledger
            String ssBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/testcontracts/SimpleStorage.sol/SimpleStorage.json",
                    "bytecode"
            );
            JsonABI ssABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/testcontracts/SimpleStorage.sol/SimpleStorage.json",
                    "abi"
            );
            String ssAddr = testbed.getRpcClient().request("testbed_deployBytecode",
                    "simpleStorageDeployer",
                    ssABI,
                    ssBytecode,
                    new SimpleStorageConstructorJSON("1"));
            assertFalse(ssAddr.isBlank());

            // Create the privacy group
            String penteAddr = testbed.getRpcClient().request("testbed_deploy",
                    "pente",
                    new PenteConfiguration.PrivacyGroupConstructorParamsJSON(
                            groupInfo,
                            "shanghai",
                            PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
                            true
                    ));
            assertFalse(penteAddr.isBlank());

            // Deploy SimpleStorageLinked to the privacy group
            String ssLinkedBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/testcontracts/SimpleStorageLinked.sol/SimpleStorageLinked.json",
                    "bytecode"
            );
            var tx = getTransactionInfo(
                testbed.getRpcClient().request("testbed_invoke",
                        new PrivateContractInvoke(
                                "simpleStorageDeployer",
                                JsonHex.addressFrom(penteAddr),
                                simpleStorageLinkedDeployABI,
                                new HashMap<>() {{
                                    put("group", groupInfo);
                                    put("bytecode", ssLinkedBytecode);
                                    put("inputs", new HashMap<>() {{
                                        put("linked", ssAddr);
                                    }});
                                }}
                        ), true));
            var extraData = new ObjectMapper().readValue(tx.extraData(), PenteConfiguration.TransactionExtraData.class);
            var ssLinkedAddr = extraData.contractAddress();

            testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(penteAddr),
                            simpleStorageSetABI,
                            new HashMap<>() {{
                                put("group", groupInfo);
                                put("to", ssLinkedAddr.toString());
                                put("inputs", new HashMap<>() {{
                                    put("x", 100);
                                }});
                            }}
                    ), true);
        }
    }
}
