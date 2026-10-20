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
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.UUID;
import java.util.function.Function;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;

class NotoInstanceTest {

  private static final String TX_ID = "00000000-0000-0000-0000-0000000000aa";
  private static final EthAddress ADDRESS =
      EthAddress.fromString("0x0102030405060708090a0b0c0d0e0f1011121314");
  private static final EthAddress DELEGATE =
      EthAddress.fromString("0x1111111111111111111111111111111111111111");
  private static final Bytes32 LOCK_ID = Bytes32.fromString("0x" + "ab".repeat(32));
  private static final HexUint256 AMOUNT = HexUint256.of(10);
  private static final HexBytes DATA = HexBytes.fromString("0x1234");
  private static final HexBytes UNLOCK_DATA = HexBytes.fromString("0x5678");
  private static final List<UnlockRecipient> RECIPIENTS =
      List.of(new UnlockRecipient("bob", HexUint256.of(5)));

  private FakeRpcClient rpc;
  private NotoInstance token;

  @BeforeEach
  void setUp() {
    rpc = new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");
    token = new NotoInstance(PaladinClient.wrap(rpc), "noto-test", ADDRESS);
  }

  /** One transaction operation: its ABI function, inputs, whether it is public, and the call. */
  private record Op(
      String function,
      Object params,
      boolean isPublic,
      Function<NotoInstance, SentTransaction> send) {
    @Override
    public String toString() {
      return function;
    }
  }

  @SuppressWarnings("deprecation")
  static Stream<Arguments> operations() {
    final NotoMintParams mint = new NotoMintParams("bob", AMOUNT, DATA);
    final NotoTransferParams transfer = new NotoTransferParams("bob", AMOUNT, DATA);
    final NotoTransferFromParams transferFrom =
        new NotoTransferFromParams("carol", "bob", AMOUNT, DATA);
    final NotoBurnParams burn = new NotoBurnParams(AMOUNT, DATA);
    final NotoBurnFromParams burnFrom = new NotoBurnFromParams("carol", AMOUNT, DATA);
    final NotoLockParams lock = new NotoLockParams(AMOUNT, DATA);
    final NotoCreateTransferLockParams createTransferLock =
        new NotoCreateTransferLockParams("alice", RECIPIENTS, UNLOCK_DATA, DATA);
    final NotoCreateMintLockParams createMintLock =
        new NotoCreateMintLockParams(RECIPIENTS, UNLOCK_DATA, DATA);
    final NotoCreateBurnLockParams createBurnLock =
        new NotoCreateBurnLockParams("alice", AMOUNT, UNLOCK_DATA, DATA);
    final NotoUnlockParams unlock = new NotoUnlockParams(LOCK_ID, "alice", RECIPIENTS, DATA);
    final NotoPrepareUnlockParams prepareUnlock =
        new NotoPrepareUnlockParams(LOCK_ID, "alice", RECIPIENTS, UNLOCK_DATA, DATA);
    final NotoPrepareMintUnlockParams prepareMintUnlock =
        new NotoPrepareMintUnlockParams(LOCK_ID, RECIPIENTS, UNLOCK_DATA, DATA);
    final NotoPrepareBurnUnlockParams prepareBurnUnlock =
        new NotoPrepareBurnUnlockParams(LOCK_ID, "alice", AMOUNT, UNLOCK_DATA, DATA);
    final NotoDelegateLockParams delegateLock = new NotoDelegateLockParams(LOCK_ID, DELEGATE, DATA);
    final NotoSpendLockParams spendLock =
        new NotoSpendLockParams(LOCK_ID, HexBytes.fromString("0x01"), DATA);
    final NotoCancelLockParams cancelLock =
        new NotoCancelLockParams(LOCK_ID, HexBytes.fromString("0x02"), DATA);
    return Stream.of(
            new Op("mint", mint, false, t -> t.mint("alice", mint)),
            new Op("transfer", transfer, false, t -> t.transfer("alice", transfer)),
            new Op("transferFrom", transferFrom, false, t -> t.transferFrom("alice", transferFrom)),
            new Op("burn", burn, false, t -> t.burn("alice", burn)),
            new Op("burnFrom", burnFrom, false, t -> t.burnFrom("alice", burnFrom)),
            new Op("lock", lock, false, t -> t.lock("alice", lock)),
            new Op("createLock", lock, false, t -> t.createLock("alice", lock)),
            new Op(
                "createTransferLock",
                createTransferLock,
                false,
                t -> t.createTransferLock("alice", createTransferLock)),
            new Op(
                "createMintLock",
                createMintLock,
                false,
                t -> t.createMintLock("alice", createMintLock)),
            new Op(
                "createBurnLock",
                createBurnLock,
                false,
                t -> t.createBurnLock("alice", createBurnLock)),
            new Op("unlock", unlock, false, t -> t.unlock("alice", unlock)),
            new Op(
                "prepareUnlock",
                prepareUnlock,
                false,
                t -> t.prepareUnlock("alice", prepareUnlock)),
            new Op(
                "prepareMintUnlock",
                prepareMintUnlock,
                false,
                t -> t.prepareMintUnlock("alice", prepareMintUnlock)),
            new Op(
                "prepareBurnUnlock",
                prepareBurnUnlock,
                false,
                t -> t.prepareBurnUnlock("alice", prepareBurnUnlock)),
            new Op("delegateLock", delegateLock, false, t -> t.delegateLock("alice", delegateLock)),
            new Op("spendLock", spendLock, true, t -> t.spendLock("alice", spendLock)),
            new Op("cancelLock", cancelLock, true, t -> t.cancelLock("alice", cancelLock)))
        .map(Arguments::of);
  }

  @ParameterizedTest
  @MethodSource("operations")
  void submitsEachOperation(final Op op) {
    final SentTransaction sent = op.send().apply(token);

    assertEquals(UUID.fromString(TX_ID), sent.id().join());
    final JsonNode body = rpc.lastCall("ptx_sendTransaction").body();
    assertEquals(op.isPublic() ? "public" : "private", body.get("type").asText());
    if (op.isPublic()) {
      assertFalse(body.has("domain"));
    } else {
      assertEquals("noto-test", body.get("domain").asText());
    }
    assertEquals(op.function(), body.get("function").asText());
    assertEquals(ADDRESS.toString(), body.get("to").asText());
    assertEquals("alice", body.get("from").asText());
    assertFalse(body.has("idempotencyKey"));
    assertEquals(PaladinObjectMapper.shared().valueToTree(op.params()), body.get("data"));
    assertTrue(hasFunction(body.get("abi"), op.function()), "ABI lacks " + op.function());
  }

  @Test
  void sendsInputsUnderTheirAbiNames() {
    token.unlock("alice", new NotoUnlockParams(LOCK_ID, "alice", RECIPIENTS, null));

    final JsonNode data = rpc.lastCall("ptx_sendTransaction").body().get("data");
    assertEquals(LOCK_ID.toString(), data.get("lockId").asText());
    assertEquals("alice", data.get("from").asText());
    assertEquals("bob", data.get("recipients").get(0).get("to").asText());
    assertEquals("0x05", data.get("recipients").get(0).get("amount").asText());
    assertEquals("0x", data.get("data").asText());
  }

  @Test
  void forwardsTheIdempotencyKey() {
    token.mint("alice", new NotoMintParams("bob", AMOUNT, DATA), "mint-1");
    token.spendLock("alice", new NotoSpendLockParams(LOCK_ID, DATA, DATA), "spend-1");

    final List<FakeRpcClient.Call> sends =
        rpc.calls().stream().filter(c -> c.method().equals("ptx_sendTransaction")).toList();
    assertEquals("mint-1", sends.get(0).body().get("idempotencyKey").asText());
    assertEquals("spend-1", sends.get(1).body().get("idempotencyKey").asText());
  }

  @Test
  void readsNameSymbolAndDecimals() {
    rpc.respond("ptx_call", "\"Cash\"");
    assertEquals("Cash", token.name("alice").join());
    assertCall("name");

    rpc.respond("ptx_call", "\"CSH\"");
    assertEquals("CSH", token.symbol("alice").join());
    assertCall("symbol");

    rpc.respond("ptx_call", "4");
    assertEquals(4, token.decimals("alice").join());
    assertCall("decimals");
  }

  @Test
  void readsBalance() {
    rpc.respond(
        "ptx_call", "{\"totalBalance\":\"0x64\",\"totalStates\":\"0x2\",\"overflow\":true}");

    final NotoBalanceOfResult balance = token.balanceOf("alice", "bob").join();

    assertEquals(HexUint256.of(100), balance.totalBalance());
    assertEquals(HexUint256.of(2), balance.totalStates());
    assertTrue(balance.overflow());
    final JsonNode body = assertCall("balanceOf");
    assertEquals("bob", body.get("data").get("account").asText());
  }

  @Test
  void exposesAddressAndDomain() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    final NotoInstance defaulted = new NotoInstance(paladin, ADDRESS);

    assertEquals(ADDRESS, defaulted.address());
    assertEquals(NotoInstance.DEFAULT_DOMAIN, defaulted.domain());
    assertEquals("noto", NotoInstance.DEFAULT_DOMAIN);
    assertEquals("noto-test", token.domain());
  }

  @Test
  void usingSubmitsThroughAnotherClient() {
    final FakeRpcClient other =
        new FakeRpcClient().respond("ptx_sendTransaction", "\"" + TX_ID + "\"");

    final NotoInstance moved = token.using(PaladinClient.wrap(other));
    moved.burn("alice", new NotoBurnParams(AMOUNT, null));

    assertEquals(ADDRESS, moved.address());
    assertEquals("noto-test", moved.domain());
    assertEquals(1, other.calls().size());
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void rejectsMissingArguments() {
    final PaladinClient paladin = PaladinClient.wrap(rpc);
    assertThrows(NullPointerException.class, () -> new NotoInstance(null, ADDRESS));
    assertThrows(NullPointerException.class, () -> new NotoInstance(paladin, null, ADDRESS));
    assertThrows(NullPointerException.class, () -> new NotoInstance(paladin, "noto", null));
    assertThrows(NullPointerException.class, () -> token.mint("alice", null));
    assertThrows(NullPointerException.class, () -> token.balanceOf("alice", null));
    assertTrue(rpc.calls().isEmpty());
  }

  @Test
  void surfacesRpcFailures() {
    final FakeRpcClient failing = new FakeRpcClient();
    final NotoInstance broken = token.using(PaladinClient.wrap(failing));

    final SentTransaction sent =
        broken.transfer("alice", new NotoTransferParams("bob", AMOUNT, null));

    assertSame(
        IllegalStateException.class,
        assertThrows(Exception.class, () -> sent.id().join()).getCause().getClass());
  }

  private JsonNode assertCall(final String function) {
    final JsonNode body = rpc.lastCall("ptx_call").body();
    assertEquals("private", body.get("type").asText());
    assertEquals("noto-test", body.get("domain").asText());
    assertEquals(function, body.get("function").asText());
    assertEquals(ADDRESS.toString(), body.get("to").asText());
    assertEquals("alice", body.get("from").asText());
    return body;
  }

  private static boolean hasFunction(final JsonNode abi, final String function) {
    for (final JsonNode entry : abi) {
      if (function.equals(entry.path("name").asText())) {
        return true;
      }
    }
    return false;
  }
}
