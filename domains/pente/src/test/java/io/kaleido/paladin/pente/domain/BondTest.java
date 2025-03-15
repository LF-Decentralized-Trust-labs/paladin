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

import static org.junit.jupiter.api.Assertions.*;

public class BondTest {

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
    void testBond() throws Exception {
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
            String bondCustodian = "bondCustodian";
            String alice = "alice";

            String custodianAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    bondCustodian, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
            String aliceAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    alice, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);

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

            String bondTrackerPublicBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/shared/BondTrackerPublic.sol/BondTrackerPublic.json",
                    "bytecode"
            );
            JsonABI bondTrackerPublicABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/shared/BondTrackerPublic.sol/BondTrackerPublic.json",
                    "abi"
            );
            String atomFactoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/shared/Atom.sol/AtomFactory.json",
                    "bytecode"
            );
            JsonABI atomFactoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/shared/Atom.sol/AtomFactory.json",
                    "abi"
            );
            JsonABI atomABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/shared/Atom.sol/Atom.json",
                    "abi"
            );

            GroupTupleJSON issuerCustodianGroup = new GroupTupleJSON(
                    JsonHex.randomBytes32(), new String[]{bondIssuer, bondCustodian});
            GroupTupleJSON aliceCustodianGroup = new GroupTupleJSON(
                    JsonHex.randomBytes32(), new String[]{alice, bondCustodian});

            // Create the privacy groups
            var issuerCustodianInstance = PenteHelper.newPrivacyGroup(
                    "pente", alice, testbed, issuerCustodianGroup, true);
            assertFalse(issuerCustodianInstance.address().isBlank());
            var aliceCustodianInstance = PenteHelper.newPrivacyGroup(
                    "pente", alice, testbed, aliceCustodianGroup, true);
            assertFalse(aliceCustodianInstance.address().isBlank());

            // Create Noto cash token
            var notoCash = NotoHelper.deploy("noto", cashIssuer, testbed,
                    new NotoHelper.ConstructorParams(
                            cashIssuer + "@node1",
                            "basic",
                            null));
            assertFalse(notoCash.address().isBlank());

            // Create the public bond tracker on the base ledger (controlled by the privacy group)
            String bondTrackerPublicAddress = testbed.getRpcClient().request("testbed_deployBytecode",
                    "issuer",
                    bondTrackerPublicABI,
                    bondTrackerPublicBytecode,
                    new HashMap<String, String>() {{
                        put("owner", issuerCustodianInstance.address());
                        put("issueDate_", "0");
                        put("maturityDate_", "1");
                        put("currencyToken_", notoCash.address());
                        put("faceValue_", "1");
                    }});

            // Deploy private bond tracker to the issuer/custodian privacy group
            var bondTracker = BondTrackerHelper.deploy(issuerCustodianInstance, bondIssuer, new HashMap<>() {{
                put("name", "BOND");
                put("symbol", "BOND");
                put("custodian", custodianAddress);
                put("publicTracker", bondTrackerPublicAddress);
            }});

            // Create Noto bond token
            var notoBond = NotoHelper.deploy("noto", bondCustodian, testbed,
                    new NotoHelper.ConstructorParams(
                            bondCustodian + "@node1",
                            "hooks",
                            new NotoHelper.OptionsParams(
                                    new NotoHelper.HookParams(
                                            issuerCustodianInstance.address(),
                                            bondTracker.address(),
                                            issuerCustodianGroup))));
            assertFalse(notoBond.address().isBlank());

            // Issue cash to investors
            notoCash.mint(cashIssuer, alice, 100000);

            // Issue bond to custodian
            notoBond.mint(bondIssuer, bondCustodian, 1000);

            // Validate Noto balances
            var notoCashStates = notoCash.queryStates(notoSchema.id, null);
            assertEquals(1, notoCashStates.size());
            assertEquals("100000", notoCashStates.getFirst().data().amount());
            assertEquals(aliceAddress, notoCashStates.getFirst().data().owner());
            var notoBondStates = notoBond.queryStates(notoSchema.id, null);
            assertEquals(1, notoBondStates.size());
            assertEquals("1000", notoBondStates.getFirst().data().amount());
            assertEquals(custodianAddress, notoBondStates.getFirst().data().owner());

            // Validate bond tracker balance
            assertEquals("1000", bondTracker.balanceOf(bondIssuer, custodianAddress));

            // Begin bond distribution
            bondTracker.beginDistribution(bondCustodian, 1, 1);

            // Add Alice as an allowed investor
            var investorList = bondTracker.investorList(bondCustodian);
            investorList.addInvestor(bondCustodian, aliceAddress);

            // Create the atom factory on the base ledger
            String atomFactoryAddress = testbed.getRpcClient().request("testbed_deployBytecode",
                    "issuer",
                    atomFactoryABI,
                    atomFactoryBytecode,
                    new HashMap<String, String>());

            // Alice deploys BondSubscription to the alice/custodian privacy group, to request subscription
            // TODO: if Alice deploys, how can custodian trust it's the correct logic?
            var bondSubscription = BondSubscriptionHelper.deploy(aliceCustodianInstance, alice, new HashMap<>() {{
                put("bondAddress_", notoBond.address());
                put("units_", 1000);
                put("custodian_", custodianAddress);
                put("atomFactory_", atomFactoryAddress);
            }});

            // Prepare the bond transfer (requires 2 calls to prepare, as the Noto transaction spawns a Pente transaction to wrap it)
            var bondTransfer = notoBond.prepareTransfer(bondCustodian, alice, 1000);
            assertEquals("private", bondTransfer.preparedTransaction().type());
            assertEquals("pente", bondTransfer.preparedTransaction().domain());
            assertEquals(issuerCustodianInstance.address(), bondTransfer.preparedTransaction().to().toString());
            assertEquals(1, bondTransfer.preparedTransaction().abi().size());
            var bondTransfer2 = issuerCustodianInstance.prepare(
                    bondTransfer.preparedTransaction().from(),
                    bondTransfer.preparedTransaction().abi().getFirst(),
                    bondTransfer.preparedTransaction().data()
            );
            assertEquals("public", bondTransfer2.preparedTransaction().type());
            var bondTransferMetadata = mapper.convertValue(bondTransfer2.preparedMetadata(), PenteHelper.PenteTransitionMetadata.class);

            // Prepare the payment transfer
            var paymentTransfer = notoCash.prepareTransfer(alice, bondCustodian, 1000);
            assertEquals("public", paymentTransfer.preparedTransaction().type());
            var paymentMetadata = mapper.convertValue(paymentTransfer.preparedMetadata(), NotoHelper.NotoTransferMetadata.class);

            // Pass the prepared transfers to the subscription contract
            bondSubscription.prepareBond(bondCustodian, bondTransfer2.preparedTransaction().to(), bondTransferMetadata.transitionWithApproval().encodedCall());
            bondSubscription.preparePayment(alice, paymentTransfer.preparedTransaction().to(), paymentMetadata.transferWithApproval().encodedCall());

            // Alice receives full bond distribution
            var distributeTX = bondSubscription.distribute(bondCustodian);

            // Look up the deployed Atom address
            HashMap<String, Object> distributeReceipt = testbed.getRpcClient().request("ptx_getTransactionReceipt", distributeTX.id());
            String distributeTXHash = distributeReceipt.get("transactionHash").toString();
            List<HashMap<String, Object>> events = testbed.getRpcClient().request("bidx_decodeTransactionEvents",
                    distributeTXHash,
                    atomFactoryABI,
                    "");
            var deployEvent = events.stream().filter(ev -> ev.get("soliditySignature").toString().startsWith("event AtomDeployed")).findFirst();
            assertFalse(deployEvent.isEmpty());
            var deployEventData = mapper.convertValue(deployEvent.get().get("data"), HashMap.class);
            var atomAddress = JsonHex.addressFrom(deployEventData.get("addr").toString());

            // Alice approves payment transfer
            notoCash.approveTransfer(
                    "alice",
                    paymentTransfer.inputStates(),
                    paymentTransfer.outputStates(),
                    paymentMetadata.approvalParams().data(),
                    atomAddress.toString());

            // Custodian approves bond transfer
            var txID = issuerCustodianInstance.approveTransition(
                    bondCustodian,
                    JsonHex.randomBytes32(),
                    atomAddress,
                    bondTransferMetadata.approvalParams().transitionHash(),
                    bondTransferMetadata.approvalParams().signatures());
            var receipt = TestbedHelper.pollForReceipt(testbed, txID, 3000);
            assertNotNull(receipt);

            // Execute the Atom
            txID = TestbedHelper.sendTransaction(testbed,
                    new Testbed.TransactionInput(
                            "public",
                            "",
                            bondCustodian,
                            atomAddress,
                            new HashMap<>(),
                            atomABI,
                            "execute"
                    ));
            receipt = TestbedHelper.pollForReceipt(testbed, txID, 3000);
            assertNotNull(receipt);

            // All prepared transactions should now be resolved
            receipt = TestbedHelper.pollForReceipt(testbed, paymentTransfer.id(), 3000);
            assertNotNull(receipt);
            receipt = TestbedHelper.pollForReceipt(testbed, bondTransfer2.id(), 3000);
            assertNotNull(receipt);
            receipt = TestbedHelper.pollForReceipt(testbed, bondTransfer.id(), 3000);
            assertNotNull(receipt);

            // TODO: figure out how to test negative cases (such as when Pente reverts due to a non-allowed investor)

            // Validate Noto balance
            notoCashStates = notoCash.queryStates(notoSchema.id, null);
            assertEquals(2, notoCashStates.size());
            assertEquals("1000", notoCashStates.get(0).data().amount());
            assertEquals(custodianAddress, notoCashStates.get(0).data().owner());
            assertEquals("99000", notoCashStates.get(1).data().amount());
            assertEquals(aliceAddress, notoCashStates.get(1).data().owner());
            notoBondStates = notoBond.queryStates(notoSchema.id, null);
            assertEquals(1, notoBondStates.size());
            assertEquals("1000", notoBondStates.getFirst().data().amount());
            assertEquals(aliceAddress, notoBondStates.getFirst().data().owner());

            // Validate bond tracker balance
            assertEquals("0", bondTracker.balanceOf(bondIssuer, custodianAddress));
            assertEquals("1000", bondTracker.balanceOf(bondIssuer, aliceAddress));
        }
    }
}
