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

import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioDomainSocketChannel;

import java.net.UnixDomainSocketAddress;

public class GRPCTargetConnector {
    public static ManagedChannel connect(String grpcTarget) {
        NettyChannelBuilder channelBuilder;
        if (grpcTarget.startsWith("unix:")) {
            String socketFile = grpcTarget.replaceFirst("unix:", "");
            channelBuilder = NettyChannelBuilder.forAddress(UnixDomainSocketAddress.of(socketFile));
        } else {
            channelBuilder = NettyChannelBuilder.forTarget(grpcTarget);
        }
        return channelBuilder
                .eventLoopGroup(new NioEventLoopGroup())
                .channelType(NioDomainSocketChannel.class)
                .usePlaintext()
                .build();
    }
}
