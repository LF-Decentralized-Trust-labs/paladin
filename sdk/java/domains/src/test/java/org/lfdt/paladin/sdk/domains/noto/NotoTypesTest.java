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
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.EntryType;
import org.lfdt.paladin.sdk.core.abi.StateMutability;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;

class NotoTypesTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();
  private static final Bytes32 LOCK_ID = Bytes32.fromString("0x" + "ab".repeat(32));
  private static final HexUint256 AMOUNT = HexUint256.of(7);
  private static final HexBytes EMPTY = HexBytes.fromString("0x");
  private static final List<UnlockRecipient> RECIPIENTS =
      List.of(new UnlockRecipient("bob", AMOUNT));

  @Test
  void unsetBytesDefaultToEmpty() {
    assertEquals(EMPTY, new NotoMintParams("bob", AMOUNT, null).data());
    assertEquals(EMPTY, new NotoTransferParams("bob", AMOUNT, null).data());
    assertEquals(EMPTY, new NotoTransferFromParams("a", "b", AMOUNT, null).data());
    assertEquals(EMPTY, new NotoBurnParams(AMOUNT, null).data());
    assertEquals(EMPTY, new NotoBurnFromParams("a", AMOUNT, null).data());
    assertEquals(EMPTY, new NotoLockParams(AMOUNT, null).data());
    assertEquals(EMPTY, new NotoDelegateLockParams(LOCK_ID, null, null).data());
    assertEquals(EMPTY, new NotoUnlockParams(LOCK_ID, "a", RECIPIENTS, null).data());

    final NotoCreateTransferLockParams transferLock =
        new NotoCreateTransferLockParams("a", RECIPIENTS, null, null);
    assertEquals(EMPTY, transferLock.unlockData());
    assertEquals(EMPTY, transferLock.data());
    final NotoCreateMintLockParams mintLock = new NotoCreateMintLockParams(RECIPIENTS, null, null);
    assertEquals(EMPTY, mintLock.unlockData());
    assertEquals(EMPTY, mintLock.data());
    final NotoCreateBurnLockParams burnLock = new NotoCreateBurnLockParams("a", AMOUNT, null, null);
    assertEquals(EMPTY, burnLock.unlockData());
    assertEquals(EMPTY, burnLock.data());
    final NotoPrepareUnlockParams prepareUnlock =
        new NotoPrepareUnlockParams(LOCK_ID, "a", RECIPIENTS, null, null);
    assertEquals(EMPTY, prepareUnlock.unlockData());
    assertEquals(EMPTY, prepareUnlock.data());
    final NotoPrepareMintUnlockParams prepareMint =
        new NotoPrepareMintUnlockParams(LOCK_ID, RECIPIENTS, null, null);
    assertEquals(EMPTY, prepareMint.unlockData());
    assertEquals(EMPTY, prepareMint.data());
    final NotoPrepareBurnUnlockParams prepareBurn =
        new NotoPrepareBurnUnlockParams(LOCK_ID, "a", AMOUNT, null, null);
    assertEquals(EMPTY, prepareBurn.unlockData());
    assertEquals(EMPTY, prepareBurn.data());
    final NotoSpendLockParams spend = new NotoSpendLockParams(LOCK_ID, null, null);
    assertEquals(EMPTY, spend.spendArgs());
    assertEquals(EMPTY, spend.data());
    final NotoCancelLockParams cancel = new NotoCancelLockParams(LOCK_ID, null, null);
    assertEquals(EMPTY, cancel.cancelArgs());
    assertEquals(EMPTY, cancel.data());
  }

  @Test
  void suppliedBytesAreKept() {
    final HexBytes data = HexBytes.fromString("0xbeef");
    assertSame(data, new NotoMintParams("bob", AMOUNT, data).data());
  }

  @Test
  void listsAreCopiedAndImmutable() {
    final List<UnlockRecipient> recipients = new ArrayList<>(RECIPIENTS);
    final NotoUnlockParams unlock = new NotoUnlockParams(LOCK_ID, "a", recipients, null);
    recipients.clear();
    assertEquals(RECIPIENTS, unlock.recipients());
    assertThrows(UnsupportedOperationException.class, () -> unlock.recipients().clear());

    final List<String> members = new ArrayList<>(List.of("a", "b"));
    final NotoPrivateGroup group = new NotoPrivateGroup(LOCK_ID, members);
    members.clear();
    assertEquals(List.of("a", "b"), group.members());

    assertThrows(NullPointerException.class, () -> new NotoCreateMintLockParams(null, null, null));
  }

  @Test
  void basicConstructorParamsUseDefaultOptions() {
    final NotoConstructorParams params = NotoConstructorParams.basic("notary");

    assertEquals("", params.name());
    assertEquals("", params.symbol());
    assertEquals("notary", params.notary());
    assertEquals(NotaryMode.BASIC, params.notaryMode());
    assertEquals(NotoBasicOptions.defaults(), params.options().basic());
    assertNull(params.options().hooks());
    assertEquals(new NotoBasicOptions(true, true, true), NotoBasicOptions.defaults());
  }

  @Test
  void constructorParamsCanBeRenamed() {
    final NotoBasicOptions rules = new NotoBasicOptions(false, null, true);
    final NotoConstructorParams params =
        NotoConstructorParams.basic("notary", rules).withName("Cash").withSymbol("CSH");

    assertEquals("Cash", params.name());
    assertEquals("CSH", params.symbol());
    assertSame(rules, params.options().basic());

    final JsonNode json = MAPPER.valueToTree(params);
    assertFalse(json.get("options").get("basic").get("restrictMint").asBoolean());
    assertFalse(json.get("options").get("basic").has("allowBurn"));
  }

  @Test
  void hooksConstructorParamsCarryTheHooks() {
    final NotoHooksOptions hooks =
        new NotoHooksOptions(
            EthAddress.fromString("0x0102030405060708090a0b0c0d0e0f1011121314"), null, null);
    final NotoConstructorParams params = NotoConstructorParams.hooks("notary", hooks);

    assertEquals(NotaryMode.HOOKS, params.notaryMode());
    assertSame(hooks, params.options().hooks());
    assertNull(params.options().basic());
  }

  @Test
  void constructorParamsRejectMissingFields() {
    final NotoOptions options = new NotoOptions(NotoBasicOptions.defaults(), null);
    assertThrows(
        NullPointerException.class,
        () -> new NotoConstructorParams("n", "s", null, NotaryMode.BASIC, options));
    assertThrows(
        NullPointerException.class, () -> new NotoConstructorParams("n", "s", "x", null, options));
    assertThrows(
        NullPointerException.class,
        () -> new NotoConstructorParams("n", "s", "x", NotaryMode.BASIC, null));
    assertThrows(NullPointerException.class, () -> NotoConstructorParams.basic("x", null));
    assertThrows(NullPointerException.class, () -> NotoConstructorParams.hooks("x", null));
  }

  @Test
  void notaryModeRoundTrips() throws Exception {
    assertEquals("\"hooks\"", MAPPER.writeValueAsString(NotaryMode.HOOKS));
    assertEquals(NotaryMode.BASIC, MAPPER.readValue("\"basic\"", NotaryMode.class));
    assertEquals(NotaryMode.HOOKS, NotaryMode.fromJson("HOOKS"));
    assertEquals("basic", NotaryMode.BASIC.jsonValue());
    assertThrows(IllegalArgumentException.class, () -> NotaryMode.fromJson("other"));
    assertThrows(IllegalArgumentException.class, () -> NotaryMode.fromJson(null));
  }

  @Test
  void balanceOfResultReadsHexValues() throws Exception {
    final NotoBalanceOfResult result =
        MAPPER.readValue(
            "{\"totalBalance\":\"0x64\",\"totalStates\":\"3\",\"overflow\":false,\"extra\":1}",
            NotoBalanceOfResult.class);

    assertEquals(new NotoBalanceOfResult(HexUint256.of(100), HexUint256.of(3), false), result);
  }

  @Test
  void abiSelectsTheConstructorForTheNotaryMode() {
    assertSame(NotoAbi.CONSTRUCTOR_BASIC, NotoAbi.constructor(NotaryMode.BASIC));
    assertSame(NotoAbi.CONSTRUCTOR_HOOKS, NotoAbi.constructor(NotaryMode.HOOKS));
    assertEquals(EntryType.CONSTRUCTOR, NotoAbi.CONSTRUCTOR_BASIC.type());
    assertEquals(
        List.of("name", "symbol", "notary", "notaryMode", "options"),
        NotoAbi.CONSTRUCTOR_HOOKS.inputs().stream().map(p -> p.name()).toList());
  }

  @Test
  void abiListsThePrivateAndPublicFunctions() {
    assertEquals(
        List.of(
            "mint",
            "burn",
            "burnFrom",
            "transfer",
            "transferFrom",
            "lock",
            "createLock",
            "unlock",
            "createTransferLock",
            "createMintLock",
            "createBurnLock",
            "prepareUnlock",
            "prepareMintUnlock",
            "prepareBurnUnlock",
            "delegateLock",
            "name",
            "symbol",
            "decimals",
            "balanceOf"),
        NotoAbi.PRIVATE.stream().map(AbiEntry::name).toList());
    assertEquals(
        List.of("spendLock", "cancelLock"), NotoAbi.PUBLIC.stream().map(AbiEntry::name).toList());

    final AbiEntry balanceOf = NotoAbi.PRIVATE.get(NotoAbi.PRIVATE.size() - 1);
    assertEquals(StateMutability.VIEW, balanceOf.stateMutability());
    assertEquals(3, balanceOf.outputs().size());
    final AbiEntry unlock = NotoAbi.PRIVATE.get(7);
    assertEquals("tuple[]", unlock.inputs().get(2).type());
    assertEquals(2, unlock.inputs().get(2).components().size());
    assertTrue(
        NotoAbi.PUBLIC.stream().allMatch(e -> e.stateMutability() == StateMutability.NONPAYABLE));
  }
}
