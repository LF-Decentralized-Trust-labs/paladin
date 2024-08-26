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

package io.kaleido.paladin.loader;

import github.com.kaleido_io.paladin.toolkit.PluginControllerGrpc;
import github.com.kaleido_io.paladin.toolkit.Service;
import github.com.kaleido_io.paladin.toolkit.Service.PluginLoad;
import io.grpc.ConnectivityState;
import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioDomainSocketChannel;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.net.UnixDomainSocketAddress;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

public class PluginLoader implements StreamObserver<PluginLoad> {

    private static final Logger LOGGER = LogManager.getLogger(PluginLoader.class);

    static final long INITIAL_CONNECT_DELAY_MS = 250;

    static final long MAX_RECONNECT_DELAY_MS = 15000;

    static final double CONNECT_DELAY_FACTOR = 2.0;

    private final String grpcTarget;

    private final UUID instanceUUID;

    private ManagedChannel channel;

    private PluginControllerGrpc.PluginControllerStub stub;

    private boolean shuttingDown;

    private final Map<UUID, Plugin> plugins = new HashMap<>();

    private int reconnectCount = 0;

    private CompletableFuture<Void> reconnect;

    public PluginLoader(String grpcTarget, UUID instanceUUID) {
        this.grpcTarget = grpcTarget;
        this.instanceUUID = instanceUUID;
        scheduleConnect();
    }

    public synchronized void shutdown() {
        shuttingDown = true;
        channel.shutdownNow();
        for (UUID instanceUUID: plugins.keySet()) {
            PluginInfo info = plugins.get(instanceUUID).info;
            LOGGER.info("stopping {} {} [{}]", info.pluginType(), info.name(), instanceUUID);
            plugins.get(instanceUUID).stop();
        }
        while (!plugins.isEmpty()) {
            try {
                this.wait();
            } catch (InterruptedException e) {
                LOGGER.warn("interrupted during shutdown");
                return;
            }
        }
    }

    private synchronized void connect() {
        LOGGER.info("Plugin loader connecting to {}", grpcTarget);
        if (reconnect != null) {
            reconnect.cancel(false);
            reconnect = null;
        }
        if (channel == null || channel.isShutdown()) {
            NettyChannelBuilder channelBuilder;
            if (grpcTarget.startsWith("unix:")) {
                String socketFile = grpcTarget.replaceFirst("unix:", "");
                channelBuilder = NettyChannelBuilder.forAddress(UnixDomainSocketAddress.of(socketFile));
            } else {
                channelBuilder = NettyChannelBuilder.forTarget(grpcTarget);
            }
            channel = channelBuilder
                    .eventLoopGroup(new NioEventLoopGroup())
                    .channelType(NioDomainSocketChannel.class)
                    .usePlaintext()
                    .build();
            stub = PluginControllerGrpc.newStub(channel);
        }
        Service.PluginLoaderInit req = Service.PluginLoaderInit.newBuilder().
                setId(instanceUUID.toString()).
                build();
        stub.initLoader(req, this);
    }

    /** Every time our connection state changes on the stream, or the channel, we go through
     *  here to cleanup and schedule a reconnect */
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
        reconnect.completeAsync(() -> { connect(); return null; },
                CompletableFuture.delayedExecutor(delay, TimeUnit.MILLISECONDS));
    }

    @Override
    public void onNext(PluginLoad loadInstruction) {
        UUID instanceUUID = UUID.fromString(loadInstruction.getPlugin().getId());
        PluginInfo info = new PluginInfo(grpcTarget, loadInstruction.getPlugin().getPluginType().toString(),
                loadInstruction.getPlugin().getName(), instanceUUID);
        LOGGER.info("load instruction for {} {} [{}] libType={} location={} class={}",
                info.pluginType(), info.name(), instanceUUID,
                loadInstruction.getLibType(), loadInstruction.getLibLocation(), loadInstruction.getClass_());
        switch (loadInstruction.getLibType()) {
            case JAR -> loadJAR(info, loadInstruction);
            case C_SHARED -> loadJNA(info, loadInstruction);
            default -> {
                LOGGER.error("Unexpected load instruction type {}", loadInstruction.getLibType());
                stub.loadFailed(Service.PluginLoadFailed.newBuilder()
                        .setPlugin(loadInstruction.getPlugin())
                        .setErrorMessage("unknown library type")
                        .build(),
                        new LoggingObserver<>("loadFailed")
                );
            }
        }
    }

    @Override
    public void onError(Throwable t) {
        LOGGER.error("Plugin loader stream error", t);
        scheduleConnect();
    }

    @Override
    public synchronized void onCompleted() {
        LOGGER.info("Plugin loader stream closed");
        scheduleConnect();
    }

    private synchronized void resetReconnectCount() {
        reconnectCount = 0;
    }

    private synchronized void loadJNA(PluginInfo info, PluginLoad loadInstruction) {
        Plugin plugin;
        try {
            plugin = new PluginJNA(grpcTarget, info, loadInstruction.getLibLocation());
        } catch(Throwable t) {
            LOGGER.error(new FormattedMessage("JNA load {} failed", loadInstruction.getLibLocation()), t);
            stub.loadFailed(Service.PluginLoadFailed.newBuilder()
                            .setPlugin(loadInstruction.getPlugin())
                            .setErrorMessage(t.getMessage())
                            .build(),
                    new LoggingObserver<>("loadFailed"));
            return;
        }
        // We've got a success
        resetReconnectCount();
        plugins.put(instanceUUID, plugin);
        runPlugin(instanceUUID, plugin);
    }

    private synchronized void loadJAR(PluginInfo info, PluginLoad loadInstruction) {
//        Plugin plugin;
        try {
            throw new UnsupportedOperationException("TODO: Implement");
        } catch(Throwable t) {
            LOGGER.error(new FormattedMessage("JAR load jar={} class={} failed",
                    loadInstruction.getLibLocation(),
                    loadInstruction.getClass_()
            ), t);
            stub.loadFailed(Service.PluginLoadFailed.newBuilder()
                            .setPlugin(loadInstruction.getPlugin())
                            .setErrorMessage(t.getMessage())
                            .build(),
                    new LoggingObserver<>("loadFailed"));
//            return;
        }
//        // We've got a success
//        resetReconnectCount();
//        plugins.put(instanceUUID, plugin);
//        runPlugin(instanceUUID, plugin);
    }

    private void runPlugin(UUID instanceUUID, Plugin plugin) {
        PluginInfo info = plugin.info;
        LOGGER.info("starting {} {} [{}]", info.pluginType(), info.name(), instanceUUID);
        CompletableFuture.runAsync(plugin).thenRun(() -> pluginStopped(instanceUUID, plugin));
    }

    private synchronized void pluginStopped(UUID instanceUUID, Plugin plugin) {
        plugins.remove(instanceUUID, plugin);
        this.notifyAll();
    }

}
