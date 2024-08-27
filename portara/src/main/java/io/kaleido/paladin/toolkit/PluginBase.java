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

import github.com.kaleido_io.paladin.toolkit.PluginControllerGrpc;
import github.com.kaleido_io.paladin.toolkit.Service;
import github.com.kaleido_io.paladin.toolkit.Service.PluginLoad;
import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.kaleido.paladin.loader.LoggingObserver;
import io.kaleido.paladin.loader.Plugin;
import io.kaleido.paladin.loader.PluginInfo;
import io.kaleido.paladin.loader.PluginJNA;
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

public abstract class PluginBase {

    private static final Logger LOGGER = LogManager.getLogger(PluginBase.class);

    protected abstract PluginInstance NewPluginInstance(String grpcTarget, String instanceUUID);

}
