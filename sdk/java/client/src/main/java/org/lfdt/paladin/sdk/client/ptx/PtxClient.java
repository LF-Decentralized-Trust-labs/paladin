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
package org.lfdt.paladin.sdk.client.ptx;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.privacygroup.PrivacyGroupClient;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.abi.ABIDecodedData;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.StoredABI;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListener;
import org.lfdt.paladin.sdk.core.transaction.BlockchainEventListenerStatus;
import org.lfdt.paladin.sdk.core.transaction.ChainedDispatch;
import org.lfdt.paladin.sdk.core.transaction.Dispatch;
import org.lfdt.paladin.sdk.core.transaction.PreparedTransaction;
import org.lfdt.paladin.sdk.core.transaction.PublicTxWithBinding;
import org.lfdt.paladin.sdk.core.transaction.Transaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionCall;
import org.lfdt.paladin.sdk.core.transaction.TransactionFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionInput;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptFull;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptListener;
import org.lfdt.paladin.sdk.core.transaction.TransactionStates;
import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Client for the {@code ptx_*} RPC namespace (private transaction manager).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype. This covers the full HTTP method surface: the transaction lifecycle (send, prepare,
 * update, call, get and query), receipts, state, prepared and public transactions, stored ABIs,
 * decode, verifier resolution, receipt and blockchain-event listeners, and dispatch queries. The
 * WebSocket subscriptions are available through {@link
 * org.lfdt.paladin.sdk.client.websocket.WebSocketSubscriptionClient}.
 */
public final class PtxClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public PtxClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Starts a fluent transaction builder against this client.
   *
   * <p>The ergonomic front door for submitting a transaction: chain the definition, {@link
   * TxBuilder#send() send()} it, then wait on the returned handle.
   *
   * <pre>{@code
   * TransactionReceipt receipt =
   *     ptx.newTx()
   *         .privateTx()
   *         .domain("noto")
   *         .from("alice")
   *         .send()
   *         .waitForReceipt()
   *         .join();
   * }</pre>
   *
   * @return a new builder bound to this client
   */
  public TxBuilder newTx() {
    return TxBuilder.on(this, new PrivacyGroupClient(rpc));
  }

  /**
   * Submits a transaction for execution ({@code ptx_sendTransaction}).
   *
   * @param transaction the transaction to submit
   * @return a future completing with the assigned transaction id
   */
  public CompletableFuture<UUID> sendTransaction(final TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_sendTransaction", transaction);
  }

  /**
   * Submits a batch of transactions for execution ({@code ptx_sendTransactions}).
   *
   * @param transactions the transactions to submit
   * @return a future completing with the assigned transaction ids, in order
   */
  public CompletableFuture<List<UUID>> sendTransactions(final List<TransactionInput> transactions) {
    return rpc.callRpc(new TypeReference<List<UUID>>() {}, "ptx_sendTransactions", transactions);
  }

  /**
   * Prepares a transaction without submitting it ({@code ptx_prepareTransaction}).
   *
   * @param transaction the transaction to prepare
   * @return a future completing with the prepared transaction id
   */
  public CompletableFuture<UUID> prepareTransaction(final TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_prepareTransaction", transaction);
  }

  /**
   * Prepares a batch of transactions without submitting them ({@code ptx_prepareTransactions}).
   *
   * @param transactions the transactions to prepare
   * @return a future completing with the prepared transaction ids, in order
   */
  public CompletableFuture<List<UUID>> prepareTransactions(
      final List<TransactionInput> transactions) {
    return rpc.callRpc(new TypeReference<List<UUID>>() {}, "ptx_prepareTransactions", transactions);
  }

  /**
   * Updates a pending transaction ({@code ptx_updateTransaction}).
   *
   * @param id the id of the transaction to update
   * @param transaction the new transaction content
   * @return a future completing with the transaction id
   */
  public CompletableFuture<UUID> updateTransaction(
      final UUID id, final TransactionInput transaction) {
    return rpc.callRpc(UUID.class, "ptx_updateTransaction", id, transaction);
  }

  /**
   * Executes a read-only call without submitting a transaction ({@code ptx_call}).
   *
   * @param transaction the call to execute
   * @return a future completing with the call result as raw JSON
   */
  public CompletableFuture<JsonNode> call(final TransactionCall transaction) {
    return rpc.callRpc(JsonNode.class, "ptx_call", transaction);
  }

  /**
   * Fetches a transaction by id ({@code ptx_getTransaction}).
   *
   * @param id the transaction id
   * @return a future completing with the transaction, or {@code null} if not found
   */
  public CompletableFuture<Transaction> getTransaction(final UUID id) {
    return rpc.callRpc(Transaction.class, "ptx_getTransaction", id);
  }

  /**
   * Fetches a transaction by id with full detail ({@code ptx_getTransactionFull}).
   *
   * @param id the transaction id
   * @return a future completing with the full transaction, or {@code null} if not found
   */
  public CompletableFuture<TransactionFull> getTransactionFull(final UUID id) {
    return rpc.callRpc(TransactionFull.class, "ptx_getTransactionFull", id);
  }

  /**
   * Fetches a transaction by its idempotency key ({@code ptx_getTransactionByIdempotencyKey}).
   *
   * @param idempotencyKey the idempotency key
   * @return a future completing with the transaction, or {@code null} if not found
   */
  public CompletableFuture<Transaction> getTransactionByIdempotencyKey(
      final String idempotencyKey) {
    return rpc.callRpc(Transaction.class, "ptx_getTransactionByIdempotencyKey", idempotencyKey);
  }

  /**
   * Queries transactions ({@code ptx_queryTransactions}).
   *
   * @param query the query to run
   * @return a future completing with the matching transactions
   */
  public CompletableFuture<List<Transaction>> queryTransactions(final QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<Transaction>>() {}, "ptx_queryTransactions", query);
  }

  /**
   * Queries transactions with full detail ({@code ptx_queryTransactionsFull}).
   *
   * @param query the query to run
   * @return a future completing with the matching full transactions
   */
  public CompletableFuture<List<TransactionFull>> queryTransactionsFull(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionFull>>() {}, "ptx_queryTransactionsFull", query);
  }

  /**
   * Fetches the receipt for a transaction ({@code ptx_getTransactionReceipt}).
   *
   * @param id the transaction id
   * @return a future completing with the receipt, or {@code null} if not yet available
   */
  public CompletableFuture<TransactionReceipt> getTransactionReceipt(final UUID id) {
    return rpc.callRpc(TransactionReceipt.class, "ptx_getTransactionReceipt", id);
  }

  /**
   * Fetches the receipt for a transaction with full detail ({@code ptx_getTransactionReceiptFull}).
   *
   * @param id the transaction id
   * @return a future completing with the full receipt, or {@code null} if not yet available
   */
  public CompletableFuture<TransactionReceiptFull> getTransactionReceiptFull(final UUID id) {
    return rpc.callRpc(TransactionReceiptFull.class, "ptx_getTransactionReceiptFull", id);
  }

  /**
   * Fetches the domain-specific receipt for a transaction ({@code ptx_getDomainReceipt}).
   *
   * @param domain the domain that produced the receipt
   * @param id the transaction id
   * @return a future completing with the domain receipt as raw JSON
   */
  public CompletableFuture<JsonNode> getDomainReceipt(final String domain, final UUID id) {
    return rpc.callRpc(JsonNode.class, "ptx_getDomainReceipt", domain, id);
  }

  /**
   * Fetches the states related to a transaction ({@code ptx_getStateReceipt}).
   *
   * @param id the transaction id
   * @return a future completing with the transaction states
   */
  public CompletableFuture<TransactionStates> getStateReceipt(final UUID id) {
    return rpc.callRpc(TransactionStates.class, "ptx_getStateReceipt", id);
  }

  /**
   * Queries transaction receipts ({@code ptx_queryTransactionReceipts}).
   *
   * @param query the query to run
   * @return a future completing with the matching receipts
   */
  public CompletableFuture<List<TransactionReceipt>> queryTransactionReceipts(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionReceipt>>() {}, "ptx_queryTransactionReceipts", query);
  }

  /**
   * Fetches a prepared transaction by id ({@code ptx_getPreparedTransaction}).
   *
   * @param id the transaction id
   * @return a future completing with the prepared transaction, or {@code null} if not found
   */
  public CompletableFuture<PreparedTransaction> getPreparedTransaction(final UUID id) {
    return rpc.callRpc(PreparedTransaction.class, "ptx_getPreparedTransaction", id);
  }

  /**
   * Queries prepared transactions ({@code ptx_queryPreparedTransactions}).
   *
   * @param query the query to run
   * @return a future completing with the matching prepared transactions
   */
  public CompletableFuture<List<PreparedTransaction>> queryPreparedTransactions(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<PreparedTransaction>>() {}, "ptx_queryPreparedTransactions", query);
  }

  /**
   * Fetches a public transaction by its node-local id ({@code ptx_getPublicTransaction}).
   *
   * @param id the node-local public transaction id
   * @return a future completing with the public transaction and its binding
   */
  public CompletableFuture<PublicTxWithBinding> getPublicTransaction(final long id) {
    return rpc.callRpc(PublicTxWithBinding.class, "ptx_getPublicTransaction", id);
  }

  /**
   * Stores an ABI and returns its hash reference ({@code ptx_storeABI}).
   *
   * @param abi the ABI entries to store
   * @return a future completing with the stored ABI hash
   */
  public CompletableFuture<Bytes32> storeABI(final List<AbiEntry> abi) {
    return rpc.callRpc(Bytes32.class, "ptx_storeABI", abi);
  }

  /**
   * Fetches a stored ABI by its hash reference ({@code ptx_getStoredABI}).
   *
   * @param hashRef the ABI hash reference
   * @return a future completing with the stored ABI, or {@code null} if not found
   */
  public CompletableFuture<StoredABI> getStoredABI(final Bytes32 hashRef) {
    return rpc.callRpc(StoredABI.class, "ptx_getStoredABI", hashRef);
  }

  /**
   * Queries stored ABIs ({@code ptx_queryStoredABIs}).
   *
   * @param query the query to run
   * @return a future completing with the matching stored ABIs
   */
  public CompletableFuture<List<StoredABI>> queryStoredABIs(final QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<StoredABI>>() {}, "ptx_queryStoredABIs", query);
  }

  /**
   * Decodes revert data against the stored ABIs ({@code ptx_decodeError}).
   *
   * @param revertData the revert data to decode
   * @param dataFormat the requested output data format
   * @return a future completing with the decoded data
   */
  public CompletableFuture<ABIDecodedData> decodeError(
      final HexBytes revertData, final String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeError", revertData, dataFormat);
  }

  /**
   * Decodes call data against the stored ABIs ({@code ptx_decodeCall}).
   *
   * @param callData the call data to decode
   * @param dataFormat the requested output data format
   * @return a future completing with the decoded data
   */
  public CompletableFuture<ABIDecodedData> decodeCall(
      final HexBytes callData, final String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeCall", callData, dataFormat);
  }

  /**
   * Decodes an event against the stored ABIs ({@code ptx_decodeEvent}).
   *
   * @param topics the event log topics
   * @param data the event log data
   * @param dataFormat the requested output data format
   * @return a future completing with the decoded data
   */
  public CompletableFuture<ABIDecodedData> decodeEvent(
      final List<Bytes32> topics, final HexBytes data, final String dataFormat) {
    return rpc.callRpc(ABIDecodedData.class, "ptx_decodeEvent", topics, data, dataFormat);
  }

  /**
   * Resolves a verifier for a key ({@code ptx_resolveVerifier}).
   *
   * @param keyIdentifier the key identifier to resolve
   * @param algorithm the signing algorithm
   * @param verifierType the verifier type to produce
   * @return a future completing with the resolved verifier
   */
  public CompletableFuture<String> resolveVerifier(
      final String keyIdentifier, final String algorithm, final String verifierType) {
    return rpc.callRpc(String.class, "ptx_resolveVerifier", keyIdentifier, algorithm, verifierType);
  }

  /**
   * Creates a receipt listener ({@code ptx_createReceiptListener}).
   *
   * @param listener the listener definition
   * @return a future completing with {@code true} if the listener was created
   */
  public CompletableFuture<Boolean> createReceiptListener(
      final TransactionReceiptListener listener) {
    return rpc.callRpc(Boolean.class, "ptx_createReceiptListener", listener);
  }

  /**
   * Queries receipt listeners ({@code ptx_queryReceiptListeners}).
   *
   * @param query the query to run
   * @return a future completing with the matching listeners
   */
  public CompletableFuture<List<TransactionReceiptListener>> queryReceiptListeners(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<TransactionReceiptListener>>() {},
        "ptx_queryReceiptListeners",
        query);
  }

  /**
   * Fetches a receipt listener by name ({@code ptx_getReceiptListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with the listener, or {@code null} if not found
   */
  public CompletableFuture<TransactionReceiptListener> getReceiptListener(
      final String listenerName) {
    return rpc.callRpc(TransactionReceiptListener.class, "ptx_getReceiptListener", listenerName);
  }

  /**
   * Starts a receipt listener ({@code ptx_startReceiptListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was started
   */
  public CompletableFuture<Boolean> startReceiptListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_startReceiptListener", listenerName);
  }

  /**
   * Stops a receipt listener ({@code ptx_stopReceiptListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was stopped
   */
  public CompletableFuture<Boolean> stopReceiptListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_stopReceiptListener", listenerName);
  }

  /**
   * Deletes a receipt listener ({@code ptx_deleteReceiptListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was deleted
   */
  public CompletableFuture<Boolean> deleteReceiptListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_deleteReceiptListener", listenerName);
  }

  /**
   * Creates a blockchain-event listener ({@code ptx_createBlockchainEventListener}).
   *
   * @param listener the listener definition
   * @return a future completing with {@code true} if the listener was created
   */
  public CompletableFuture<Boolean> createBlockchainEventListener(
      final BlockchainEventListener listener) {
    return rpc.callRpc(Boolean.class, "ptx_createBlockchainEventListener", listener);
  }

  /**
   * Queries blockchain-event listeners ({@code ptx_queryBlockchainEventListeners}).
   *
   * @param query the query to run
   * @return a future completing with the matching listeners
   */
  public CompletableFuture<List<BlockchainEventListener>> queryBlockchainEventListeners(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<BlockchainEventListener>>() {},
        "ptx_queryBlockchainEventListeners",
        query);
  }

  /**
   * Fetches a blockchain-event listener by name ({@code ptx_getBlockchainEventListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with the listener, or {@code null} if not found
   */
  public CompletableFuture<BlockchainEventListener> getBlockchainEventListener(
      final String listenerName) {
    return rpc.callRpc(
        BlockchainEventListener.class, "ptx_getBlockchainEventListener", listenerName);
  }

  /**
   * Starts a blockchain-event listener ({@code ptx_startBlockchainEventListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was started
   */
  public CompletableFuture<Boolean> startBlockchainEventListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_startBlockchainEventListener", listenerName);
  }

  /**
   * Stops a blockchain-event listener ({@code ptx_stopBlockchainEventListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was stopped
   */
  public CompletableFuture<Boolean> stopBlockchainEventListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_stopBlockchainEventListener", listenerName);
  }

  /**
   * Deletes a blockchain-event listener ({@code ptx_deleteBlockchainEventListener}).
   *
   * @param listenerName the listener name
   * @return a future completing with {@code true} if the listener was deleted
   */
  public CompletableFuture<Boolean> deleteBlockchainEventListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "ptx_deleteBlockchainEventListener", listenerName);
  }

  /**
   * Fetches the runtime status of a blockchain-event listener ({@code
   * ptx_getBlockchainEventListenerStatus}).
   *
   * @param listenerName the listener name
   * @return a future completing with the listener status
   */
  public CompletableFuture<BlockchainEventListenerStatus> getBlockchainEventListenerStatus(
      final String listenerName) {
    return rpc.callRpc(
        BlockchainEventListenerStatus.class, "ptx_getBlockchainEventListenerStatus", listenerName);
  }

  /**
   * Queries dispatches ({@code ptx_queryDispatches}).
   *
   * @param query the query to run
   * @return a future completing with the matching dispatches
   */
  public CompletableFuture<List<Dispatch>> queryDispatches(final QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<Dispatch>>() {}, "ptx_queryDispatches", query);
  }

  /**
   * Fetches a dispatch by id ({@code ptx_getDispatch}).
   *
   * @param id the dispatch id
   * @return a future completing with the dispatch, or {@code null} if not found
   */
  public CompletableFuture<Dispatch> getDispatch(final String id) {
    return rpc.callRpc(Dispatch.class, "ptx_getDispatch", id);
  }

  /**
   * Queries chained dispatches ({@code ptx_queryChainedDispatches}).
   *
   * @param query the query to run
   * @return a future completing with the matching chained dispatches
   */
  public CompletableFuture<List<ChainedDispatch>> queryChainedDispatches(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<ChainedDispatch>>() {}, "ptx_queryChainedDispatches", query);
  }

  /**
   * Fetches a chained dispatch by id ({@code ptx_getChainedDispatch}).
   *
   * @param id the chained dispatch id
   * @return a future completing with the chained dispatch, or {@code null} if not found
   */
  public CompletableFuture<ChainedDispatch> getChainedDispatch(final String id) {
    return rpc.callRpc(ChainedDispatch.class, "ptx_getChainedDispatch", id);
  }
}
