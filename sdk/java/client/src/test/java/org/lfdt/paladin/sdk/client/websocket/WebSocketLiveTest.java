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

import static org.junit.jupiter.api.Assertions.*;

import com.fasterxml.jackson.databind.JsonNode;
import java.io.IOException;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.rpc.HttpRpcClient;
import org.lfdt.paladin.sdk.client.websocket.WebSocketSubscriptionClient.SubscriptionEvent;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageBatch;
import org.lfdt.paladin.sdk.core.transaction.TransactionEventBatch;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceiptBatch;

/** Opt-in verification against the three nodes in the local operator development cluster. */
@EnabledIfEnvironmentVariable(named = "PALADIN_LIVE_TEST", matches = "true")
class WebSocketLiveTest {
  @Test
  @Timeout(180)
  void liveSubscriptionsAndReconnect() throws Exception {
    for (final int httpPort : List.of(31548, 31648, 31748)) {
      verifyNode(httpPort);
    }
  }

  private void verifyNode(final int httpPort) throws Exception {
    final String name = "java_ws_live_" + UUID.randomUUID().toString().replace("-", "");
    final String receiptsName = name + "_receipts";
    final String eventsName = name + "_events";
    final String messagesName = name + "_messages";
    final BlockingQueue<SubscriptionEvent> receipts = new LinkedBlockingQueue<>();
    final BlockingQueue<SubscriptionEvent> events = new LinkedBlockingQueue<>();
    final BlockingQueue<SubscriptionEvent> messages = new LinkedBlockingQueue<>();
    try (HttpRpcClient http =
            new HttpRpcClient(RpcClientConfig.builder("http://127.0.0.1:" + httpPort).build());
        TcpProxy proxy = new TcpProxy(httpPort + 1)) {
      boolean receiptCreated = false;
      boolean eventCreated = false;
      boolean messageCreated = false;
      try {
        rpc(
            http,
            "ptx_createReceiptListener",
            Map.of(
                "name",
                receiptsName,
                "started",
                true,
                "filters",
                Map.of("sequenceAbove", 0),
                "options",
                Map.of("incompleteStateReceiptBehavior", "process")));
        receiptCreated = true;
        final List<Map<String, Object>> abi =
            List.of(
                Map.of(
                    "type",
                    "event",
                    "name",
                    "PaladinRegisterSmartContract_V0",
                    "inputs",
                    List.of(
                        Map.of("name", "txId", "type", "bytes32", "indexed", true),
                        Map.of("name", "instance", "type", "address", "indexed", true),
                        Map.of("name", "config", "type", "bytes", "indexed", false))));
        rpc(
            http,
            "ptx_createBlockchainEventListener",
            Map.of(
                "name",
                eventsName,
                "started",
                true,
                "sources",
                List.of(Map.of("abi", abi)),
                "options",
                Map.of("fromBlock", 0, "batchSize", 1, "batchTimeout", "100ms")));
        eventCreated = true;
        rpc(
            http,
            "pgroup_createMessageListener",
            Map.of(
                "name",
                messagesName,
                "started",
                true,
                "filters",
                Map.of("topic", name),
                "options",
                Map.of()));
        messageCreated = true;
        final WebSocketClientConfig config =
            WebSocketClientConfig.builder("ws://127.0.0.1:" + proxy.port())
                .heartbeatInterval(Duration.ofSeconds(1))
                .reconnectDelay(Duration.ofMillis(200))
                .build();
        try (WebSocketSubscriptionClient ws =
            new WebSocketSubscriptionClient(
                config,
                error ->
                    System.out.println("LIVE " + httpPort + " transport: " + error.getMessage()))) {
          await(ws.connected());
          final var receiptSub = await(ws.subscribeReceipts(receiptsName, receipts::add));
          final SubscriptionEvent first = take(receipts);
          final TransactionReceiptBatch batch = first.resultAs(TransactionReceiptBatch.class);
          assertFalse(batch.receipts().isEmpty());
          System.out.println(
              "LIVE "
                  + httpPort
                  + " receipt batch: "
                  + batch.receipts().size()
                  + " typed receipts");
          await(first.nack());
          final SubscriptionEvent redelivery = take(receipts);
          assertEquals(
              batch.receipts().getFirst().id(),
              redelivery.resultAs(TransactionReceiptBatch.class).receipts().getFirst().id());
          System.out.println("LIVE " + httpPort + " NACK redelivery passed");
          final String oldId = receiptSub.serverId();
          proxy.dropConnections();
          final SubscriptionEvent restored = take(receipts);
          assertNotEquals(oldId, receiptSub.serverId());
          assertSame(receiptSub, restored.subscription());
          assertThrows(Exception.class, () -> await(redelivery.ack()));
          assertEquals(
              batch.receipts().getFirst().id(),
              restored.resultAs(TransactionReceiptBatch.class).receipts().getFirst().id());
          await(restored.ack());
          assertTrue(await(receiptSub.unsubscribe()));
          System.out.println(
              "LIVE "
                  + httpPort
                  + " reconnect, resubscribe, stale ACK rejection, ACK and unsubscribe passed");

          final var eventSub = await(ws.subscribeBlockchainEvents(eventsName, events::add));
          final SubscriptionEvent event = take(events);
          assertFalse(event.resultAs(TransactionEventBatch.class).events().isEmpty());
          await(event.ack());
          assertTrue(await(eventSub.unsubscribe()));
          System.out.println("LIVE " + httpPort + " typed blockchain event delivery passed");

          final var messageSub = await(ws.subscribeMessages(messagesName, messages::add));
          final JsonNode groups = rpc(http, "pgroup_queryGroups", Map.of("limit", 1));
          if (!groups.isEmpty()) {
            final JsonNode group = groups.get(0);
            final JsonNode sentId =
                rpc(
                    http,
                    "pgroup_sendMessage",
                    Map.of(
                        "domain",
                        group.path("domain").asText(),
                        "group",
                        group.path("id").asText(),
                        "topic",
                        name,
                        "data",
                        Map.of("test", "java-sdk-websocket-live")));
            final SubscriptionEvent message = take(messages);
            assertEquals(
                sentId.asText(),
                message
                    .resultAs(PrivacyGroupMessageBatch.class)
                    .messages()
                    .getFirst()
                    .id()
                    .toString());
            await(message.ack());
            System.out.println("LIVE " + httpPort + " typed privacy-group message delivery passed");
          } else {
            System.out.println(
                "LIVE "
                    + httpPort
                    + " privacy-group subscription confirmed; no local group for message delivery");
          }
          assertTrue(await(messageSub.unsubscribe()));
        }
      } finally {
        if (messageCreated) rpc(http, "pgroup_deleteMessageListener", messagesName);
        if (eventCreated) rpc(http, "ptx_deleteBlockchainEventListener", eventsName);
        if (receiptCreated) rpc(http, "ptx_deleteReceiptListener", receiptsName);
        System.out.println("LIVE " + httpPort + " temporary listeners cleaned up");
      }
    }
  }

  private static JsonNode rpc(final HttpRpcClient http, final String method, final Object... params)
      throws Exception {
    return await(http.callRpc(JsonNode.class, method, params));
  }

  private static <T> T await(final CompletableFuture<T> future) throws Exception {
    return future.get(25, TimeUnit.SECONDS);
  }

  private static SubscriptionEvent take(final BlockingQueue<SubscriptionEvent> events)
      throws InterruptedException {
    final SubscriptionEvent event = events.poll(25, TimeUnit.SECONDS);
    assertNotNull(event, "Timed out waiting for live subscription delivery");
    return event;
  }

  /** Forwards real TCP traffic and can disconnect only this test's WebSocket. */
  private static final class TcpProxy implements AutoCloseable {
    private final ServerSocket server;
    private final ExecutorService workers = Executors.newVirtualThreadPerTaskExecutor();
    private final List<Socket> connections = new CopyOnWriteArrayList<>();

    private TcpProxy(final int upstreamPort) throws IOException {
      server = new ServerSocket(0, 10, InetAddress.getLoopbackAddress());
      workers.submit(
          () -> {
            while (!server.isClosed()) {
              try {
                final Socket downstream = server.accept();
                final Socket upstream = new Socket("127.0.0.1", upstreamPort);
                connections.add(downstream);
                connections.add(upstream);
                workers.submit(() -> forward(downstream, upstream));
                workers.submit(() -> forward(upstream, downstream));
              } catch (final IOException e) {
                if (!server.isClosed()) throw new IllegalStateException(e);
              }
            }
          });
    }

    private int port() {
      return server.getLocalPort();
    }

    private void forward(final Socket source, final Socket target) {
      try {
        source.getInputStream().transferTo(target.getOutputStream());
      } catch (final IOException ignored) {
        // Expected when the test deliberately cuts the connection.
      } finally {
        closeSocket(source);
        closeSocket(target);
      }
    }

    private void dropConnections() {
      for (final Socket socket : List.copyOf(connections)) closeSocket(socket);
    }

    private void closeSocket(final Socket socket) {
      connections.remove(socket);
      try {
        socket.close();
      } catch (final IOException ignored) {
      }
    }

    @Override
    public void close() throws IOException {
      server.close();
      dropConnections();
      workers.shutdownNow();
    }
  }
}
