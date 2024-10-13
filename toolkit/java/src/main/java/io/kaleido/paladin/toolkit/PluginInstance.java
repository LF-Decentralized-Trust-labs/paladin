/*
 * Copyright © 2024 Kaleido, Inc.
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

package io.kaleido.paladin.toolkit;

import io.kaleido.paladin.toolkit.PluginControllerGrpc;
import io.kaleido.paladin.toolkit.Service;
import io.grpc.Context;
import io.grpc.ManagedChannel;
import io.grpc.stub.StreamObserver;
import io.netty.util.concurrent.CompleteFuture;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;
import org.apache.logging.log4j.message.Message;

import java.io.IOException;
import java.util.UUID;
import java.util.concurrent.*;

abstract class PluginInstance<MSG> {

    private static final Logger LOGGER = LogManager.getLogger(PluginInstance.class);

    static final long INITIAL_CONNECT_DELAY_MS = 250;

    static final long MAX_RECONNECT_DELAY_MS = 15000;

    static final double CONNECT_DELAY_FACTOR = 2.0;

    private final String grpcTarget;

    private final String pluginId;

    private final InFlight<UUID, MSG> inflightRequests = new InFlight<UUID, MSG>();

    private ManagedChannel channel;

    protected PluginControllerGrpc.PluginControllerStub stub;

    private boolean shuttingDown;

    private int reconnectCount = 0;

    private CompletableFuture<Void> reconnect;

    private StreamObserver<MSG> sendStream;

    public PluginInstance(String grpcTarget, String pluginId) {
        this.grpcTarget = grpcTarget;
        this.pluginId = pluginId;
    }

    /* Must immediately after construction in the child (can't be called during construction otherwise
       in super() call otherwise we cannot guarantee the child constructor will have
       finished initializing variables before we go async */
    protected void init() {
        scheduleConnect();
    }

    abstract StreamObserver<MSG> connect(StreamObserver<MSG> observer);

    abstract CompletableFuture<MSG> handleRequest(MSG msg);

    abstract Service.Header getHeader(MSG msg);

    abstract MSG buildMessage(Service.Header header);

    public final synchronized void shutdown() {
        shuttingDown = true;
        if (channel != null) {
            channel.shutdownNow();
        }
    }

    private synchronized void connectAndRegister() {
        LOGGER.info("Plugin loader connecting to {}", grpcTarget);
        if (reconnect != null) {
            reconnect.cancel(false);
            reconnect = null;
        }
        try {
            if (channel == null || channel.isShutdown()) {
                channel = GRPCTargetConnector.connect(grpcTarget);
                stub = PluginControllerGrpc.newStub(channel);
            }
            this.sendStream = connect(new StreamHandler());
            // Send the register - which will kick the server to send use messages to process
            Service.Header registerHeader = newHeader(Service.Header.MessageType.REGISTER);
            this.sendStream.onNext(buildMessage(registerHeader));
            reconnectCount = 0;
        } catch(Throwable t) {
            LOGGER.error("Connect failed", t);
            scheduleConnect();
        }
    }

    private final Service.Header newHeader(Service.Header.MessageType msgType)  {
        return Service.Header.newBuilder().
                setPluginId(pluginId).
                setMessageId(UUID.randomUUID().toString()).
                setMessageType(msgType).
                build();
    }

    final Service.Header newRequestHeader()  {
        return newHeader(Service.Header.MessageType.REQUEST_FROM_PLUGIN);
    }

    final Service.Header getReplyHeader(MSG req) {
        return Service.Header.newBuilder().
                setPluginId(pluginId).
                setMessageId(UUID.randomUUID().toString()).
                setCorrelationId(getHeader(req).getMessageId()).
                setMessageType(Service.Header.MessageType.RESPONSE_FROM_PLUGIN).
                build();
    }

    final CompletableFuture<MSG> requestReply(MSG requestMessage) {
        CompletableFuture<MSG> inflight =
                inflightRequests.addRequest(UUID.fromString(getHeader(requestMessage).getMessageId()));
        sendStream.onNext(requestMessage);
        return inflight;
    }

    private synchronized void scheduleConnect() {
        if (shuttingDown || reconnect != null) {
            return;
        }
        long delay = INITIAL_CONNECT_DELAY_MS;
        for (int i = 0; i < reconnectCount; i++) {
            delay = (long)((double)(delay) * CONNECT_DELAY_FACTOR);
            if (delay > MAX_RECONNECT_DELAY_MS) {
                delay = MAX_RECONNECT_DELAY_MS;
                break;
            }
        }
        LOGGER.info("Scheduling loader connect in {}ms", delay);
        reconnectCount++;
        reconnect = new CompletableFuture<>();
        reconnect.completeAsync(() -> { connectAndRegister(); return null; },
                CompletableFuture.delayedExecutor(delay, TimeUnit.MILLISECONDS));
    }

    private UUID getCorrelationUUID(Service.Header header) {
        try {
            return UUID.fromString(header.getCorrelationId());
        } catch(IllegalArgumentException e) {
            LOGGER.warn(new FormattedMessage("response with unexpected correlation ID format received {}: {}",header.getCorrelationId()),e);
            return null;
        }
    }

    private Void send(MSG msg) {
        sendStream.onNext(msg);
        return null;
    }

    private Void sendErrorReply(Service.Header reqHeader, Throwable t) {
        Service.Header resHeader = Service.Header.newBuilder().
                setPluginId(pluginId).
                setMessageId(UUID.randomUUID().toString()).
                setCorrelationId(reqHeader.getMessageId()).
                setMessageType(Service.Header.MessageType.ERROR_RESPONSE).
                setErrorMessage(t.getMessage()).
                build();
        LOGGER.error(new FormattedMessage("sending error reply {} to {}", resHeader.getMessageId(), reqHeader.getMessageId()), t);
        sendStream.onNext(buildMessage(resHeader));
        return null;
    }

    private final class StreamHandler implements StreamObserver<MSG> {
        @Override
        public void onNext(MSG msg) {
            // See if this is a request, or a response
            Service.Header header = getHeader(msg);
            switch (header.getMessageType()) {
                case Service.Header.MessageType.RESPONSE_TO_PLUGIN -> {
                    UUID cid = getCorrelationUUID(header);
                    if (cid != null) {
                        LOGGER.debug("Received reply {} to {} type {}", header.getMessageId(), cid, header.getMessageType());
                        inflightRequests.completeRequest(cid, msg);
                    }
                }
                case Service.Header.MessageType.ERROR_RESPONSE -> {
                    UUID cid = getCorrelationUUID(header);
                    if (cid != null) {
                        LOGGER.debug("Received reply {} to {} type {}", header.getMessageId(), cid, header.getMessageType());
                        inflightRequests.failRequest(cid, new Exception(header.getErrorMessage()));
                    }
                }
                case Service.Header.MessageType.REQUEST_TO_PLUGIN -> {
                    // Dispatch for async handling of the request (do not block this thread at all)
                    CompletableFuture.runAsync(() -> handleRequest(msg)
                            .thenApply(PluginInstance.this::send)
                            .exceptionally((t) -> sendErrorReply(getHeader(msg), t))
                    );
                }
                default -> {
                    LOGGER.warn("Received unexpected message {} type {}", header.getMessageId(), header.getMessageType());
                }
            }
        }

        @Override
        public void onError(Throwable t) {
            LOGGER.error("Plugin loader stream error", t);
            scheduleConnect();
        }

        @Override
        public void onCompleted() {
            LOGGER.info("Plugin loader stream closed");
            scheduleConnect();
        }
    }

}
