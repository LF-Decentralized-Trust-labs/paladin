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
import io.kaleido.paladin.pente.domain.helpers.*;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;

public class CSDBondTest {

    private final Testbed.Setup testbedSetup = new Testbed.Setup(
            "../../core/go/db/migrations/sqlite",
            "build/testbed.java-bond.log",
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
    record StateSchema(
            @JsonProperty
            JsonHex.Bytes32 id,
            @JsonProperty
            String signature
    ) {
    }

    @Test
    void testCSDBond() throws Exception {
        JsonHex.Address penteFactoryAddress = deployPenteFactory();
        JsonHex.Address notoFactoryAddress = deployNotoFactory();
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

            String cashIssuer = "cashIssuer";
            String bondIssuer = "bondIssuer";
            String bondChecker = "bondChecker";
            String alice = "alice";

            String issuerAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    bondIssuer, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
            String aliceAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    alice, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);

            var mapper = new ObjectMapper();
            List<JsonNode> notoSchemas = testbed.getRpcClient().request("pstate_listSchemas",
                    "noto");
            assertEquals(2, notoSchemas.size());
            var notoSchema = mapper.convertValue(notoSchemas.getLast(), StateSchema.class);
            assertEquals("type=NotoCoin(bytes32 salt,string owner,uint256 amount),labels=[owner,amount]",
                    notoSchema.signature());

            GroupTupleJSON issuerGroup = new GroupTupleJSON(
                    JsonHex.randomBytes32(), new String[]{bondIssuer});

            // Create the privacy groups
            var issuerInstance = PenteHelper.newPrivacyGroup(
                    "pente", alice, testbed, issuerGroup, true);
            assertFalse(issuerInstance.address().isBlank());

            // Create Noto cash token
            var notoCash = NotoHelper.deploy("noto", cashIssuer, testbed,
                    new NotoHelper.ConstructorParams(
                            cashIssuer + "@node1",
                            null,
                            true));
            assertFalse(notoCash.address().isBlank());

            // Deploy private investor list to the issuer privacy group
            var investorList = InvestorListHelper.deploy(issuerInstance, bondIssuer, new HashMap<>() {{
                put("initialOwner", issuerAddress);
            }});

            // Deploy private bond tracker to the issuer privacy group
            var bondTracker = CSDBondTrackerHelper.deploy(issuerInstance, bondIssuer, new HashMap<>() {{
                put("name", "BOND");
                put("symbol", "BOND");
                put("transferPolicy_", investorList.address());
            }});

            // Perform bond pre-issuance workflow
            bondTracker.approveRequest(bondChecker);
            bondTracker.setISIN(bondIssuer, "ZZ0123456AB0");
            bondTracker.approveISIN(bondChecker);

            // Create Noto bond token
            var notoBond = NotoHelper.deploy("noto", bondIssuer, testbed,
                    new NotoHelper.ConstructorParams(
                            bondIssuer + "@node1",
                            new NotoHelper.HookParams(
                                    issuerInstance.address(),
                                    bondTracker.address(),
                                    issuerGroup),
                            false));
            assertFalse(notoBond.address().isBlank());

            // Issue cash to investors
            notoCash.mint(cashIssuer, alice, 100000);

            // Issue bond to issuer
            bondTracker.prepareIssuance(bondIssuer);
            notoBond.mint(bondChecker, bondIssuer, 1000);

            // Validate Noto balances
            var notoCashStates = notoCash.queryStates(notoSchema.id, null);
            assertEquals(1, notoCashStates.size());
            assertEquals("100000", notoCashStates.getFirst().data().amount());
            assertEquals(aliceAddress, notoCashStates.getFirst().data().owner());
            var notoBondStates = notoBond.queryStates(notoSchema.id, null);
            assertEquals(1, notoBondStates.size());
            assertEquals("1000", notoBondStates.getFirst().data().amount());
            assertEquals(issuerAddress, notoBondStates.getFirst().data().owner());

            // Validate bond tracker balance
            assertEquals("1000", bondTracker.balanceOf(bondIssuer, issuerAddress));
        }
    }
}
