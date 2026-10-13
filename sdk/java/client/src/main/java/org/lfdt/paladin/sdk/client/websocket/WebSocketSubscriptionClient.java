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

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.http.HttpClient;
import java.net.http.WebSocket;
import java.nio.ByteBuffer;
import java.time.Duration;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionStage;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Consumer;
import java.util.function.Function;
import org.lfdt.paladin.sdk.client.exception.PaladinConnectionException;
import org.lfdt.paladin.sdk.client.exception.PaladinRpcException;
import org.lfdt.paladin.sdk.client.exception.PaladinTimeoutException;
import org.lfdt.paladin.sdk.client.rpc.JsonRpcRequest;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/**
 * WebSocket JSON-RPC subscriptions for receipts, blockchain events, and privacy-group messages.
 *
 * <p>Subscriptions keep a stable local handle while the server assigns a new subscription ID after
 * each reconnect. The client resubscribes automatically, with exponential backoff, and sends ping
 * frames to detect a lost connection. Each delivered batch must be acknowledged or negatively
 * acknowledged by the consumer, exactly once. The server pauses that subscription until it receives
 * one. If the consumer throws before answering, the client negatively acknowledges the batch.
 *
 * <p>Callbacks run on the WebSocket listener thread. Keep them short and hand longer work to an
 * application executor. This client is thread-safe; close it when finished. Returned futures are
 * completed outside the client's internal lock, so dependent stages may call back into the client.
 */
public final class WebSocketSubscriptionClient implements AutoCloseable {

  private static final String PTX = "ptx";
  private static final String PGROUP = "pgroup";
  private static final Duration CLOSE_FRAME_TIMEOUT = Duration.ofSeconds(1);

  private final WebSocketClientConfig config;
  private final ObjectMapper mapper = PaladinObjectMapper.shared();
  private final HttpClient httpClient;
  private final ScheduledExecutorService scheduler =
      Executors.newSingleThreadScheduledExecutor(
          runnable -> {
            final Thread thread = new Thread(runnable, "paladin-websocket");
            thread.setDaemon(true);
            return thread;
          });
  private final Consumer<Throwable> errorHandler;
  private final Function<WebSocket.Listener, CompletableFuture<WebSocket>> connector;
  private final AtomicLong requestCounter = new AtomicLong();
  private final Map<UUID, Subscription> subscriptions = new LinkedHashMap<>();
  private final Map<String, Subscription> active = new HashMap<>();
  private final Map<String, CompletableFuture<JsonNode>> pending = new HashMap<>();
  private final CompletableFuture<Void> firstConnection = new CompletableFuture<>();

  private WebSocket socket;
  private CompletableFuture<WebSocket> sendTail = CompletableFuture.completedFuture(null);
  private ScheduledFuture<?> reconnectTask;
  private ScheduledFuture<?> heartbeatTask;
  private boolean connecting;
  private boolean closed;
  private boolean terminated;
  private long generation;
  private int reconnectAttempts;
  private long lastPongNanos;

  /**
   * Opens a WebSocket connection immediately.
   *
   * @param config the connection settings
   */
  public WebSocketSubscriptionClient(final WebSocketClientConfig config) {
    this(config, error -> {});
  }

  /**
   * Opens a WebSocket connection immediately, reporting connection and callback failures.
   *
   * @param config the connection settings
   * @param errorHandler receives asynchronous failures
   */
  public WebSocketSubscriptionClient(
      final WebSocketClientConfig config, final Consumer<Throwable> errorHandler) {
    this(config, errorHandler, null);
  }

  WebSocketSubscriptionClient(
      final WebSocketClientConfig config,
      final Consumer<Throwable> errorHandler,
      final Function<WebSocket.Listener, CompletableFuture<WebSocket>> connector) {
    this.config = Objects.requireNonNull(config, "config");
    this.errorHandler = Objects.requireNonNull(errorHandler, "errorHandler");
    httpClient = HttpClient.newBuilder().connectTimeout(config.connectTimeout()).build();
    this.connector = connector == null ? this::openSocket : connector;
    connect();
  }

  private CompletableFuture<WebSocket> openSocket(final WebSocket.Listener listener) {
    final WebSocket.Builder builder =
        httpClient.newWebSocketBuilder().connectTimeout(config.connectTimeout());
    config.headers().forEach(builder::header);
    return builder.buildAsync(config.uri(), listener);
  }

  /**
   * Completes when the first connection opens, or fails if reconnection is disabled.
   *
   * @return the first connection result
   */
  public CompletableFuture<Void> connected() {
    return firstConnection;
  }

  /**
   * Subscribes to transaction receipt batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeReceipts(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PTX, "receipts", listenerName, consumer);
  }

  /**
   * Subscribes to blockchain event batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeBlockchainEvents(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PTX, "blockchainevents", listenerName, consumer);
  }

  /**
   * Subscribes to privacy-group message batches.
   *
   * @param listenerName the server-side listener name
   * @param consumer receives batches that must be acknowledged
   * @return a stable subscription handle
   */
  public CompletableFuture<Subscription> subscribeMessages(
      final String listenerName, final Consumer<SubscriptionEvent> consumer) {
    return subscribe(PGROUP, "messages", listenerName, consumer);
  }

  private CompletableFuture<Subscription> subscribe(
      final String namespace,
      final String type,
      final String listenerName,
      final Consumer<SubscriptionEvent> consumer) {
    if (listenerName == null || listenerName.isBlank()) {
      throw new IllegalArgumentException("listenerName must not be blank");
    }
    final Subscription subscription =
        new Subscription(
            namespace, type, listenerName, Objects.requireNonNull(consumer, "consumer"));
    final long attempt;
    synchronized (this) {
      if (closed || terminated) {
        return CompletableFuture.failedFuture(new IllegalStateException("client is closed"));
      }
      subscriptions.put(subscription.localId, subscription);
      // While disconnected, the next opened() call sends this subscription.
      attempt = socket == null ? -1 : generation;
    }
    if (attempt >= 0) {
      sendSubscribe(attempt, subscription);
    }
    return subscription.ready;
  }

  // Locking rule: the monitor guards connection state only. Futures are completed, the error
  // handler is called, and sockets are opened, sent to, or closed after the monitor is released,
  // so callers' dependent stages can safely call back into this client from any thread.

  private void connect() {
    final long attempt;
    synchronized (this) {
      if (closed || terminated || connecting || socket != null) {
        return;
      }
      connecting = true;
      attempt = ++generation;
    }
    final CompletableFuture<WebSocket> opening;
    try {
      opening = connector.apply(new SocketListener(attempt));
    } catch (final RuntimeException e) {
      disconnected(attempt, e);
      return;
    }
    opening.whenComplete(
        (opened, failure) -> {
          if (failure != null) {
            disconnected(attempt, failure);
          }
        });
  }

  private void opened(final long attempt, final WebSocket openedSocket) {
    final List<Subscription> resubscribe;
    synchronized (this) {
      if (closed || terminated || attempt != generation) {
        resubscribe = null;
      } else {
        socket = openedSocket;
        connecting = false;
        reconnectAttempts = 0;
        sendTail = CompletableFuture.completedFuture(openedSocket);
        lastPongNanos = System.nanoTime();
        if (reconnectTask != null) {
          reconnectTask.cancel(false);
          reconnectTask = null;
        }
        heartbeatTask =
            scheduler.scheduleAtFixedRate(
                () -> heartbeat(attempt),
                config.heartbeatInterval().toMillis(),
                config.heartbeatInterval().toMillis(),
                TimeUnit.MILLISECONDS);
        resubscribe = List.copyOf(subscriptions.values());
      }
    }
    if (resubscribe == null) {
      // A connection from an earlier generation, or one that finished after close().
      openedSocket.abort();
      return;
    }
    for (final Subscription subscription : resubscribe) {
      sendSubscribe(attempt, subscription);
    }
    firstConnection.complete(null);
  }

  private void disconnected(final long attempt, final Throwable cause) {
    final WebSocket oldSocket;
    final List<CompletableFuture<JsonNode>> failedRequests;
    final List<Subscription> failedSubscriptions;
    final boolean terminate;
    synchronized (this) {
      if (closed
          || terminated
          || attempt != generation
          || reconnectTask != null
          || (socket == null && !connecting)) {
        return;
      }
      oldSocket = socket;
      socket = null;
      connecting = false;
      if (heartbeatTask != null) {
        heartbeatTask.cancel(false);
        heartbeatTask = null;
      }
      active.clear();
      for (final Subscription subscription : subscriptions.values()) {
        subscription.serverId = null;
      }
      failedRequests = List.copyOf(pending.values());
      pending.clear();
      terminate = config.reconnectDelay() == null;
      if (terminate) {
        terminated = true;
        failedSubscriptions = List.copyOf(subscriptions.values());
        scheduler.shutdownNow();
      } else {
        failedSubscriptions = List.of();
        final long base = config.reconnectDelay().toMillis();
        final long max = config.maxReconnectDelay().toMillis();
        final long delay = Math.min(max, base * (1L << Math.min(reconnectAttempts++, 30)));
        reconnectTask =
            scheduler.schedule(
                () -> {
                  synchronized (WebSocketSubscriptionClient.this) {
                    reconnectTask = null;
                  }
                  connect();
                },
                delay,
                TimeUnit.MILLISECONDS);
      }
    }
    if (oldSocket != null) {
      oldSocket.abort();
    }
    final PaladinConnectionException error =
        new PaladinConnectionException("WebSocket disconnected", cause);
    for (final CompletableFuture<JsonNode> future : failedRequests) {
      future.completeExceptionally(error);
    }
    if (terminate) {
      firstConnection.completeExceptionally(error);
      for (final Subscription subscription : failedSubscriptions) {
        subscription.ready.completeExceptionally(error);
      }
    }
    report(error);
  }

  private void heartbeat(final long attempt) {
    final WebSocket current;
    final boolean timedOut;
    synchronized (this) {
      if (closed || attempt != generation || socket == null) {
        return;
      }
      current = socket;
      timedOut = System.nanoTime() - lastPongNanos > config.heartbeatInterval().toNanos() * 3 / 2;
    }
    if (timedOut) {
      disconnected(attempt, new PaladinTimeoutException("WebSocket heartbeat timed out", null));
      return;
    }
    current
        .sendPing(ByteBuffer.wrap(new byte[] {1}))
        .exceptionally(
            failure -> {
              disconnected(attempt, failure);
              return null;
            });
  }

  private void sendSubscribe(final long attempt, final Subscription subscription) {
    request(
            attempt,
            subscription.namespace + "_subscribe",
            subscription.type,
            subscription.listenerName)
        .whenComplete((result, failure) -> subscribed(attempt, subscription, result, failure));
  }

  private void subscribed(
      final long attempt,
      final Subscription subscription,
      final JsonNode result,
      final Throwable failure) {
    final String serverId =
        failure == null && result != null && result.isTextual() && !result.asText().isBlank()
            ? result.asText()
            : null;
    final Throwable error =
        failure != null
            ? failure
            : serverId == null
                ? new PaladinConnectionException("invalid subscription ID", null)
                : null;
    final boolean removed;
    boolean firstAttempt = false;
    synchronized (this) {
      if (closed || attempt != generation) {
        return;
      }
      removed = !subscriptions.containsKey(subscription.localId);
      if (!removed && error != null) {
        if (failure != null && socket == null) {
          // The connection dropped; the next opened() call resubscribes.
          return;
        }
        firstAttempt = !subscription.ready.isDone();
        if (firstAttempt) {
          subscriptions.remove(subscription.localId);
        }
      } else if (!removed) {
        subscription.serverId = serverId;
        active.put(serverId, subscription);
      }
    }
    if (removed) {
      // Unsubscribed while the request was in flight: release the server-side subscription.
      if (serverId != null) {
        request(attempt, subscription.namespace + "_unsubscribe", serverId);
      }
    } else if (error == null) {
      subscription.ready.complete(subscription);
    } else if (firstAttempt) {
      subscription.ready.completeExceptionally(error);
    } else {
      report(error);
    }
  }

  private CompletableFuture<JsonNode> request(
      final long attempt, final String method, final Object... params) {
    final String id = nextRequestId();
    final String text;
    try {
      text = mapper.writeValueAsString(new JsonRpcRequest(id, method, List.of(params)));
    } catch (final Exception e) {
      return CompletableFuture.failedFuture(e);
    }
    final CompletableFuture<JsonNode> result = new CompletableFuture<>();
    synchronized (this) {
      if (closed || socket == null || attempt != generation) {
        return CompletableFuture.failedFuture(
            new PaladinConnectionException("WebSocket is disconnected", null));
      }
      pending.put(id, result);
      scheduler.schedule(
          () ->
              failRequest(
                  id, new PaladinTimeoutException("WebSocket RPC " + method + " timed out", null)),
          config.requestTimeout().toMillis(),
          TimeUnit.MILLISECONDS);
    }
    send(attempt, text)
        .whenComplete(
            (ignored, failure) -> {
              if (failure != null) {
                failRequest(id, failure);
              }
            });
    return result;
  }

  private void failRequest(final String id, final Throwable error) {
    final CompletableFuture<JsonNode> response;
    synchronized (this) {
      response = pending.remove(id);
    }
    if (response != null) {
      response.completeExceptionally(error);
    }
  }

  private String nextRequestId() {
    return String.format(Locale.ROOT, "%09d", requestCounter.incrementAndGet());
  }

  private CompletableFuture<Void> send(final long attempt, final String text) {
    final WebSocket current;
    final CompletableFuture<WebSocket> previous;
    final CompletableFuture<WebSocket> next = new CompletableFuture<>();
    synchronized (this) {
      current = socket;
      if (closed || current == null || attempt != generation) {
        return CompletableFuture.failedFuture(
            new PaladinConnectionException("WebSocket is disconnected", null));
      }
      // Reserve this frame's place in the send order; the actual send happens unlocked.
      previous = sendTail;
      sendTail = next;
    }
    previous
        .handle((ignored, failure) -> null)
        .thenCompose(ignored -> current.sendText(text, true))
        .whenComplete(
            (sent, failure) -> {
              if (failure == null) {
                next.complete(sent);
              } else {
                next.completeExceptionally(failure);
                disconnected(attempt, failure);
              }
            });
    return next.thenApply(ignored -> null);
  }

  private void message(final long attempt, final String text) {
    final JsonNode root;
    try {
      root = mapper.readTree(text);
    } catch (final Exception e) {
      report(e);
      return;
    }
    if (root.hasNonNull("method")) {
      notification(attempt, root);
      return;
    }
    final String id = root.path("id").asText();
    final CompletableFuture<JsonNode> response;
    synchronized (this) {
      if (attempt != generation) {
        return;
      }
      response = pending.remove(id);
    }
    if (response == null) {
      return;
    }
    if (root.hasNonNull("error")) {
      final JsonNode error = root.get("error");
      response.completeExceptionally(
          new PaladinRpcException(
              error.path("code").asInt(), error.path("message").asText(), error.get("data"), 0));
    } else {
      response.complete(root.get("result"));
    }
  }

  private void notification(final long attempt, final JsonNode root) {
    final JsonNode params = root.path("params");
    final String serverId = params.path("subscription").asText();
    final Subscription subscription;
    synchronized (this) {
      if (attempt != generation) {
        return;
      }
      subscription = active.get(serverId);
    }
    if (subscription == null
        || !(subscription.namespace + "_subscription").equals(root.path("method").asText())) {
      return;
    }
    final SubscriptionEvent event =
        new SubscriptionEvent(subscription, serverId, attempt, params.get("result"));
    try {
      subscription.consumer.accept(event);
    } catch (final RuntimeException e) {
      report(e);
      // Only nack a batch the consumer has not already answered before throwing.
      event
          .nackUnlessAnswered()
          .exceptionally(
              failure -> {
                report(failure);
                return null;
              });
    }
  }

  private void report(final Throwable error) {
    try {
      errorHandler.accept(error);
    } catch (final RuntimeException ignored) {
      // An application error handler must not stop reconnection or message processing.
    }
  }

  /**
   * Closes the connection and fails every pending subscription and request. A connection attempt
   * that completes after this call is aborted.
   */
  @Override
  public void close() {
    final WebSocket current;
    final List<CompletableFuture<JsonNode>> failedRequests;
    final List<Subscription> removed;
    synchronized (this) {
      if (closed) {
        return;
      }
      closed = true;
      generation++;
      if (reconnectTask != null) {
        reconnectTask.cancel(false);
        reconnectTask = null;
      }
      if (heartbeatTask != null) {
        heartbeatTask.cancel(false);
        heartbeatTask = null;
      }
      current = socket;
      socket = null;
      connecting = false;
      failedRequests = List.copyOf(pending.values());
      pending.clear();
      removed = List.copyOf(subscriptions.values());
      for (final Subscription subscription : removed) {
        subscription.serverId = null;
      }
      subscriptions.clear();
      active.clear();
    }
    scheduler.shutdownNow();
    final IllegalStateException error = new IllegalStateException("client is closed");
    firstConnection.completeExceptionally(error);
    for (final CompletableFuture<JsonNode> future : failedRequests) {
      future.completeExceptionally(error);
    }
    for (final Subscription subscription : removed) {
      subscription.ready.completeExceptionally(error);
    }
    if (current != null) {
      try {
        current
            .sendClose(WebSocket.NORMAL_CLOSURE, "closing")
            .get(CLOSE_FRAME_TIMEOUT.toMillis(), TimeUnit.MILLISECONDS);
      } catch (final InterruptedException e) {
        Thread.currentThread().interrupt();
      } catch (final ExecutionException | TimeoutException e) {
        // Best effort: the server also releases the subscriptions when the connection drops.
      }
    }
    // shutdownNow() aborts in-flight exchanges, including a connection attempt that is still
    // opening, instead of waiting for them as close() would.
    httpClient.shutdownNow();
  }

  /** A stable local subscription handle retained across reconnections. */
  public final class Subscription {

    private final UUID localId = UUID.randomUUID();
    private final String namespace;
    private final String type;
    private final String listenerName;
    private final Consumer<SubscriptionEvent> consumer;
    private final CompletableFuture<Subscription> ready = new CompletableFuture<>();
    private volatile String serverId;

    private Subscription(
        final String namespace,
        final String type,
        final String listenerName,
        final Consumer<SubscriptionEvent> consumer) {
      this.namespace = namespace;
      this.type = type;
      this.listenerName = listenerName;
      this.consumer = consumer;
    }

    /**
     * Returns the local identifier, which stays stable after reconnection.
     *
     * @return the local identifier
     */
    public UUID localId() {
      return localId;
    }

    /**
     * Returns the current server identifier.
     *
     * @return the server identifier, or {@code null} while disconnected
     */
    public String serverId() {
      return serverId;
    }

    /**
     * Stops this subscription. While disconnected, it is removed locally and returns false.
     *
     * @return whether the server reported an active subscription
     */
    public CompletableFuture<Boolean> unsubscribe() {
      final String id;
      final long attempt;
      synchronized (WebSocketSubscriptionClient.this) {
        if (subscriptions.remove(localId) == null) {
          return CompletableFuture.completedFuture(false);
        }
        id = serverId;
        serverId = null;
        if (id != null) {
          active.remove(id);
        }
        attempt = generation;
      }
      ready.completeExceptionally(new IllegalStateException("subscription removed"));
      if (id == null) {
        return CompletableFuture.completedFuture(false);
      }
      return request(attempt, namespace + "_unsubscribe", id).thenApply(JsonNode::asBoolean);
    }
  }

  /** One server batch with the server ID captured when it was delivered. */
  public final class SubscriptionEvent {

    private final Subscription subscription;
    private final String serverId;
    private final long attempt;
    private final JsonNode result;
    private final AtomicBoolean answered = new AtomicBoolean();

    private SubscriptionEvent(
        final Subscription subscription,
        final String serverId,
        final long attempt,
        final JsonNode result) {
      this.subscription = subscription;
      this.serverId = serverId;
      this.attempt = attempt;
      this.result = result;
    }

    /**
     * Returns the subscription that received this batch.
     *
     * @return the stable subscription handle
     */
    public Subscription subscription() {
      return subscription;
    }

    /**
     * Returns the batch payload, including {@code batchId} and its {@code receipts}, {@code
     * events}, or {@code messages} array.
     *
     * @return the batch payload
     */
    public JsonNode result() {
      return result;
    }

    /**
     * Converts the batch payload to a Java type such as {@code TransactionReceiptBatch}, {@code
     * TransactionEventBatch}, or {@code PrivacyGroupMessageBatch}.
     *
     * @param resultType the expected batch type
     * @param <T> the batch type
     * @return the converted batch
     */
    public <T> T resultAs(final Class<T> resultType) {
      return mapper.convertValue(result, resultType);
    }

    /**
     * Acknowledges this batch on its original connection. A batch can be answered only once; a
     * later {@code ack()} or {@code nack()} fails with {@link IllegalStateException}.
     *
     * @return completion of the WebSocket send
     */
    public CompletableFuture<Void> ack() {
      return respond("_ack");
    }

    /**
     * Negatively acknowledges this batch on its original connection. A batch can be answered only
     * once; a later {@code ack()} or {@code nack()} fails with {@link IllegalStateException}.
     *
     * @return completion of the WebSocket send
     */
    public CompletableFuture<Void> nack() {
      return respond("_nack");
    }

    private CompletableFuture<Void> nackUnlessAnswered() {
      if (!answered.compareAndSet(false, true)) {
        return CompletableFuture.completedFuture(null);
      }
      return send("_nack");
    }

    private CompletableFuture<Void> respond(final String suffix) {
      if (!answered.compareAndSet(false, true)) {
        return CompletableFuture.failedFuture(
            new IllegalStateException("batch has already been answered"));
      }
      return send(suffix);
    }

    private CompletableFuture<Void> send(final String suffix) {
      synchronized (WebSocketSubscriptionClient.this) {
        if (active.get(serverId) != subscription || attempt != generation) {
          return CompletableFuture.failedFuture(
              new PaladinConnectionException("subscription connection has changed", null));
        }
      }
      final String text;
      try {
        text =
            mapper.writeValueAsString(
                new JsonRpcRequest(
                    nextRequestId(), subscription.namespace + suffix, List.of(serverId)));
      } catch (final Exception e) {
        return CompletableFuture.failedFuture(e);
      }
      return WebSocketSubscriptionClient.this.send(attempt, text);
    }
  }

  private final class SocketListener implements WebSocket.Listener {

    private final long attempt;
    private final StringBuilder text = new StringBuilder();

    private SocketListener(final long attempt) {
      this.attempt = attempt;
    }

    @Override
    public void onOpen(final WebSocket webSocket) {
      opened(attempt, webSocket);
      webSocket.request(1);
    }

    @Override
    public CompletionStage<?> onText(
        final WebSocket webSocket, final CharSequence data, final boolean last) {
      text.append(data);
      if (last) {
        message(attempt, text.toString());
        text.setLength(0);
      }
      webSocket.request(1);
      return null;
    }

    @Override
    public CompletionStage<?> onPong(final WebSocket webSocket, final ByteBuffer data) {
      synchronized (WebSocketSubscriptionClient.this) {
        if (attempt == generation) {
          lastPongNanos = System.nanoTime();
        }
      }
      webSocket.request(1);
      return null;
    }

    @Override
    public CompletionStage<?> onClose(
        final WebSocket webSocket, final int statusCode, final String reason) {
      disconnected(attempt, new PaladinConnectionException("WebSocket closed: " + reason, null));
      return null;
    }

    @Override
    public void onError(final WebSocket webSocket, final Throwable error) {
      disconnected(attempt, error);
    }
  }
}
