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
package io.kaleido;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.netty.channel.epoll.EpollDomainSocketChannel;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.unix.DomainSocketAddress;
import paladin.contracts.ContractPlugin;
import paladin.contracts.PaladinContractPluginServiceGrpc;

import java.io.File;
import java.io.IOException;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;

public class Main {
    public static void main(String[] args) throws IOException {
        File f = File.createTempFile("paladin", ".sock");
        if (!f.delete() ){
            throw new IOException(String.format("Failed to deleted socket placeholder after creation: %s", f.getAbsolutePath()));
        }
        int rc = new PaladinJNI().run(f.getAbsolutePath());
        if (rc != 0) {
            throw new IOException("Failed to start golang gRPC server");
        }

        doGRPCPingPong(f.getAbsolutePath());
    }

    static void doGRPCPingPong(String socketAddress) {
        ManagedChannel channel = NettyChannelBuilder.forAddress(new DomainSocketAddress(socketAddress))
                .eventLoopGroup(new EpollEventLoopGroup())
                .channelType(EpollDomainSocketChannel.class)
                .usePlaintext()
                .build();

        final CountDownLatch finishLatch = new CountDownLatch(1);
        StreamObserver<ContractPlugin.ContractPluginEvent> responseObserver = new StreamObserver<>() {

            @Override
            public void onNext(ContractPlugin.ContractPluginEvent contractPluginEvent) {
                System.out.printf("Response in Java %s [%s] to %s\n",
                        contractPluginEvent.getId(),
                        contractPluginEvent.getType(),
                        contractPluginEvent.getCorrelationId()
                );
                finishLatch.countDown();
            }

            @Override
            public void onError(Throwable e) {
                e.printStackTrace(System.err);
                finishLatch.countDown();
            }

            @Override
            public void onCompleted() {
                System.out.println("Response stream in Java shut down");
            }
        };

        PaladinContractPluginServiceGrpc.PaladinContractPluginServiceBlockingStub blockingStub =
                PaladinContractPluginServiceGrpc.newBlockingStub(channel);
        PaladinContractPluginServiceGrpc.PaladinContractPluginServiceStub asyncStub =
                PaladinContractPluginServiceGrpc.newStub(channel);
        StreamObserver<ContractPlugin.ContractPluginEvent> requestStream = asyncStub.register(responseObserver);

        requestStream.onNext(ContractPlugin.ContractPluginEvent.newBuilder()
                .setId(UUID.randomUUID().toString())
                .setType("ping")
                .build()
        );

        try {
            finishLatch.wait();
        } catch (InterruptedException e) {
            System.out.println("Rude");
        }

    }
}