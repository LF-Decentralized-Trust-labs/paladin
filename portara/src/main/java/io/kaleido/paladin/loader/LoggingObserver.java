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
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioDomainSocketChannel;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.util.UUID;

public class LoggingObserver<T> implements StreamObserver<T> {

    private static final Logger LOGGER = LogManager.getLogger(LoggingObserver.class);

    private final String desc;

    public LoggingObserver(String desc) {
        this.desc = desc;
        LOGGER.info("--> {}", desc);
    }

    @Override
    public void onNext(T value) {
        LOGGER.info("<-- {} {}", desc, value);
    }

    @Override
    public void onError(Throwable t) {
        LOGGER.error(new FormattedMessage("<-- {} ERROR", desc), t);
    }

    @Override
    public void onCompleted() {}
}
