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

import io.kaleido.paladin.Main;
import io.kaleido.paladin.pente.domain.PenteConfiguration.GroupTupleJSON;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.PrivateContractInvoke;
import io.kaleido.paladin.toolkit.ResourceLoader;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertEquals;

public class DomainIntegrationTests {

    private static final Logger LOGGER = LogManager.getLogger(DomainIntegrationTests.class);

    private final Testbed.Setup testbedSetup = new Testbed.Setup("../../core/go/db/migrations/sqlite", 5000);

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
                    new HashMap<String,String>());
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
                    new HashMap<String,String>());
            return new JsonHex.Address(contractAddr);
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record NotoConstructorParamsJSON(
            @JsonProperty
            String notary,
            @JsonProperty
            String guardPublicAddress,
            @JsonProperty
            JsonHex.Address guardPrivateAddress,
            @JsonProperty
            GroupTupleJSON guardPrivateGroup
    ) {
    }

    static final JsonABI.Entry notoMintABI = JsonABI.newFunction(
            "mint",
            JsonABI.newParameters(
                    JsonABI.newParameter("to", "string"),
                    JsonABI.newParameter("amount", "uint256")
            ),
            JsonABI.newParameters()
    );

    static final JsonABI.Entry notoTrackerDeployABI = JsonABI.newFunction(
            "deploy",
            JsonABI.newParameters(
                    JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                            JsonABI.newParameter("salt", "bytes32"),
                            JsonABI.newParameter("members", "string[]")
                    )),
                    JsonABI.newParameter("bytecode", "bytes"),
                    JsonABI.newTuple("inputs", "", JsonABI.newParameters(
                            JsonABI.newParameter("maxSupply", "uint256")
                    ))
            ),
            JsonABI.newParameters()
    );

    @JsonIgnoreProperties(ignoreUnknown = true)
    record StateSchema(
            @JsonProperty
            JsonHex.Bytes32 id
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
            String amount
    ) {
    }

    Testbed.PrivateContractTransaction getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.PrivateContractTransaction.class);
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
            List<JsonNode> notoSchemas = testbed.getRpcClient().request("pstate_listSchemas",
                    "noto");
            assertEquals(1, notoSchemas.size());
            var notoSchema = mapper.convertValue(notoSchemas.getFirst(), StateSchema.class);

            // Create the privacy group
            String penteInstanceAddress = testbed.getRpcClient().request("testbed_deploy",
                    "pente",
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
                    "contracts/private/NotoTrackerSimple.sol/NotoTrackerSimple.json",
                    "bytecode"
            );
            var tx = getTransactionInfo(
                    testbed.getRpcClient().request("testbed_invoke",
                            new PrivateContractInvoke(
                                    "notary",
                                    JsonHex.addressFrom(penteInstanceAddress),
                                    notoTrackerDeployABI,
                                    new HashMap<>() {{
                                        put("group", groupInfo);
                                        put("bytecode", notoTrackerBytecode);
                                        put("inputs", new HashMap<>() {{
                                            put("maxSupply", 1000000);
                                        }});
                                    }}
                            ), true));
            var extraData = new ObjectMapper().readValue(tx.extraData(), PenteConfiguration.TransactionExtraData.class);
            var notoTrackerAddress = extraData.contractAddress();

            // Create Noto token
            String notoInstanceAddress = testbed.getRpcClient().request("testbed_deploy",
                    "noto",
                    new NotoConstructorParamsJSON(
                            "notary",
                            penteInstanceAddress,
                            notoTrackerAddress,
                            groupInfo));
            assertFalse(notoInstanceAddress.isBlank());

            // Perform Noto mint
            testbed.getRpcClient().request("testbed_invoke",
                    new PrivateContractInvoke(
                            "notary",
                            JsonHex.addressFrom(notoInstanceAddress),
                            notoMintABI,
                            new HashMap<>() {{
                                put("to", "alice");
                                put("amount", 1000000);
                            }}
                    ), true);

            // Validate minted coin
            List<JsonNode> notoStates = testbed.getRpcClient().request("pstate_queryStates",
                    "noto",
                    notoInstanceAddress,
                    notoSchema.id,
                    null,
                    "available");
            assertEquals(1, notoStates.size());
            var notoCoin = mapper.convertValue(notoStates.getFirst(), NotoCoin.class);
            assertEquals("1000000", notoCoin.data.amount);

            // Minting more will fail (goes above maxSupply)
            // TODO: add a graceful way to test for failed transactions
//            testbed.getRpcClient().request("testbed_invoke",
//                    new PrivateContractInvoke(
//                            "notary",
//                            JsonHex.addressFrom(notoInstanceAddress),
//                            notoMintABI,
//                            new HashMap<>() {{
//                                put("to", "alice");
//                                put("amount", 1);
//                            }}
//                    ), true);
        }
    }
}
