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
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.pente.domain.PenteConfiguration.GroupTupleJSON;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

public class DomainIntegrationTests {

    private final Testbed.Setup testbedSetup = new Testbed.Setup(
            "../../core/go/db/migrations/sqlite",
            "build/testbed.java-domain-integration.log",
            5000);

    JsonHex.Address deployPenteFactory() throws Exception {
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

    JsonHex.Address deployNotoFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/domains/noto/NotoFactory.sol/NotoFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/domains/noto/NotoFactory.sol/NotoFactory.json",
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

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoConstructorParamsJSON(
            @JsonProperty
            String notary,
            @JsonProperty
            String notaryMode,
            @JsonProperty
            NotoOptionsJSON options
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoOptionsJSON(
            @JsonProperty
            NotoHookParamsJSON hooks
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoHookParamsJSON(
            @JsonProperty
            String publicAddress,
            @JsonProperty
            JsonHex.Address privateAddress,
            @JsonProperty
            GroupTupleJSON privateGroup
    ) {
    }

    static final JsonABI notoABI = new JsonABI(List.of(
            JsonABI.newFunction(
                    "mint",
                    JsonABI.newParameters(
                            JsonABI.newParameter("to", "string"),
                            JsonABI.newParameter("amount", "uint256"),
                            JsonABI.newParameter("data", "bytes")
                    ),
                    JsonABI.newParameters()
            )
    ));

    static final JsonABI notoTrackerABI = new JsonABI(Arrays.asList(
            JsonABI.newFunction(
                    "deploy",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("bytecode", "bytes"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                    JsonABI.newParameter("name", "string"),
                                    JsonABI.newParameter("symbol", "string")
                            ))
                    ),
                    JsonABI.newParameters()
            ),
            JsonABI.newFunction(
                    "balanceOf",
                    JsonABI.newParameters(
                            JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                    JsonABI.newParameter("salt", "bytes32"),
                                    JsonABI.newParameter("members", "string[]")
                            )),
                            JsonABI.newParameter("to", "address"),
                            JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                                    JsonABI.newParameter("account", "address")
                            ))
                    ),
                    JsonABI.newParameters(
                            JsonABI.newParameter("output", "uint256")
                    )
            )
    ));

    @JsonIgnoreProperties(ignoreUnknown = true)
    record PenteCallOutputJSON(
            @JsonProperty
            String output
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record StateSchema(
            @JsonProperty
            JsonHex.Bytes32 id,
            @JsonProperty
            String signature
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoCoin(
            @JsonProperty
            NotoCoinData data
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoCoinData(
            @JsonProperty
            String owner,
            @JsonProperty
            String amount
    ) {
    }

    Testbed.TransactionResult getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    @Test
    void testNotoPente() throws Exception {
        JsonHex.Address penteFactoryAddress = deployPenteFactory();
        JsonHex.Address notoFactoryAddress = deployNotoFactory();
        JsonHex.Bytes32 groupSalt = JsonHex.randomBytes32();
        try (Testbed testbed = new Testbed(testbedSetup,
                new Testbed.ConfigDomain(
                        "pente",
                        penteFactoryAddress,
                        new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()),
                        new HashMap<>()
                ),
                new Testbed.ConfigDomain(
                        "noto",
                        notoFactoryAddress,
                        new Testbed.ConfigPlugin("c-shared", "noto", ""),
                        new HashMap<>()
                )
        )) {

            PenteConfiguration.GroupTupleJSON groupInfo = new PenteConfiguration.GroupTupleJSON(
                    groupSalt,
                    new String[]{"notary"}
            );

            var mapper = new ObjectMapper();

            List<HashMap<String, Object>> notoSchemas = testbed.getRpcClient().request("pstate_listSchemas", "noto");
            StateSchema notoSchema = null;
            for (var schemaJson : notoSchemas) {
                var schema = mapper.convertValue(schemaJson, StateSchema.class);
                if (schema.signature().startsWith("type=NotoCoin")) {
                    notoSchema = schema;
                }
            }
            assertNotNull(notoSchema);

            // Create the privacy group
            String penteInstanceAddress = testbed.getRpcClient().request("testbed_deploy",
                    "pente", "notary",
                    new PenteConfiguration.PrivacyGroupConstructorParamsJSON(
                            groupInfo,
                            "shanghai",
                            PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
                            true
                    ));
            assertFalse(penteInstanceAddress.isBlank());

            // Deploy NotoTracker to the privacy group
            String notoTrackerBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/private/NotoTrackerERC20.sol/NotoTrackerERC20.json",
                    "bytecode"
            );
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new Testbed.TransactionInput(
                                    "private",
                                    "",
                                    "notary",
                                    JsonHex.addressFrom(penteInstanceAddress),
                                    new HashMap<>() {{
                                        put("group", groupInfo);
                                        put("bytecode", notoTrackerBytecode);
                                        put("inputs", new HashMap<>() {{
                                            put("name", "NOTO");
                                            put("symbol", "NOTO");
                                        }});
                                    }},
                                    notoTrackerABI,
                                    "deploy"
                            ), true));
            var domainReceipt = mapper.convertValue(tx.domainReceipt(), PenteEVMTransaction.JSONReceipt.class);
            var notoTrackerAddress = domainReceipt.receipt().contractAddress();

            // Create Noto token
            String notoInstanceAddress = testbed.getRpcClient().request("testbed_deploy",
                    "noto", "notary",
                    new NotoConstructorParamsJSON(
                            "notary@node1",
                            "hooks",
                            new NotoOptionsJSON(
                                    new NotoHookParamsJSON(
                                            penteInstanceAddress,
                                            notoTrackerAddress,
                                            groupInfo))));
            assertFalse(notoInstanceAddress.isBlank());

            // Perform Noto mint
            testbed.getRpcClient().request("testbed_invoke",
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "notary",
                            JsonHex.addressFrom(notoInstanceAddress),
                            new HashMap<>() {{
                                put("to", "alice");
                                put("amount", 1000000);
                                put("data", "0x");
                            }},
                            notoABI,
                            "mint"
                    ), true);

            // Validate minted coin
            List<JsonNode> notoStates = testbed.getRpcClient().request("pstate_queryContractStates",
                    "noto",
                    notoInstanceAddress,
                    notoSchema.id,
                    null,
                    "available");
            assertEquals(1, notoStates.size());
            var notoCoin = mapper.convertValue(notoStates.getFirst(), NotoCoin.class);
            assertEquals("1000000", notoCoin.data.amount);

            String aliceAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    "alice", Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
            assertEquals(aliceAddress, notoCoin.data.owner);

            // Validate ERC20 balance
            LinkedHashMap<String, Object> balanceResult = testbed.getRpcClient().request("testbed_call",
                    new Testbed.TransactionInput(
                            "private",
                            "",
                            "notary",
                            JsonHex.addressFrom(penteInstanceAddress),
                            new HashMap<>() {{
                                put("group", groupInfo);
                                put("to", notoTrackerAddress.toString());
                                put("inputs", new HashMap<>() {{
                                    put("account", aliceAddress);
                                }});
                            }},
                            notoTrackerABI,
                            "balanceOf"
                    ), "");

            var aliceBalance = mapper.convertValue(balanceResult, PenteCallOutputJSON.class);
            assertEquals("1000000", aliceBalance.output);
        }
    }
}
