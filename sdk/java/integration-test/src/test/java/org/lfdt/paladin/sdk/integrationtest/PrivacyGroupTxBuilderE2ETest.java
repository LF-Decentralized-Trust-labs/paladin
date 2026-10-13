/*
 * Copyright contributors to Paladin, an LFDT project
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
package org.lfdt.paladin.sdk.integrationtest;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/** Exercises the Java builder against the operator's three-node installation. */
@Tag("operator-e2e")
class PrivacyGroupTxBuilderE2ETest {

  @Test
  @Timeout(180)
  void deployMintTransferAndReadAcrossGroupMembers() throws Exception {
    final JsonNode artifact =
        PaladinObjectMapper.shared()
            .readTree(Files.readString(Path.of(System.getProperty("paladin.erc20Artifact"))));
    final List<AbiEntry> abi =
        List.of(PaladinObjectMapper.shared().treeToValue(artifact.get("abi"), AbiEntry[].class));
    final String bytecode = artifact.get("bytecode").asText();
    final String suffix = UUID.randomUUID().toString();
    final String alice = "java.alice." + suffix + "@node1";
    final String bob = "java.bob." + suffix + "@node2";
    try (PaladinClient node1 = PaladinClient.http(System.getProperty("paladin.node1"));
        PaladinClient node2 = PaladinClient.http(System.getProperty("paladin.node2"));
        PaladinClient node3 = PaladinClient.http(System.getProperty("paladin.node3"))) {
      final PrivacyGroup group =
          node1
              .privacyGroups()
              .createGroup(
                  PrivacyGroupInput.builder("pente")
                      .name("java-txbuilder-" + suffix)
                      .members(List.of(alice, bob))
                      .configuration("evmVersion", "shanghai")
                      .configuration("endorsementType", "group_scoped_identities")
                      .configuration("externalCallsEnabled", "true")
                      .build())
              .join();
      awaitGroup(node1, group);
      awaitGroup(node2, group);
      assertNull(
          node3.privacyGroups().getGroupById("pente", group.id()).join(),
          "the excluded node must not receive the group");

      final SentTransaction deploy =
          node1
              .newTx()
              .privacyGroup(group)
              .from(alice)
              .constructor()
              .abi(abi)
              .bytecode(bytecode)
              .inputs(Map.of("name", "Java Stars", "symbol", "JSTAR"))
              .idempotencyKey("java-deploy-" + suffix)
              .send();
      assertSuccess(deploy);
      final JsonNode domainReceipt =
          node1.ptx().getDomainReceipt("pente", deploy.id().join()).join();
      final String contract = domainReceipt.path("receipt").path("contractAddress").asText();
      assertTrue(
          contract.startsWith("0x"), "Pente receipt must contain the private contract address");
      final String aliceAddress =
          node1.ptx().resolveVerifier(alice, "ecdsa:secp256k1", "eth_address").join();
      final String bobAddress =
          node2.ptx().resolveVerifier(bob, "ecdsa:secp256k1", "eth_address").join();
      assertSuccess(
          node1
              .newTx()
              .privacyGroup(group)
              .from(alice)
              .to(contract)
              .abi(abi)
              .function("mint")
              .inputs(Map.of("to", aliceAddress, "amount", 100))
              .send());
      assertSuccess(
          node1
              .newTx()
              .privacyGroupId(group.id())
              .domain("pente")
              .from(alice)
              .to(contract)
              .abi(abi)
              .function("transfer(address,uint256)")
              .inputs(Map.of("to", bobAddress, "value", 25))
              .send());

      // Read on each member so the test checks distribution as well as local execution.
      awaitBalance(node1, group, contract, abi, alice, aliceAddress, "75");
      awaitBalance(node2, group, contract, abi, bob, bobAddress, "25");
      assertSuccess(
          node2
              .newTx()
              .privacyGroup(group)
              .from(bob)
              .to(contract)
              .abi(abi)
              .function("transfer")
              .inputs(Map.of("to", aliceAddress, "value", 5))
              .send());
      awaitBalance(node1, group, contract, abi, alice, aliceAddress, "80");
      awaitBalance(node2, group, contract, abi, bob, bobAddress, "20");
    }
  }

  private static void assertSuccess(final SentTransaction sent) {
    final TransactionReceipt receipt = sent.waitForReceipt(Duration.ofSeconds(30)).join();
    assertTrue(receipt.success(), receipt.failureMessage());
  }

  private static void awaitGroup(final PaladinClient node, final PrivacyGroup group)
      throws InterruptedException {
    final long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
    PrivacyGroup found = null;
    while (System.nanoTime() < deadline) {
      found = node.privacyGroups().getGroupById("pente", group.id()).join();
      if (found != null && found.contractAddress() != null) {
        return;
      }
      Thread.sleep(200);
    }
    assertNotNull(found, "privacy group did not arrive");
    assertNotNull(found.contractAddress(), "privacy group genesis did not confirm");
  }

  private static void awaitBalance(
      final PaladinClient node,
      final PrivacyGroup group,
      final String contract,
      final List<AbiEntry> abi,
      final String from,
      final String account,
      final String expected)
      throws InterruptedException {
    final long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
    String actual = null;
    while (System.nanoTime() < deadline) {
      final JsonNode result =
          node.newTx()
              .privacyGroup(group)
              .from(from)
              .to(contract)
              .abi(abi)
              .function("balanceOf")
              .inputs(Map.of("account", account))
              .dataFormat("mode=array&number=string")
              .call()
              .join();
      actual = result.path(0).asText();
      if (expected.equals(actual)) {
        return;
      }
      Thread.sleep(200);
    }
    assertEquals(expected, actual, "balance did not propagate to the group member");
  }
}
