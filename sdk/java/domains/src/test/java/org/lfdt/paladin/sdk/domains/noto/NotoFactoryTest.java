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
package org.lfdt.paladin.sdk.domains.noto;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.time.Duration;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletionException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;

class NotoFactoryTest {

  private static final String TX_ID = "00000000-0000-0000-0000-0000000000bb";
  private static final String CONTRACT = "0x0102030405060708090a0b0c0d0e0f1011121314";

  private FakeRpcClient rpc;
  private NotoFactory factory;

  @BeforeEach
  void setUp() {
    rpc = new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");
    factory = new NotoFactory(PaladinClient.wrap(rpc), "noto");
  }

  private static String receipt(final boolean success) {
    return "{\"id\":\""
        + TX_ID
        + "\",\"sequence\":1,\"success\":"
        + success
        + (success ? ",\"contractAddress\":\"" + CONTRACT + "\"" : ",\"failureMessage\":\"boom\"")
        + "}";
  }

  @Test
  void deploysABasicToken() {
    final NotoDeployment deployment =
        factory.newNoto(
            "alice",
            NotoConstructorParams.basic("notary@node1").withName("Cash").withSymbol("CSH"));

    assertEquals(UUID.fromString(TX_ID), deployment.id().join());
    assertSame(deployment.id(), deployment.transaction().id());
    final JsonNode body = rpc.lastCall("ptx_sendTransaction").body();
    assertEquals("private", body.get("type").asText());
    assertEquals("noto", body.get("domain").asText());
    assertEquals("alice", body.get("from").asText());
    assertFalse(body.has("function"));
    assertFalse(body.has("to"));
    assertFalse(body.has("idempotencyKey"));

    final JsonNode constructor = body.get("abi").get(0);
    assertEquals("constructor", constructor.get("type").asText());
    assertEquals("basic", optionsTuple(constructor).get("name").asText());

    final JsonNode data = body.get("data");
    assertEquals("Cash", data.get("name").asText());
    assertEquals("CSH", data.get("symbol").asText());
    assertEquals("notary@node1", data.get("notary").asText());
    assertEquals("basic", data.get("notaryMode").asText());
    final JsonNode basic = data.get("options").get("basic");
    assertTrue(basic.get("restrictMint").asBoolean());
    assertTrue(basic.get("allowBurn").asBoolean());
    assertTrue(basic.get("allowLock").asBoolean());
    assertFalse(data.get("options").has("hooks"));
  }

  @Test
  void deploysAHooksTokenWithAnIdempotencyKey() {
    final NotoHooksOptions hooks =
        new NotoHooksOptions(
            EthAddress.fromString(CONTRACT),
            new NotoPrivateGroup(Bytes32.fromString("0x" + "01".repeat(32)), List.of("a", "b")),
            null);

    factory.newNoto("alice", NotoConstructorParams.hooks("notary", hooks), "deploy-1");

    final JsonNode body = rpc.lastCall("ptx_sendTransaction").body();
    assertEquals("deploy-1", body.get("idempotencyKey").asText());
    assertEquals("hooks", optionsTuple(body.get("abi").get(0)).get("name").asText());
    final JsonNode data = body.get("data");
    assertEquals("hooks", data.get("notaryMode").asText());
    assertEquals("", data.get("name").asText());
    assertFalse(data.get("options").has("basic"));
    final JsonNode sent = data.get("options").get("hooks");
    assertEquals(CONTRACT, sent.get("publicAddress").asText());
    assertEquals("b", sent.get("privateGroup").get("members").get(1).asText());
    assertFalse(sent.has("privateAddress"));
  }

  @Test
  void waitForDeployReturnsTheToken() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final NotoInstance token =
        factory.newNoto("alice", NotoConstructorParams.basic("notary")).waitForDeploy().join();

    assertEquals(EthAddress.fromString(CONTRACT), token.address());
    assertEquals("noto", token.domain());
    assertEquals(TX_ID, rpc.lastCall("ptx_getTransactionReceipt").params().get(0).asText());
  }

  @Test
  void waitForDeployWithATimeoutReturnsTheToken() {
    rpc.respond("ptx_getTransactionReceipt", receipt(true));

    final NotoInstance token =
        factory
            .newNoto("alice", NotoConstructorParams.basic("notary"))
            .waitForDeploy(Duration.ofSeconds(5))
            .join();

    assertEquals(EthAddress.fromString(CONTRACT), token.address());
  }

  @Test
  void waitForDeployFailsWhenTheDeploymentFails() {
    rpc.respond("ptx_getTransactionReceipt", receipt(false));
    final NotoDeployment deployment = factory.newNoto("alice", NotoConstructorParams.basic("n"));

    final CompletionException failure =
        assertThrows(CompletionException.class, () -> deployment.waitForDeploy().join());

    final PaladinException cause = assertInstanceOf(PaladinException.class, failure.getCause());
    assertTrue(cause.getMessage().contains(TX_ID), cause.getMessage());
    assertTrue(cause.getMessage().contains("boom"), cause.getMessage());
  }

  @Test
  void waitForDeployFailsWhenNoContractWasCreated() {
    rpc.respond(
        "ptx_getTransactionReceipt", "{\"id\":\"" + TX_ID + "\",\"sequence\":1,\"success\":true}");
    final NotoDeployment deployment = factory.newNoto("alice", NotoConstructorParams.basic("n"));

    final CompletionException failure =
        assertThrows(
            CompletionException.class,
            () -> deployment.waitForDeploy(Duration.ofSeconds(5)).join());

    assertInstanceOf(PaladinException.class, failure.getCause());
  }

  @Test
  void usingDeploysThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");

    final NotoFactory moved = factory.using(PaladinClient.wrap(other));
    moved.newNoto("alice", NotoConstructorParams.basic("notary"));

    assertEquals("noto", moved.domain());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsMissingArguments() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    assertThrows(NullPointerException.class, () -> new NotoFactory(null, "noto"));
    assertThrows(NullPointerException.class, () -> new NotoFactory(paladin, null));
    assertThrows(NullPointerException.class, () -> factory.newNoto("alice", null));
    assertTrue(rpc.calls().isEmpty());
  }

  /** The single component of the constructor's {@code options} tuple: basic or hooks. */
  private static JsonNode optionsTuple(final JsonNode constructor) {
    final JsonNode options = constructor.get("inputs").get(4);
    assertEquals("options", options.get("name").asText());
    assertEquals(1, options.get("components").size());
    return options.get("components").get(0);
  }
}
