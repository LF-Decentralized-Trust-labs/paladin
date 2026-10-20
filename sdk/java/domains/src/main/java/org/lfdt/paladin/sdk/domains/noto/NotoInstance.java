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

import com.fasterxml.jackson.databind.JsonNode;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.types.EthAddress;

/**
 * A deployed Noto token. Each operation submits one transaction and returns its {@link
 * SentTransaction} handle; wait for the outcome with {@link SentTransaction#waitForReceipt()}:
 *
 * <pre>{@code
 * TransactionReceipt receipt =
 *     token
 *         .transfer("alice", new NotoTransferParams("bob@node2", HexUint256.of(10), null))
 *         .waitForReceipt()
 *         .join();
 * }</pre>
 *
 * <p>Private operations are submitted through the Noto domain; {@link #spendLock} and {@link
 * #cancelLock} are public transactions against the base-ledger contract. The read-only queries
 * {@link #name}, {@link #symbol}, {@link #decimals} and {@link #balanceOf} are calls that submit
 * nothing.
 *
 * <p>Instances are immutable and thread-safe.
 */
public final class NotoInstance {

  /** The name the Noto domain is registered under by default. */
  public static final String DEFAULT_DOMAIN = "noto";

  private final PaladinClient paladin;
  private final String domain;
  private final EthAddress address;

  /**
   * Binds to a token deployed through the {@value #DEFAULT_DOMAIN} domain.
   *
   * @param paladin the client used to submit transactions; must not be {@code null}
   * @param address the address of the token contract; must not be {@code null}
   */
  public NotoInstance(final PaladinClient paladin, final EthAddress address) {
    this(paladin, DEFAULT_DOMAIN, address);
  }

  /**
   * Binds to a token deployed through the named domain.
   *
   * @param paladin the client used to submit transactions; must not be {@code null}
   * @param domain the name the Noto domain is registered under; must not be {@code null}
   * @param address the address of the token contract; must not be {@code null}
   */
  public NotoInstance(final PaladinClient paladin, final String domain, final EthAddress address) {
    this.paladin = Objects.requireNonNull(paladin, "paladin");
    this.domain = Objects.requireNonNull(domain, "domain");
    this.address = Objects.requireNonNull(address, "address");
  }

  /**
   * The address of the token contract.
   *
   * @return the contract address
   */
  public EthAddress address() {
    return address;
  }

  /**
   * The name of the Noto domain the token was deployed through.
   *
   * @return the domain name
   */
  public String domain() {
    return domain;
  }

  /**
   * Returns a handle on the same token that submits through another client, for example a different
   * node.
   *
   * @param paladin the client to submit through
   * @return a new instance
   */
  public NotoInstance using(final PaladinClient paladin) {
    return new NotoInstance(paladin, domain, address);
  }

  /**
   * Mints new tokens to a recipient.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction mint(final String from, final NotoMintParams params) {
    return mint(from, params, null);
  }

  /**
   * Mints new tokens to a recipient. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction mint(
      final String from, final NotoMintParams params, final String idempotencyKey) {
    return sendPrivate("mint", from, params, idempotencyKey);
  }

  /**
   * Transfers the sender's tokens to a recipient.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction transfer(final String from, final NotoTransferParams params) {
    return transfer(from, params, null);
  }

  /**
   * Transfers the sender's tokens to a recipient. The idempotency key prevents a retried submission
   * from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction transfer(
      final String from, final NotoTransferParams params, final String idempotencyKey) {
    return sendPrivate("transfer", from, params, idempotencyKey);
  }

  /**
   * Transfers another identity's tokens to a recipient.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction transferFrom(final String from, final NotoTransferFromParams params) {
    return transferFrom(from, params, null);
  }

  /**
   * Transfers another identity's tokens to a recipient. The idempotency key prevents a retried
   * submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction transferFrom(
      final String from, final NotoTransferFromParams params, final String idempotencyKey) {
    return sendPrivate("transferFrom", from, params, idempotencyKey);
  }

  /**
   * Burns the sender's tokens.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction burn(final String from, final NotoBurnParams params) {
    return burn(from, params, null);
  }

  /**
   * Burns the sender's tokens. The idempotency key prevents a retried submission from executing
   * twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction burn(
      final String from, final NotoBurnParams params, final String idempotencyKey) {
    return sendPrivate("burn", from, params, idempotencyKey);
  }

  /**
   * Burns another identity's tokens.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction burnFrom(final String from, final NotoBurnFromParams params) {
    return burnFrom(from, params, null);
  }

  /**
   * Burns another identity's tokens. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction burnFrom(
      final String from, final NotoBurnFromParams params, final String idempotencyKey) {
    return sendPrivate("burnFrom", from, params, idempotencyKey);
  }

  /**
   * Locks some of the sender's tokens.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   * @deprecated use {@link #createLock(String, NotoLockParams)} instead
   */
  @Deprecated
  public SentTransaction lock(final String from, final NotoLockParams params) {
    return lock(from, params, null);
  }

  /**
   * Locks some of the sender's tokens. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   * @deprecated use {@link #createLock(String, NotoLockParams, String)} instead
   */
  @Deprecated
  public SentTransaction lock(
      final String from, final NotoLockParams params, final String idempotencyKey) {
    return sendPrivate("lock", from, params, idempotencyKey);
  }

  /**
   * Locks some of the sender's tokens.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction createLock(final String from, final NotoLockParams params) {
    return createLock(from, params, null);
  }

  /**
   * Locks some of the sender's tokens. The idempotency key prevents a retried submission from
   * executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction createLock(
      final String from, final NotoLockParams params, final String idempotencyKey) {
    return sendPrivate("createLock", from, params, idempotencyKey);
  }

  /**
   * Locks tokens and fixes the transfer that will spend the lock.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction createTransferLock(
      final String from, final NotoCreateTransferLockParams params) {
    return createTransferLock(from, params, null);
  }

  /**
   * Locks tokens and fixes the transfer that will spend the lock. The idempotency key prevents a
   * retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction createTransferLock(
      final String from, final NotoCreateTransferLockParams params, final String idempotencyKey) {
    return sendPrivate("createTransferLock", from, params, idempotencyKey);
  }

  /**
   * Creates a lock whose spend mints new tokens.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction createMintLock(final String from, final NotoCreateMintLockParams params) {
    return createMintLock(from, params, null);
  }

  /**
   * Creates a lock whose spend mints new tokens. The idempotency key prevents a retried submission
   * from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction createMintLock(
      final String from, final NotoCreateMintLockParams params, final String idempotencyKey) {
    return sendPrivate("createMintLock", from, params, idempotencyKey);
  }

  /**
   * Locks tokens and fixes the burn that will spend the lock.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction createBurnLock(final String from, final NotoCreateBurnLockParams params) {
    return createBurnLock(from, params, null);
  }

  /**
   * Locks tokens and fixes the burn that will spend the lock. The idempotency key prevents a
   * retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction createBurnLock(
      final String from, final NotoCreateBurnLockParams params, final String idempotencyKey) {
    return sendPrivate("createBurnLock", from, params, idempotencyKey);
  }

  /**
   * Releases locked tokens to the given recipients.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction unlock(final String from, final NotoUnlockParams params) {
    return unlock(from, params, null);
  }

  /**
   * Releases locked tokens to the given recipients. The idempotency key prevents a retried
   * submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction unlock(
      final String from, final NotoUnlockParams params, final String idempotencyKey) {
    return sendPrivate("unlock", from, params, idempotencyKey);
  }

  /**
   * Prepares a transfer that spends a lock, for later execution.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareUnlock(final String from, final NotoPrepareUnlockParams params) {
    return prepareUnlock(from, params, null);
  }

  /**
   * Prepares a transfer that spends a lock, for later execution. The idempotency key prevents a
   * retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareUnlock(
      final String from, final NotoPrepareUnlockParams params, final String idempotencyKey) {
    return sendPrivate("prepareUnlock", from, params, idempotencyKey);
  }

  /**
   * Prepares a mint that spends a lock, for later execution.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareMintUnlock(
      final String from, final NotoPrepareMintUnlockParams params) {
    return prepareMintUnlock(from, params, null);
  }

  /**
   * Prepares a mint that spends a lock, for later execution. The idempotency key prevents a retried
   * submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareMintUnlock(
      final String from, final NotoPrepareMintUnlockParams params, final String idempotencyKey) {
    return sendPrivate("prepareMintUnlock", from, params, idempotencyKey);
  }

  /**
   * Prepares a burn that spends a lock, for later execution.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareBurnUnlock(
      final String from, final NotoPrepareBurnUnlockParams params) {
    return prepareBurnUnlock(from, params, null);
  }

  /**
   * Prepares a burn that spends a lock, for later execution. The idempotency key prevents a retried
   * submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction prepareBurnUnlock(
      final String from, final NotoPrepareBurnUnlockParams params, final String idempotencyKey) {
    return sendPrivate("prepareBurnUnlock", from, params, idempotencyKey);
  }

  /**
   * Delegates control of a lock to another address.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction delegateLock(final String from, final NotoDelegateLockParams params) {
    return delegateLock(from, params, null);
  }

  /**
   * Delegates control of a lock to another address. The idempotency key prevents a retried
   * submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction delegateLock(
      final String from, final NotoDelegateLockParams params, final String idempotencyKey) {
    return sendPrivate("delegateLock", from, params, idempotencyKey);
  }

  /**
   * Spends a prepared lock with a public transaction on the base ledger.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction spendLock(final String from, final NotoSpendLockParams params) {
    return spendLock(from, params, null);
  }

  /**
   * Spends a prepared lock with a public transaction on the base ledger. The idempotency key
   * prevents a retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction spendLock(
      final String from, final NotoSpendLockParams params, final String idempotencyKey) {
    return sendPublic("spendLock", from, params, idempotencyKey);
  }

  /**
   * Cancels a prepared lock with a public transaction on the base ledger.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @return a handle on the submitted transaction
   */
  public SentTransaction cancelLock(final String from, final NotoCancelLockParams params) {
    return cancelLock(from, params, null);
  }

  /**
   * Cancels a prepared lock with a public transaction on the base ledger. The idempotency key
   * prevents a retried submission from executing twice.
   *
   * @param from lookup string of the identity that signs the transaction
   * @param params the operation inputs
   * @param idempotencyKey a key unique to this operation, or {@code null}
   * @return a handle on the submitted transaction
   */
  public SentTransaction cancelLock(
      final String from, final NotoCancelLockParams params, final String idempotencyKey) {
    return sendPublic("cancelLock", from, params, idempotencyKey);
  }

  /**
   * Reads the token name.
   *
   * @param from lookup string of the identity making the call
   * @return a future completing with the token name
   */
  public CompletableFuture<String> name(final String from) {
    return call("name", from, Map.of()).thenApply(JsonNode::asText);
  }

  /**
   * Reads the token symbol.
   *
   * @param from lookup string of the identity making the call
   * @return a future completing with the token symbol
   */
  public CompletableFuture<String> symbol(final String from) {
    return call("symbol", from, Map.of()).thenApply(JsonNode::asText);
  }

  /**
   * Reads the number of decimal places the token amounts use.
   *
   * @param from lookup string of the identity making the call
   * @return a future completing with the token decimals
   */
  public CompletableFuture<Integer> decimals(final String from) {
    return call("decimals", from, Map.of()).thenApply(JsonNode::asInt);
  }

  /**
   * Reads the balance of an account, as visible to the calling identity.
   *
   * @param from lookup string of the identity making the call
   * @param account lookup string of the account to read
   * @return a future completing with the balance
   */
  public CompletableFuture<NotoBalanceOfResult> balanceOf(final String from, final String account) {
    return call("balanceOf", from, Map.of("account", Objects.requireNonNull(account, "account")))
        .thenApply(
            result -> PaladinObjectMapper.shared().convertValue(result, NotoBalanceOfResult.class));
  }

  private SentTransaction sendPrivate(
      final String function, final String from, final Object params, final String idempotencyKey) {
    return invoke(function, from, params)
        .privateTx()
        .domain(domain)
        .abi(NotoAbi.PRIVATE)
        .idempotencyKey(idempotencyKey)
        .send();
  }

  private SentTransaction sendPublic(
      final String function, final String from, final Object params, final String idempotencyKey) {
    return invoke(function, from, params)
        .publicTx()
        .abi(NotoAbi.PUBLIC)
        .idempotencyKey(idempotencyKey)
        .send();
  }

  private CompletableFuture<JsonNode> call(
      final String function, final String from, final Object params) {
    return invoke(function, from, params).privateTx().domain(domain).abi(NotoAbi.PRIVATE).call();
  }

  private TxBuilder invoke(final String function, final String from, final Object params) {
    return paladin
        .newTx()
        .function(function)
        .to(address)
        .from(from)
        .inputs(Objects.requireNonNull(params, "params"));
  }
}
