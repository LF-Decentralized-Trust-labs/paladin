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
import io.grpc.ManagedChannel;
import io.grpc.stub.StreamObserver;
import io.kaleido.paladin.toolkit.GRPCTargetConnector;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

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

    private final UUID instanceId;

    private ManagedChannel channel;

    private PluginControllerGrpc.PluginControllerStub stub;

    private boolean shuttingDown;

    private final Map<String, Plugin> plugins = new HashMap<>();

    private int reconnectCount = 0;

    private CompletableFuture<Void> reconnect;

    public PluginLoader(String grpcTarget, UUID instanceId) {
        this.grpcTarget = grpcTarget;
        this.instanceId = instanceId;
        scheduleConnect();
    }

    public synchronized void shutdown() {
        shuttingDown = true;
        if (channel != null) {
            channel.shutdownNow();
        }
        for (String instanceId: plugins.keySet()) {
            Plugin plugin = plugins.get(instanceId);
            // Run the stops in parallel in the background (not holding the mutex)
            CompletableFuture.runAsync(() -> {
                LOGGER.info("stopping {} {} [{}]", plugin.info.pluginType(), plugin.info.name(), instanceId);
                try {
                    plugin.stop();
                } catch (Throwable t) {
                    // Don't block exit if we fail
                    pluginStopped(instanceId, plugin, t);
                }
            });
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

    private synchronized void connectAndInit() {
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
            Service.PluginLoaderInit req = Service.PluginLoaderInit.newBuilder().
                    setId(instanceId.toString()).
                    build();
            stub.initLoader(req, this);
        } catch(Throwable t) {
            LOGGER.error("Connect and init failed", t);
            scheduleConnect();
        }
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
        reconnect.completeAsync(() -> { connectAndInit(); return null; },
                CompletableFuture.delayedExecutor(delay, TimeUnit.MILLISECONDS));
    }

    @Override
    public void onNext(PluginLoad loadInstruction) {
        PluginInfo info = new PluginInfo(grpcTarget, loadInstruction.getPlugin().getPluginType().toString(),
                loadInstruction.getPlugin().getName(), loadInstruction.getPlugin().getId());
        LOGGER.info("load instruction for {} {} [{}] libType={} location={} class={}",
                info.pluginType(), info.name(), instanceId,
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

    private synchronized void loadPlugin(PluginLoad loadInstruction, Plugin plugin) {
        try {
            plugins.put(loadInstruction.getPlugin().getId(), plugin);
            plugin.loadAndStart();
            resetReconnectCount();
        } catch(Throwable t) {
            LOGGER.error("plugin load failed", t);
            stub.loadFailed(Service.PluginLoadFailed.newBuilder()
                            .setPlugin(loadInstruction.getPlugin())
                            .setErrorMessage(t.getMessage())
                            .build(),
                    new LoggingObserver<>("loadFailed"));
            return;
        }
        // We've got a success
        resetReconnectCount();
        plugins.put(loadInstruction.getPlugin().getId(), plugin);
    }

    private synchronized void loadJNA(PluginInfo info, PluginLoad loadInstruction) {
        Plugin plugin = new PluginJNA(grpcTarget, info, this::pluginStopped, loadInstruction.getLibLocation());
        loadPlugin(loadInstruction, plugin);
    }

    private synchronized void loadJAR(PluginInfo info, PluginLoad loadInstruction) {
        Plugin plugin = new PluginJAR(grpcTarget, info, this::pluginStopped, loadInstruction.getLibLocation(), loadInstruction.getClass_());
        loadPlugin(loadInstruction, plugin);
    }


    synchronized void pluginStopped(String instanceId, Plugin plugin, Throwable t) {
        if (t != null) {
            LOGGER.error(new FormattedMessage("exception from plugin {} {} [{}]", plugin.info.pluginType(), plugin.info.name(), instanceId), t);
        }
        plugins.remove(instanceId, plugin);
        this.notifyAll();
    }

}
