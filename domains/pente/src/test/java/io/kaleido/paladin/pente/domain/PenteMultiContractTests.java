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

import java.io.IOException;
import java.util.Arrays;
import java.util.HashMap;
import java.util.LinkedHashMap;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.fail;
import org.junit.jupiter.api.Test;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.Algorithms;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.ResourceLoader;
import io.kaleido.paladin.toolkit.Verifiers;

public class PenteMultiContractTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup(
            "../../core/go/db/migrations/sqlite",
            "build/testbed.java-pente-multi-contract.log",
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

    static final JsonABI tickTockABI = new JsonABI(Arrays.asList(
            JsonABI.newFunction(
                    "deploy",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("bytecode", "bytes"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                    JsonABI.newParameter("policy", "address")
                            ))
                    ),
                    JsonABI.newParameters()
            ),
            JsonABI.newFunction(
                    "onMint",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("to", "address"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                    JsonABI.newParameter("to", "address"),
                                    JsonABI.newParameter("amount", "uint256"),
                                    JsonABI.newParameter("data", "bytes")
                            ))
                    ),
                    JsonABI.newParameters()
            ),
            JsonABI.newFunction(
                    "tickTock",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("to", "address"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                            ))
                    ),
                    JsonABI.newParameters()
            )
    ));

    static final JsonABI helperABI = new JsonABI(Arrays.asList(
            JsonABI.newFunction(
                    "deploy",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("bytecode", "bytes"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                    JsonABI.newParameter("owner", "address")
                            ))
                    ),
                    JsonABI.newParameters()
            )
    ));

    Testbed.TransactionResult getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    @Test
    void testContractsCallEachOther() throws Exception {
        JsonHex.Address address = deployFactory();
        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup, new Testbed.ConfigDomain(
                "pente", address, new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()), new HashMap<>()
        ))) {
            PenteConfiguration.GroupTupleJSON groupInfo = new PenteConfiguration.GroupTupleJSON(
                    groupSalt,
                    new String[]{"notary", "participant1", "participant2"}
            );

            // Create the privacy group
            String penteAddr = testbed.getRpcClient().request("testbed_deploy",
                    "pente", "notary",
                    new PenteConfiguration.PrivacyGroupConstructorParamsJSON(
                            groupInfo,
                            "shanghai",
                            PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
                            false
                    ));
            assertFalse(penteAddr.isBlank());

            // Deploy helper contract to the privacy group
            String policyCheckerBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/private/TickTockHelper.sol/TickTockHelper.json",
                    "bytecode"
            );

            String notaryAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    "notary", Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);

            var mapper = new ObjectMapper();
            var policyTx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "notary",
                                    JsonHex.addressFrom(penteAddr),
                                    new HashMap<>() {{
                                        put("group", groupInfo);
                                        put("bytecode", policyCheckerBytecode);
                                        put("inputs", new HashMap<>() {{
                                            put("owner", notaryAddress);
                                        }});
                                    }},
                                    helperABI,
                                    "deploy"
                            ), true));
            var policyDomainReceipt = mapper.convertValue(policyTx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var policyCheckerAddr = policyDomainReceipt.receipt().contractAddress();

            // Deploy TickTock to the privacy group
            String tickTockBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/private/TickTock.sol/TickTock.json",
                    "bytecode"
            );
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "notary",
                                    JsonHex.addressFrom(penteAddr),
                                    new HashMap<>() {{
                                        put("group", groupInfo);
                                        put("bytecode", tickTockBytecode);
                                        put("inputs", new HashMap<>() {{
                                            put("policy", policyCheckerAddr);
                                        }});
                                    }},
                                    tickTockABI,
                                    "deploy"
                            ), true));
            var domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var mainContractAddress = domainReceipt.receipt().contractAddress();

            tx = getTransactionInfo(testbed.getRpcClient().request("testbed_invoke",
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "participant1",
                            JsonHex.addressFrom(penteAddr),
                            new HashMap<>() {{
                                put("group", groupInfo);
                                put("to", mainContractAddress.toString());
                                put("inputs", new HashMap<>());
                            }},
                            tickTockABI,
                            "tickTock"
                    ), true));
            domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);

            assertEquals(2, domainReceipt.receipt().logs().size());

            // Tick event topic 0: 0x69e9c124815de375694fc163013eda46d15d79fe197fc42da80f1cf29f2a24a4
            // Tock event topic 0: 0xf8913ab6e531ecb8dde9472b4c1b3f054c2aa26bde054b2e5e19140bebebc1af

            // Log 0: Tick event
            assertEquals("0x69e9c124815de375694fc163013eda46d15d79fe197fc42da80f1cf29f2a24a4",
                    domainReceipt.receipt().logs().get(0).topics().get(0).toString());
            assertEquals(mainContractAddress,
                    domainReceipt.receipt().logs().get(0).address());


            // Log 1: Tock event
            assertEquals("0xf8913ab6e531ecb8dde9472b4c1b3f054c2aa26bde054b2e5e19140bebebc1af",
                    domainReceipt.receipt().logs().get(1).topics().get(0).toString());
            assertEquals(mainContractAddress,
                    domainReceipt.receipt().logs().get(1).address());
        }
    }
}
