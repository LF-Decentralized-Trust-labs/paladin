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
package org.lfdt.paladin.sdk.client.websocket;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.http.WebSocket;
import java.nio.ByteBuffer;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.client.exception.PaladinConnectionException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageBatch;
import org.lfdt.paladin.sdk.core.transaction.TransactionEventBatch;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptBatch;

class WebSocketSubscriptionClientTest {

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  @Test
  void subscribesRoutesBatchesAndAcknowledges() throws Exception {
    final FakeConnector connector = new FakeConnector();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), error -> {}, connector::connect)) {
      final FakeSocket socket = connector.next();
      final BlockingQueue<WebSocketSubscriptionClient.SubscriptionEvent> events =
          new LinkedBlockingQueue<>();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> receipts =
          client.subscribeReceipts("receipt-listener", events::add);
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> chain =
          client.subscribeBlockchainEvents("event-listener", events::add);
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> messages =
          client.subscribeMessages("message-listener", events::add);

      final JsonNode receiptRequest = socket.nextRequest();
      final JsonNode chainRequest = socket.nextRequest();
      final JsonNode messageRequest = socket.nextRequest();
      assertEquals("ptx_subscribe", receiptRequest.path("method").asText());
      assertEquals("receipts", receiptRequest.path("params").get(0).asText());
      assertEquals("receipt-listener", receiptRequest.path("params").get(1).asText());
      assertEquals("blockchainevents", chainRequest.path("params").get(0).asText());
      assertEquals("pgroup_subscribe", messageRequest.path("method").asText());
      assertEquals("messages", messageRequest.path("params").get(0).asText());
      socket.respond(receiptRequest, "receipt-id");
      socket.respond(chainRequest, "event-id");
      socket.respond(messageRequest, "message-id");
      assertEquals("receipt-id", receipts.get(2, TimeUnit.SECONDS).serverId());
      assertEquals("event-id", chain.get(2, TimeUnit.SECONDS).serverId());
      assertEquals("message-id", messages.get(2, TimeUnit.SECONDS).serverId());

      socket.notify("ptx_subscription", "receipt-id", "{\"batchId\":1,\"receipts\":[]}");
      socket.notify("ptx_subscription", "event-id", "{\"batchId\":\"abc\",\"events\":[]}");
      socket.notify("pgroup_subscription", "message-id", "{\"batchId\":2,\"messages\":[]}");
      final WebSocketSubscriptionClient.SubscriptionEvent receipt =
          events.poll(2, TimeUnit.SECONDS);
      assertNotNull(receipt);
      assertEquals(receipts.get(), receipt.subscription());
      assertEquals(1, receipt.result().path("batchId").asInt());
      assertEquals(1L, receipt.resultAs(TransactionReceiptBatch.class).batchId());
      receipt.ack().get(2, TimeUnit.SECONDS);
      final WebSocketSubscriptionClient.SubscriptionEvent chainEvent =
          events.poll(2, TimeUnit.SECONDS);
      assertNotNull(chainEvent);
      assertEquals("abc", chainEvent.resultAs(TransactionEventBatch.class).batchId());
      chainEvent.nack().get(2, TimeUnit.SECONDS);
      final WebSocketSubscriptionClient.SubscriptionEvent messageEvent =
          events.poll(2, TimeUnit.SECONDS);
      assertNotNull(messageEvent);
      assertEquals(2L, messageEvent.resultAs(PrivacyGroupMessageBatch.class).batchId());
      messageEvent.ack().get(2, TimeUnit.SECONDS);
      assertEquals("ptx_ack", socket.nextRequest().path("method").asText());
      assertEquals("ptx_nack", socket.nextRequest().path("method").asText());
      assertEquals("pgroup_ack", socket.nextRequest().path("method").asText());

      final CompletableFuture<Boolean> removed = messages.get().unsubscribe();
      final JsonNode unsubscribe = socket.nextRequest();
      assertEquals("pgroup_unsubscribe", unsubscribe.path("method").asText());
      assertEquals("message-id", unsubscribe.path("params").get(0).asText());
      socket.respond(unsubscribe, true);
      assertTrue(removed.get(2, TimeUnit.SECONDS));
      assertFalse(messages.get().unsubscribe().get(2, TimeUnit.SECONDS));
    }
  }

  @Test
  void resubscribesWithStableHandleAndRejectsOldAcknowledgments() throws Exception {
    final FakeConnector connector = new FakeConnector();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), error -> {}, connector::connect)) {
      final FakeSocket first = connector.next();
      final BlockingQueue<WebSocketSubscriptionClient.SubscriptionEvent> events =
          new LinkedBlockingQueue<>();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> pending =
          client.subscribeReceipts("same-name", events::add);
      final JsonNode request = first.nextRequest();
      first.respond(request, "first-id");
      final WebSocketSubscriptionClient.Subscription handle = pending.get(2, TimeUnit.SECONDS);
      final var localId = handle.localId();
      first.notify("ptx_subscription", "first-id", "{\"batchId\":1,\"receipts\":[]}");
      final WebSocketSubscriptionClient.SubscriptionEvent stale = events.poll(2, TimeUnit.SECONDS);
      assertNotNull(stale);
      first.disconnect();
      final FakeSocket second = connector.next();
      assertNull(handle.serverId());
      final JsonNode replacement = second.nextRequest();
      assertEquals("same-name", replacement.path("params").get(1).asText());
      second.respond(replacement, "second-id");
      assertEquals(localId, handle.localId());
      assertEquals("second-id", handle.serverId());
      assertThrows(ExecutionException.class, () -> stale.ack().get(2, TimeUnit.SECONDS));
      second.notify("ptx_subscription", "first-id", "{\"batchId\":2}");
      assertNull(events.poll(50, TimeUnit.MILLISECONDS));
      second.notify("ptx_subscription", "second-id", "{\"batchId\":3}");
      final WebSocketSubscriptionClient.SubscriptionEvent current =
          events.poll(2, TimeUnit.SECONDS);
      assertNotNull(current);
      current.ack().get(2, TimeUnit.SECONDS);
      final JsonNode ack = second.nextRequest();
      assertEquals("second-id", ack.path("params").get(0).asText());
      assertNotEquals("first-id", handle.serverId());
    }
  }

  @Test
  void reportsRpcErrorsAndStopsWhenReconnectDisabled() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final List<Throwable> errors = new ArrayList<>();
    final WebSocketClientConfig settings =
        WebSocketClientConfig.builder("ws://localhost:8548").reconnectDelay(Duration.ZERO).build();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(settings, errors::add, connector::connect)) {
      final FakeSocket socket = connector.next();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> subscription =
          client.subscribeReceipts("bad-listener", ignored -> {});
      final JsonNode request = socket.nextRequest();
      socket.error(request, -32602, "invalid listener");
      assertThrows(ExecutionException.class, () -> subscription.get(2, TimeUnit.SECONDS));
      socket.disconnect();
      assertEquals(1, errors.size());
      assertThrows(
          ExecutionException.class, () -> client.subscribeMessages("next", ignored -> {}).get());
    }
  }

  @Test
  void validatesConfiguration() {
    assertThrows(
        IllegalArgumentException.class, () -> WebSocketClientConfig.builder("http://localhost"));
    assertThrows(
        IllegalArgumentException.class, () -> WebSocketClientConfig.builder("ws:///missing-host"));
    assertThrows(
        IllegalArgumentException.class,
        () -> WebSocketClientConfig.builder("ws://localhost").heartbeatInterval(Duration.ZERO));
    assertThrows(
        IllegalArgumentException.class,
        () ->
            WebSocketClientConfig.builder("ws://localhost").reconnectDelay(Duration.ofSeconds(-1)));
    final WebSocketClientConfig settings =
        WebSocketClientConfig.builder("wss://example.com")
            .connectTimeout(Duration.ofSeconds(1))
            .requestTimeout(Duration.ofSeconds(2))
            .heartbeatInterval(Duration.ofSeconds(3))
            .reconnectDelay(Duration.ofSeconds(4))
            .maxReconnectDelay(Duration.ofSeconds(5))
            .header("Authorization", "Bearer token")
            .build();
    assertEquals("wss", settings.uri().getScheme());
    assertEquals(Duration.ofSeconds(1), settings.connectTimeout());
    assertEquals(Duration.ofSeconds(2), settings.requestTimeout());
    assertEquals(Duration.ofSeconds(3), settings.heartbeatInterval());
    assertEquals(Duration.ofSeconds(4), settings.reconnectDelay());
    assertEquals(Duration.ofSeconds(5), settings.maxReconnectDelay());
    assertEquals("Bearer token", settings.headers().get("Authorization"));
  }

  @Test
  void retriesAnInitialConnectionFailureAndPreservesPendingSubscription() throws Exception {
    final FakeConnector connector = new FakeConnector();
    connector.failFirst = true;
    final List<Throwable> errors = new ArrayList<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), errors::add, connector::connect)) {
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> subscription =
          client.subscribeMessages("after-retry", ignored -> {});
      final FakeSocket socket = connector.next();
      client.connected().get(2, TimeUnit.SECONDS);
      assertEquals(1, errors.size());
      final JsonNode request = socket.nextRequest();
      assertEquals("after-retry", request.path("params").get(1).asText());
      socket.respond(request, "new-id");
      assertEquals("new-id", subscription.get(2, TimeUnit.SECONDS).serverId());
    }
  }

  @Test
  void reportsMalformedFramesAndNegativelyAcknowledgesCallbackFailures() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final List<Throwable> errors = new ArrayList<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), errors::add, connector::connect)) {
      final FakeSocket socket = connector.next();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> subscription =
          client.subscribeReceipts(
              "listener",
              ignored -> {
                throw new IllegalStateException("consumer failed");
              });
      final JsonNode request = socket.nextRequest();
      socket.respond(request, "sub-id");
      subscription.get(2, TimeUnit.SECONDS);
      socket.raw("not-json");
      socket.notify("pgroup_subscription", "sub-id", "{\"batchId\":1}");
      socket.notify("ptx_subscription", "missing", "{\"batchId\":1}");
      socket.notify("ptx_subscription", "sub-id", "{\"batchId\":1}");
      final JsonNode nack = socket.nextRequest();
      assertEquals("ptx_nack", nack.path("method").asText());
      assertEquals("sub-id", nack.path("params").get(0).asText());
      assertEquals(2, errors.size());
    }
  }

  @Test
  void timesOutUnansweredRequestsAndDisconnectsOnMissedPong() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final WebSocketClientConfig settings =
        WebSocketClientConfig.builder("ws://localhost:8548")
            .heartbeatInterval(Duration.ofMillis(100))
            .requestTimeout(Duration.ofMillis(20))
            .reconnectDelay(Duration.ofMillis(10))
            .build();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(settings, error -> {}, connector::connect)) {
      final FakeSocket first = connector.next();
      first.replyToPing = false;
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> unanswered =
          client.subscribeReceipts("unanswered", ignored -> {});
      first.nextRequest();
      assertThrows(ExecutionException.class, () -> unanswered.get(2, TimeUnit.SECONDS));
      final FakeSocket second = connector.next();
      second.replyToPing = true;
      assertNull(second.nextRequestOrNull(50));
    }
  }

  @Test
  void closesPendingRequestsAndRemovesPendingSubscriptions() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), error -> {}, connector::connect);
    final FakeSocket socket = connector.next();
    final CompletableFuture<WebSocketSubscriptionClient.Subscription> pending =
        client.subscribeMessages("pending", ignored -> {});
    final JsonNode request = socket.nextRequest();
    client.close();
    client.close();
    assertThrows(ExecutionException.class, () -> pending.get(2, TimeUnit.SECONDS));
    assertThrows(
        ExecutionException.class,
        () -> client.subscribeReceipts("after-close", ignored -> {}).get());
    socket.respond(request, "late-id");
  }

  @Test
  void subscribingFromConnectionCallbackSendsOneRequest() throws Exception {
    final AtomicReference<WebSocket.Listener> listener = new AtomicReference<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(
            config(),
            error -> {},
            value -> {
              listener.set(value);
              return new CompletableFuture<>();
            })) {
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> subscription =
          client
              .connected()
              .thenCompose(ignored -> client.subscribeReceipts("listener", event -> {}));
      final FakeSocket socket = new FakeSocket(listener.get());
      listener.get().onOpen(socket);
      final JsonNode request = socket.nextRequest();
      socket.respond(request, "subscription-id");
      assertEquals("subscription-id", subscription.get(2, TimeUnit.SECONDS).serverId());
      assertNull(socket.nextRequestOrNull(50));
    }
  }

  @Test
  void dependentStagesCanWaitOnClientCallsFromOtherThreads() throws Exception {
    final AtomicReference<WebSocket.Listener> listener = new AtomicReference<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(
            config(),
            error -> {},
            value -> {
              listener.set(value);
              return new CompletableFuture<>();
            })) {
      final FakeSocket socket = new FakeSocket(listener.get());
      // The stage blocks on another thread that needs the client: this deadlocks if the
      // connection future is completed while the client holds its lock.
      final CompletableFuture<Boolean> crossThreadCall =
          client
              .connected()
              .thenApply(
                  ignored -> {
                    try {
                      CompletableFuture.runAsync(
                              () -> client.subscribeReceipts("from-callback", event -> {}))
                          .get(2, TimeUnit.SECONDS);
                      return true;
                    } catch (final Exception e) {
                      return false;
                    }
                  });
      listener.get().onOpen(socket);
      assertTrue(crossThreadCall.get(5, TimeUnit.SECONDS));
      assertEquals("from-callback", socket.nextRequest().path("params").get(1).asText());
    }
  }

  @Test
  void abortsAConnectionThatOpensAfterClose() throws Exception {
    final AtomicReference<WebSocket.Listener> listener = new AtomicReference<>();
    final WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(
            config(),
            error -> {},
            value -> {
              listener.set(value);
              return new CompletableFuture<>();
            });
    final CompletableFuture<WebSocketSubscriptionClient.Subscription> pending =
        client.subscribeReceipts("pending", ignored -> {});
    client.close();
    final FakeSocket late = new FakeSocket(listener.get());
    listener.get().onOpen(late);
    assertTrue(late.aborted);
    assertNull(late.nextRequestOrNull(50));
    assertThrows(ExecutionException.class, () -> pending.get(2, TimeUnit.SECONDS));
    assertThrows(ExecutionException.class, () -> client.connected().get(2, TimeUnit.SECONDS));
  }

  @Test
  void retriesWhenTheConnectorThrows() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final AtomicBoolean thrown = new AtomicBoolean();
    final List<Throwable> errors = new ArrayList<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(
            config(),
            errors::add,
            value -> {
              if (!thrown.getAndSet(true)) {
                throw new IllegalStateException("connector failed");
              }
              return connector.connect(value);
            })) {
      connector.next();
      client.connected().get(2, TimeUnit.SECONDS);
      assertEquals(1, errors.size());
    }
  }

  @Test
  void answeredHeartbeatsKeepTheConnectionOpen() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final WebSocketClientConfig settings =
        WebSocketClientConfig.builder("ws://localhost:8548")
            .heartbeatInterval(Duration.ofMillis(20))
            .reconnectDelay(Duration.ofMillis(10))
            .build();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(settings, error -> {}, connector::connect)) {
      final FakeSocket socket = connector.next();
      assertTrue(socket.pings.await(2, TimeUnit.SECONDS));
      assertNull(connector.sockets.poll(100, TimeUnit.MILLISECONDS));
      client.connected().get(2, TimeUnit.SECONDS);
    }
  }

  @Test
  void connectsThroughTheHttpClientAndStopsWhenRefused() throws Exception {
    final List<Throwable> errors = new CopyOnWriteArrayList<>();
    final WebSocketClientConfig settings =
        WebSocketClientConfig.builder("ws://127.0.0.1:1")
            .header("Authorization", "Bearer token")
            .connectTimeout(Duration.ofSeconds(2))
            .reconnectDelay(Duration.ZERO)
            .build();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(settings, errors::add)) {
      assertThrows(ExecutionException.class, () -> client.connected().get(10, TimeUnit.SECONDS));
      assertEquals(1, errors.size());
      assertThrows(
          ExecutionException.class,
          () -> client.subscribeReceipts("after-failure", ignored -> {}).get());
    }
    try (WebSocketSubscriptionClient client = new WebSocketSubscriptionClient(settings)) {
      assertThrows(ExecutionException.class, () -> client.connected().get(10, TimeUnit.SECONDS));
    }
  }

  @Test
  void releasesASubscriptionRemovedWhileResubscribing() throws Exception {
    final FakeConnector connector = new FakeConnector();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), error -> {}, connector::connect)) {
      final FakeSocket first = connector.next();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> pending =
          client.subscribeMessages("removed", ignored -> {});
      first.respond(first.nextRequest(), "first-id");
      final WebSocketSubscriptionClient.Subscription handle = pending.get(2, TimeUnit.SECONDS);
      first.disconnect();
      final FakeSocket second = connector.next();
      final JsonNode resubscribe = second.nextRequest();
      assertFalse(handle.unsubscribe().get(2, TimeUnit.SECONDS));
      second.respond(resubscribe, "second-id");
      final JsonNode release = second.nextRequest();
      assertEquals("pgroup_unsubscribe", release.path("method").asText());
      assertEquals("second-id", release.path("params").get(0).asText());
    }
  }

  @Test
  void rejectsInvalidSubscriptionIdsAndReportsFailedResubscriptions() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final List<Throwable> errors = new CopyOnWriteArrayList<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), errors::add, connector::connect)) {
      final FakeSocket first = connector.next();
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> invalid =
          client.subscribeReceipts("blank-id", ignored -> {});
      first.respond(first.nextRequest(), "");
      assertThrows(ExecutionException.class, () -> invalid.get(2, TimeUnit.SECONDS));

      final CompletableFuture<WebSocketSubscriptionClient.Subscription> established =
          client.subscribeReceipts("established", ignored -> {});
      first.respond(first.nextRequest(), "first-id");
      established.get(2, TimeUnit.SECONDS);
      first.disconnect();
      final FakeSocket second = connector.next();
      second.respond(second.nextRequest(), "");
      final FakeSocket third = reconnect(connector, second);
      third.error(third.nextRequest(), -32602, "listener removed");
      // Each disconnect is reported too: disconnect, invalid ID, disconnect, RPC error.
      assertEquals(4, errors.size());
      assertEquals(PaladinConnectionException.class, errors.get(1).getClass());
      assertEquals("invalid subscription ID", errors.get(1).getMessage());
      assertEquals(PaladinRpcException.class, errors.get(3).getClass());
    }
  }

  @Test
  void reconnectsAfterSocketErrorsAndFailedSends() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final List<Throwable> errors = new CopyOnWriteArrayList<>();
    try (WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), errors::add, connector::connect)) {
      final FakeSocket first = connector.next();
      first.failSends = true;
      final CompletableFuture<WebSocketSubscriptionClient.Subscription> retried =
          client.subscribeReceipts("send-fails", ignored -> {});
      // A failed send drops the connection; the subscription is retried on the next one.
      final FakeSocket second = connector.next();
      final JsonNode request = second.nextRequest();
      assertEquals("send-fails", request.path("params").get(1).asText());
      second.respond(request, "retried-id");
      assertEquals("retried-id", retried.get(2, TimeUnit.SECONDS).serverId());
      second.listener.onError(second, new IllegalStateException("socket error"));
      connector.next();
      assertEquals(2, errors.size());
    }
  }

  @Test
  void closesEvenWhenTheCloseFrameFails() throws Exception {
    final FakeConnector connector = new FakeConnector();
    final WebSocketSubscriptionClient client =
        new WebSocketSubscriptionClient(config(), error -> {}, connector::connect);
    final FakeSocket socket = connector.next();
    socket.failClose = true;
    client.close();
    assertThrows(
        ExecutionException.class,
        () -> client.subscribeReceipts("after-close", ignored -> {}).get(2, TimeUnit.SECONDS));
  }

  private static FakeSocket reconnect(final FakeConnector connector, final FakeSocket current)
      throws InterruptedException {
    current.disconnect();
    return connector.next();
  }

  private static WebSocketClientConfig config() {
    return WebSocketClientConfig.builder("ws://localhost:8548")
        .reconnectDelay(Duration.ofMillis(10))
        .heartbeatInterval(Duration.ofSeconds(5))
        .build();
  }

  private static final class FakeConnector {
    private final BlockingQueue<FakeSocket> sockets = new LinkedBlockingQueue<>();
    private final AtomicBoolean first = new AtomicBoolean(true);
    private boolean failFirst;

    private CompletableFuture<WebSocket> connect(final WebSocket.Listener listener) {
      if (failFirst && first.getAndSet(false)) {
        return CompletableFuture.failedFuture(new IllegalStateException("connection refused"));
      }
      final FakeSocket socket = new FakeSocket(listener);
      listener.onOpen(socket);
      sockets.add(socket);
      return CompletableFuture.completedFuture(socket);
    }

    private FakeSocket next() throws InterruptedException {
      final FakeSocket socket = sockets.poll(2, TimeUnit.SECONDS);
      assertNotNull(socket);
      return socket;
    }
  }

  private static final class FakeSocket implements WebSocket {
    private final Listener listener;
    private final BlockingQueue<String> requests = new LinkedBlockingQueue<>();
    private boolean replyToPing = true;
    private volatile boolean failSends;
    private volatile boolean failClose;
    private final CountDownLatch pings = new CountDownLatch(3);
    private volatile boolean aborted;

    private FakeSocket(final Listener listener) {
      this.listener = listener;
    }

    private JsonNode nextRequest() throws Exception {
      final String request = requests.poll(2, TimeUnit.SECONDS);
      assertNotNull(request);
      return MAPPER.readTree(request);
    }

    private JsonNode nextRequestOrNull(final long timeoutMs) throws Exception {
      final String request = requests.poll(timeoutMs, TimeUnit.MILLISECONDS);
      return request == null ? null : MAPPER.readTree(request);
    }

    private void respond(final JsonNode request, final Object result) throws Exception {
      final String response =
          MAPPER.writeValueAsString(
              MAPPER.createObjectNode().put("jsonrpc", "2.0").set("id", request.get("id")));
      final var body = (com.fasterxml.jackson.databind.node.ObjectNode) MAPPER.readTree(response);
      body.set("result", MAPPER.valueToTree(result));
      listener.onText(this, body.toString(), true);
    }

    private void error(final JsonNode request, final int code, final String message) {
      listener.onText(
          this,
          "{\"jsonrpc\":\"2.0\",\"id\":\""
              + request.path("id").asText()
              + "\",\"error\":{\"code\":"
              + code
              + ",\"message\":\""
              + message
              + "\"}}",
          true);
    }

    private void notify(final String method, final String id, final String result) {
      listener.onText(
          this,
          "{\"jsonrpc\":\"2.0\",\"method\":\""
              + method
              + "\",\"params\":{\"subscription\":\""
              + id
              + "\",\"result\":"
              + result
              + "}}",
          true);
    }

    private void disconnect() {
      listener.onClose(this, 1006, "test disconnect");
    }

    private void raw(final String text) {
      listener.onText(this, text.substring(0, 2), false);
      listener.onText(this, text.substring(2), true);
    }

    @Override
    public CompletableFuture<WebSocket> sendText(final CharSequence data, final boolean last) {
      if (failSends) {
        return CompletableFuture.failedFuture(new IllegalStateException("send failed"));
      }
      requests.add(data.toString());
      return CompletableFuture.completedFuture(this);
    }

    @Override
    public CompletableFuture<WebSocket> sendBinary(final ByteBuffer data, final boolean last) {
      return CompletableFuture.completedFuture(this);
    }

    @Override
    public CompletableFuture<WebSocket> sendPing(final ByteBuffer message) {
      if (replyToPing) {
        listener.onPong(this, message);
        pings.countDown();
      }
      return CompletableFuture.completedFuture(this);
    }

    @Override
    public CompletableFuture<WebSocket> sendPong(final ByteBuffer message) {
      return CompletableFuture.completedFuture(this);
    }

    @Override
    public CompletableFuture<WebSocket> sendClose(final int statusCode, final String reason) {
      if (failClose) {
        return CompletableFuture.failedFuture(new IllegalStateException("close failed"));
      }
      return CompletableFuture.completedFuture(this);
    }

    @Override
    public void request(final long n) {}

    @Override
    public String getSubprotocol() {
      return "";
    }

    @Override
    public boolean isOutputClosed() {
      return false;
    }

    @Override
    public boolean isInputClosed() {
      return false;
    }

    @Override
    public void abort() {
      aborted = true;
    }
  }
}
