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
package org.lfdt.paladin.sdk.client.tx;

import java.time.Duration;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.ptx.PtxClient;
import org.lfdt.paladin.sdk.core.transaction.Transaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/**
 * A handle on a transaction that has been submitted to the node, returned by {@link
 * TxBuilder#send()}.
 *
 * <p>This is the Java counterpart of Go's {@code pldclient.SentTransaction} and TypeScript's {@code
 * TransactionFuture}: submission and waiting are two separate steps in every Paladin SDK, so {@code
 * send()} hands back this handle immediately rather than blocking or waiting for a receipt. Chain
 * {@link #waitForReceipt()} onto it to wait:
 *
 * <pre>{@code
 * CompletableFuture<TransactionReceipt> future =
 *     ptx.newTx()
 *         .publicTx()
 *         .from("alice")
 *         .send()
 *         .waitForReceipt();
 * }</pre>
 *
 * <p>The handle wraps a <em>future</em> transaction id, so no part of {@code send()} blocks; the
 * submission itself is still in flight when this object is created. A submission that failed — a
 * transport error, or a transaction the builder rejected before it ever reached the node — is
 * replayed from every method here, since all of them are derived from that id.
 *
 * <p>Nothing here is consumed by use: {@link #waitForReceipt()} may be called more than once, and
 * mixed freely with {@link #getReceipt()} and {@link #getTransaction()}.
 *
 * <p>Instances are immutable and safe to share across threads.
 */
public final class SentTransaction {

  private final PtxClient ptx;
  private final CompletableFuture<UUID> id;
  private final Duration initialPollingInterval;
  private final Duration pollingInterval;
  private final Duration receiptTimeout;

  /**
   * Wraps an in-flight submission. The polling settings are snapshotted from the builder at send
   * time, so later changes to the builder cannot alter a wait already under way.
   */
  SentTransaction(
      final PtxClient ptx,
      final CompletableFuture<UUID> id,
      final Duration initialPollingInterval,
      final Duration pollingInterval,
      final Duration receiptTimeout) {
    this.ptx = Objects.requireNonNull(ptx, "ptx");
    this.id = Objects.requireNonNull(id, "id");
    this.initialPollingInterval =
        Objects.requireNonNull(initialPollingInterval, "initialPollingInterval");
    this.pollingInterval = Objects.requireNonNull(pollingInterval, "pollingInterval");
    this.receiptTimeout = Objects.requireNonNull(receiptTimeout, "receiptTimeout");
  }

  /**
   * The node-assigned id of the submitted transaction.
   *
   * @return a future completing with the transaction id, or failing with the {@link
   *     PaladinInvalidTransactionException} {@link TxBuilder#build()} would have thrown, or with
   *     the underlying transport failure if the submission itself failed
   */
  public CompletableFuture<UUID> id() {
    return id;
  }

  /**
   * Waits for the transaction's receipt, polling until it lands or the builder's {@link
   * TxBuilder#receiptTimeout(Duration)} elapses.
   *
   * <p>A transaction that reverted still produces a receipt, so this completes <em>normally</em>
   * with a receipt whose {@link TransactionReceipt#success()} is {@code false} and whose {@link
   * TransactionReceipt#failureMessage()} explains why. Inspect the receipt rather than relying on
   * the future failing — matching the TypeScript SDK.
   *
   * <p>Polling is fully asynchronous: it occupies no thread while waiting, the timeout starts once
   * submission returns an id, and it also applies while a receipt RPC is pending. Cancellation or
   * timeout stops further polls; an already issued RPC may still finish.
   *
   * @return a future completing with the transaction's receipt; failing with a {@link
   *     PaladinTimeoutException} if no receipt arrives in time, or with the submission failure or
   *     an underlying transport failure
   */
  public CompletableFuture<TransactionReceipt> waitForReceipt() {
    return waitForReceipt(receiptTimeout);
  }

  /**
   * Waits for the transaction's receipt with an explicit timeout, overriding the builder's {@link
   * TxBuilder#receiptTimeout(Duration)} for this call. Equivalent to Go's {@code
   * SentTransaction.Wait(timeout)}. The polling strategy still comes from the builder.
   *
   * <p>Reverts and asynchrony behave exactly as described on {@link #waitForReceipt()}.
   *
   * @param timeout the total time to wait for a receipt; must be positive
   * @return a future completing with the transaction's receipt; failing with a {@link
   *     PaladinTimeoutException} if no receipt arrives in time, with a {@link
   *     PaladinInvalidTransactionException} if the timeout is {@code null} or non-positive, or the
   *     timeout or polling interval cannot be represented in nanoseconds; or with the submission
   *     failure or an underlying transport failure
   */
  public CompletableFuture<TransactionReceipt> waitForReceipt(final Duration timeout) {
    if (timeout == null || timeout.isNegative() || timeout.isZero()) {
      return CompletableFuture.failedFuture(
          new PaladinInvalidTransactionException(
              "receipt timeout must be positive, got: " + timeout));
    }
    final long timeoutNanos;
    final long initialDelayNanos;
    try {
      timeoutNanos = timeout.toNanos();
      initialDelayNanos = initialPollingInterval.toNanos();
    } catch (final ArithmeticException e) {
      return CompletableFuture.failedFuture(
          new PaladinInvalidTransactionException(
              "receipt timeout or polling interval is too large", e));
    }
    final CompletableFuture<TransactionReceipt> result = new CompletableFuture<>();
    id.whenComplete(
        (txId, submissionError) -> {
          if (result.isDone()) {
            return;
          }
          if (submissionError != null) {
            result.completeExceptionally(submissionError);
            return;
          }
          final CompletableFuture<TransactionReceipt> polling = new CompletableFuture<>();
          polling
              .orTimeout(timeoutNanos, TimeUnit.NANOSECONDS)
              .whenComplete(
                  (receipt, error) -> {
                    if (error instanceof TimeoutException) {
                      result.completeExceptionally(
                          new PaladinTimeoutException(
                              "no receipt for transaction "
                                  + txId
                                  + " after "
                                  + timeout.toMillis()
                                  + "ms"));
                    } else if (error != null) {
                      result.completeExceptionally(error);
                    } else {
                      result.complete(receipt);
                    }
                  });
          result.whenComplete((receipt, error) -> polling.cancel(false));
          pollForReceipt(txId, polling, initialDelayNanos);
        });
    return result;
  }

  /**
   * Fetches the receipt once, without waiting ({@code ptx_getTransactionReceipt}). Equivalent to
   * Go's {@code SentTransaction.GetReceipt()}.
   *
   * @return a future completing with the receipt, or with {@code null} if the transaction has not
   *     completed yet; failing with the submission failure or an underlying transport failure
   */
  public CompletableFuture<TransactionReceipt> getReceipt() {
    return id.thenCompose(ptx::getTransactionReceipt);
  }

  /**
   * Fetches the transaction as the node holds it ({@code ptx_getTransaction}). Equivalent to Go's
   * {@code SentTransaction.GetTransaction()}.
   *
   * @return a future completing with the transaction; failing with the submission failure or an
   *     underlying transport failure
   */
  public CompletableFuture<Transaction> getTransaction() {
    return id.thenCompose(ptx::getTransaction);
  }

  /**
   * Polls for a receipt until one lands or the deadline passes, without holding a thread between
   * attempts.
   */
  private void pollForReceipt(
      final UUID txId, final CompletableFuture<TransactionReceipt> result, final long delayNanos) {
    if (result.isDone()) {
      return;
    }
    final CompletableFuture<TransactionReceipt> request;
    try {
      request = ptx.getTransactionReceipt(txId);
    } catch (final RuntimeException e) {
      result.completeExceptionally(e);
      return;
    }
    request.whenComplete(
        (receipt, error) -> {
          if (result.isDone()) {
            return;
          }
          if (error != null) {
            result.completeExceptionally(error);
          } else if (receipt != null) {
            result.complete(receipt);
          } else {
            final Executor delayed =
                CompletableFuture.delayedExecutor(delayNanos, TimeUnit.NANOSECONDS);
            // Add rather than multiply so a caller's large fixed interval cannot overflow.
            final long nextDelay =
                delayNanos + Math.min(delayNanos, pollingInterval.toNanos() - delayNanos);
            delayed.execute(() -> pollForReceipt(txId, result, nextDelay));
          }
        });
  }
}
