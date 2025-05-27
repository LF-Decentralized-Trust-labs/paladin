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

import java.io.IOException;
import java.util.*;

import static org.junit.jupiter.api.Assertions.*;

public class PenteDomainTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup(
            "../../core/go/db/migrations/sqlite",
            "build/testbed.java-pente.log",
            5000);

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
                    new HashMap<String, String>());
            return new JsonHex.Address(contractAddr);
        }
    }

    static final JsonABI simpleStorageABI = new JsonABI(Arrays.asList(
            JsonABI.newFunction(
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
            ),
            JsonABI.newFunction(
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
            )
    ));

    @JsonIgnoreProperties(ignoreUnknown = true)
    static final record SimpleStorageConstructorJSON(
            @JsonProperty
            String x
    ) {
    }

    static final JsonABI simpleStorageLinkedABI = new JsonABI(List.of(
            JsonABI.newFunction(
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
            )
    ));

    Testbed.TransactionResult getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    LinkedHashMap waitForReceipt(Testbed testbed, String txID, int waitMs) throws InterruptedException, IOException {
        final int waitIncrement = 100;
        for (int i = 0; i < waitMs; i += waitIncrement) {
            var approveReceipt = testbed.getRpcClient().request("ptx_getTransactionReceipt", txID);
            if (approveReceipt != null) {
                return new ObjectMapper().convertValue(approveReceipt, LinkedHashMap.class);
            }
            Thread.sleep(waitIncrement);
        }
        fail("Receipt not found");
        return null;
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
                    "pente", "member1",
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
            var mapper = new ObjectMapper();
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "simpleStorageDeployer",
                                    JsonHex.addressFrom(contractAddr),
                                    deployValues,
                                    simpleStorageABI,
                                    "deploy"
                            ), true));
            var domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var expectedContractAddress = domainReceipt.receipt().contractAddress();

            // Invoke set on Simple Storage
            Map<String, Object> setValues = new HashMap<>() {{
                put("group", groupInfo);
                put("to", expectedContractAddress.toString());
                put("inputs", new HashMap<>() {{
                    put("x", "2233445566");
                }});
            }};
            testbed.getRpcClient().request("testbed_invoke",
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            setValues,
                            simpleStorageABI,
                            "set"
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
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(contractAddr),
                            setValues,
                            simpleStorageABI,
                            "set"
                    ), true);

        }
    }

    @Test
    void testSimpleStorageApproval() throws Exception {
        JsonHex.Address address = deployFactory();
        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup, new Testbed.ConfigDomain(
                "pente", address, new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()), new HashMap<>()
        ))) {
            var mapper = new ObjectMapper();
            PenteConfiguration.GroupTupleJSON groupInfo = new PenteConfiguration.GroupTupleJSON(
                    groupSalt,
                    new String[]{"member1", "member2"}
            );

            // Create the privacy group
            String contractAddr = testbed.getRpcClient().request("testbed_deploy",
                    "pente", "member1",
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
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "simpleStorageDeployer",
                                    JsonHex.addressFrom(contractAddr),
                                    deployValues,
                                    simpleStorageABI,
                                    "deploy"
                            ), true));
            var domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var expectedContractAddress = domainReceipt.receipt().contractAddress();

            // Prepare a "set" on Simple Storage
            Map<String, Object> setValues = new HashMap<>() {{
                put("group", groupInfo);
                put("to", expectedContractAddress.toString());
                put("inputs", new HashMap<>() {{
                    put("x", "2233445566");
                }});
            }};
            var preparedSet = mapper.convertValue(
                    testbed.getRpcClient().request("testbed_prepare",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "simpleStorageDeployer",
                                    JsonHex.addressFrom(contractAddr),
                                    setValues,
                                    simpleStorageABI,
                                    "set")),
                    Testbed.TransactionResult.class);
            var metadata = mapper.convertValue(preparedSet.preparedMetadata(), PenteConfiguration.PenteTransitionMetadata.class);
            var transitionParams = mapper.convertValue(preparedSet.preparedTransaction().data(), PenteConfiguration.PenteTransitionParams.class);

            String member3Address = testbed.getRpcClient().request("testbed_resolveVerifier",
                    "member3", Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);

            // Approve the "set"
            var config = new PenteConfiguration();
            String approveTx = testbed.getRpcClient().request("ptx_sendTransaction",
                    new LinkedHashMap<String, Object>() {{
                        put("type", "public");
                        put("from", "member1");
                        put("to", contractAddr);
                        put("abi", config.getPrivacyGroupABI());
                        put("function", "approveTransition");
                        put("data", new LinkedHashMap<String, Object>() {{
                            put("txId", new JsonHex.Bytes32("0x0000000000000000000000000000000000000000000000000000000000000000"));
                            put("delegate", member3Address);
                            put("transitionHash", metadata.approvalParams().transitionHash());
                            put("signatures", metadata.approvalParams().signatures());
                        }});
                    }});
            var approveReceipt = waitForReceipt(testbed, approveTx, 5000);
            assertEquals(true, approveReceipt.get("success"));

            // Utilize the approval
            String setTx = testbed.getRpcClient().request("ptx_sendTransaction",
                    new LinkedHashMap<String, Object>() {{
                        put("type", "public");
                        put("from", "member3");
                        put("to", contractAddr);
                        put("abi", config.getPrivacyGroupABI());
                        put("function", "transitionWithApproval");
                        put("data", new LinkedHashMap<String, Object>() {{
                            put("txId", new JsonHex.Bytes32("0x0000000000000000000000000000000000000000000000000000000000000000"));
                            put("states", transitionParams.states());
                            put("externalCalls", transitionParams.externalCalls());
                        }});
                    }});
            var setReceipt = waitForReceipt(testbed, setTx, 5000);
            assertEquals(true, setReceipt.get("success"));
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
                    "pente", "member1",
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
            var mapper = new ObjectMapper();
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "simpleStorageDeployer",
                                    JsonHex.addressFrom(penteAddr),
                                    new HashMap<>() {{
                                        put("group", groupInfo);
                                        put("bytecode", ssLinkedBytecode);
                                        put("inputs", new HashMap<>() {{
                                            put("linked", ssAddr);
                                        }});
                                    }},
                                    simpleStorageLinkedABI,
                                    "deploy"
                            ), true));
            var domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var ssLinkedAddr = domainReceipt.receipt().contractAddress();

            testbed.getRpcClient().request("testbed_invoke",
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "simpleStorageDeployer",
                            JsonHex.addressFrom(penteAddr),
                            new HashMap<>() {{
                                put("group", groupInfo);
                                put("to", ssLinkedAddr.toString());
                                put("inputs", new HashMap<>() {{
                                    put("x", 100);
                                }});
                            }},
                            simpleStorageABI,
                            "set"
                    ), true);
        }
    }
}
