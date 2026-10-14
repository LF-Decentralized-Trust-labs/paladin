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

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertSame;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.core.type.TypeReference;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Supplier;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.lfdt.paladin.sdk.client.exception.PaladinInvalidTransactionException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

@Timeout(15)
class SentTransactionTest {
  private static final UUID ID = UUID.randomUUID();
  private static final TransactionReceipt RECEIPT =
      TransactionReceipt.builder().id(ID).success(true).build();

  @Test
  void oversizedDurationsFailInsteadOfLeavingAnUncompletedWait() {
    final ReceiptRpc rpc = new ReceiptRpc(() -> CompletableFuture.completedFuture(null));
    final Duration oversized = Duration.ofSeconds(Long.MAX_VALUE);
    assertInstanceOf(
        PaladinInvalidTransactionException.class,
        assertThrows(
                ExecutionException.class,
                () -> transaction(rpc).send().waitForReceipt(oversized).get(2, TimeUnit.SECONDS))
            .getCause());
    assertInstanceOf(
        PaladinInvalidTransactionException.class,
        assertThrows(
                ExecutionException.class,
                () ->
                    transaction(rpc)
                        .pollingInterval(oversized)
                        .send()
                        .waitForReceipt()
                        .get(2, TimeUnit.SECONDS))
            .getCause());
    assertEquals(0, rpc.polls.get());
  }

  @Test
  void synchronousRpcFailureIsReturnedToTheWaiter() {
    final IllegalStateException failure = new IllegalStateException("transport closed");
    final ReceiptRpc rpc =
        new ReceiptRpc(
            () -> {
              throw failure;
            });
    assertSame(
        failure,
        assertThrows(
                ExecutionException.class,
                () -> transaction(rpc).send().waitForReceipt().get(2, TimeUnit.SECONDS))
            .getCause());
  }

  @Test
  void stalledReceiptRpcCannotOutliveWaitDeadline() throws Exception {
    final CompletableFuture<TransactionReceipt> pending = new CompletableFuture<>();
    final ReceiptRpc rpc = new ReceiptRpc(() -> pending);
    final CompletableFuture<TransactionReceipt> waiting =
        transaction(rpc).receiptTimeout(Duration.ofMillis(100)).send().waitForReceipt();
    final ExecutionException error =
        assertThrows(ExecutionException.class, () -> waiting.get(2, TimeUnit.SECONDS));
    assertInstanceOf(PaladinTimeoutException.class, error.getCause());
    assertEquals(1, rpc.polls.get());
    // A late null response must not restart the polling loop after the timeout.
    pending.complete(null);
    pause(200);
    assertEquals(1, rpc.polls.get());
  }

  @Test
  void cancellingWaitStopsScheduledPolls() throws Exception {
    final ReceiptRpc rpc = new ReceiptRpc(() -> CompletableFuture.completedFuture(null));
    final SentTransaction sent = transaction(rpc).send();
    final CompletableFuture<TransactionReceipt> waiting = sent.waitForReceipt();
    assertEquals(1, rpc.polls.get());
    assertTrue(waiting.cancel(false));
    pause(200);
    assertEquals(1, rpc.polls.get());
    assertEquals(ID, sent.id().join());
    // Cancellation belongs to this wait, not the reusable transaction handle.
    assertNull(sent.getReceipt().join());
    assertEquals(2, rpc.polls.get());
  }

  @Test
  void cancellingBeforeSubmissionCompletesDoesNotCancelSubmission() {
    final ReceiptRpc rpc = new ReceiptRpc(() -> CompletableFuture.completedFuture(RECEIPT));
    rpc.submission = new CompletableFuture<>();
    final SentTransaction sent = transaction(rpc).send();
    final CompletableFuture<TransactionReceipt> waiting = sent.waitForReceipt();
    waiting.cancel(false);
    assertFalse(rpc.submission.isCancelled());
    rpc.submission.complete(ID);
    assertEquals(0, rpc.polls.get());
    assertSame(RECEIPT, sent.waitForReceipt().join());
  }

  @Test
  void simultaneousWaitsHaveIndependentDeadlines() throws Exception {
    final CompletableFuture<TransactionReceipt> pending = new CompletableFuture<>();
    final ReceiptRpc rpc = new ReceiptRpc(() -> pending);
    final SentTransaction sent = transaction(rpc).send();
    final CompletableFuture<TransactionReceipt> shortWait =
        sent.waitForReceipt(Duration.ofMillis(50));
    final CompletableFuture<TransactionReceipt> longWait =
        sent.waitForReceipt(Duration.ofSeconds(5));
    assertInstanceOf(
        PaladinTimeoutException.class,
        assertThrows(ExecutionException.class, () -> shortWait.get(2, TimeUnit.SECONDS))
            .getCause());
    assertFalse(longWait.isDone());
    pending.complete(RECEIPT);
    assertSame(RECEIPT, longWait.get(2, TimeUnit.SECONDS));
  }

  @Test
  void compareDetectionDelayAndRequestCountForConcurrentWaiters() throws Exception {
    // In-memory RPC deliberately isolates SDK detection delay from node/network execution time.
    for (final int readyMillis : new int[] {150, 2500}) {
      final List<CompletableFuture<Measurement>> fixed = new ArrayList<>();
      final List<CompletableFuture<Measurement>> adaptive = new ArrayList<>();
      final int waiters = 8;
      for (int i = 0; i < waiters; i++) {
        fixed.add(measure(readyMillis, true));
        adaptive.add(measure(readyMillis, false));
      }
      final Measurement before = summarize(fixed);
      final Measurement after = summarize(adaptive);
      System.out.printf(
          "receipt ready=%dms, waiters=%d: fixed detection=%.1fms polls=%d; adaptive detection=%.1fms polls=%d%n",
          readyMillis,
          waiters,
          before.delayMillis / waiters,
          before.polls,
          after.delayMillis / waiters,
          after.polls);
      assertTrue(
          after.delayMillis < before.delayMillis,
          "adaptive polling should detect receipts earlier");
      assertTrue(
          after.polls <= before.polls + 3 * waiters, "backoff bounds the extra queries per waiter");
    }
  }

  private static CompletableFuture<Measurement> measure(
      final int readyMillis, final boolean fixed) {
    final long readyAt = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(readyMillis);
    final ReceiptRpc rpc =
        new ReceiptRpc(
            () -> CompletableFuture.completedFuture(System.nanoTime() >= readyAt ? RECEIPT : null));
    final TxBuilder builder = transaction(rpc);
    if (fixed) {
      builder.pollingInterval(Duration.ofSeconds(1));
    }
    return builder
        .send()
        .waitForReceipt()
        .thenApply(
            receipt ->
                new Measurement((System.nanoTime() - readyAt) / 1_000_000.0, rpc.polls.get()));
  }

  private static Measurement summarize(final List<CompletableFuture<Measurement>> measurements)
      throws Exception {
    double delay = 0;
    int polls = 0;
    for (final CompletableFuture<Measurement> future : measurements) {
      final Measurement measurement = future.get(10, TimeUnit.SECONDS);
      delay += measurement.delayMillis;
      polls += measurement.polls;
    }
    return new Measurement(delay, polls);
  }

  private record Measurement(double delayMillis, int polls) {}

  private static TxBuilder transaction(final RpcClient rpc) {
    return TxBuilder.on(rpc)
        .publicTx()
        .from("alice")
        .to("0x0102030405060708090a0b0c0d0e0f1011121314")
        .function("transfer");
  }

  private static void pause(final long millis) throws Exception {
    CompletableFuture.runAsync(
            () -> {}, CompletableFuture.delayedExecutor(millis, TimeUnit.MILLISECONDS))
        .get(2, TimeUnit.SECONDS);
  }

  private static final class ReceiptRpc implements RpcClient {
    private final AtomicInteger polls = new AtomicInteger();
    private final Supplier<CompletableFuture<TransactionReceipt>> receipt;
    private CompletableFuture<UUID> submission = CompletableFuture.completedFuture(ID);

    private ReceiptRpc(final Supplier<CompletableFuture<TransactionReceipt>> receipt) {
      this.receipt = receipt;
    }

    @Override
    public <T> CompletableFuture<T> callRpc(
        final Class<T> resultType, final String method, final Object... params) {
      if ("ptx_sendTransaction".equals(method)) {
        return submission.thenApply(resultType::cast);
      }
      assertEquals("ptx_getTransactionReceipt", method);
      polls.incrementAndGet();
      return receipt.get().thenApply(resultType::cast);
    }

    @Override
    public <T> CompletableFuture<T> callRpc(
        final TypeReference<T> resultType, final String method, final Object... params) {
      throw new AssertionError("unexpected generic RPC");
    }

    @Override
    public void close() {}
  }
}
