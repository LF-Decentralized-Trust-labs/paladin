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
package io.kaleido.kata;

import java.util.concurrent.*;
import java.net.UnixDomainSocketAddress;

import io.grpc.Context.CancellableContext;
import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.grpc.Context;
import io.netty.channel.Channel;
import io.netty.channel.MultithreadEventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioDomainSocketChannel;
import github.com.kaleido_io.paladin.kata.Kata;
import github.com.kaleido_io.paladin.kata.KataMessageServiceGrpc;
import github.com.kaleido_io.paladin.kata.Kata.ListenRequest;

public class Handler {

    private final MultithreadEventLoopGroup eventLoopGroup;
    private final Class<? extends Channel> channelBuilder;
    private String socketAddress;
    private ManagedChannel channel;
    private CancellableContext listenerContext;
    private String destinationName; // unique name that can be used by other kata services and plugins to send
                                    // messages to this handler

    public String getDestinationName() {
        return destinationName;
    }

    private static final class DrainMonitor {
    }

    private final DrainMonitor drainMonitor = new DrainMonitor();

    private ConcurrentHashMap<String, Request> inflightRequests;

    public Handler(String socketAddress, String destinationName) {
        this.socketAddress = socketAddress;
        this.eventLoopGroup = new NioEventLoopGroup();
        this.channelBuilder = NioDomainSocketChannel.class;
        inflightRequests = new ConcurrentHashMap<>();
        this.destinationName = destinationName;
    }

    public void start() {
        System.out.println("start" + this.socketAddress);

        this.channel = NettyChannelBuilder.forAddress(UnixDomainSocketAddress.of(this.socketAddress))
                .eventLoopGroup(this.eventLoopGroup)
                .channelType(this.channelBuilder)
                .usePlaintext()
                .build();

        waitGRPCReady();

        KataMessageServiceGrpc.KataMessageServiceStub asyncStub = KataMessageServiceGrpc
                .newStub(this.channel);

        StreamObserver<Kata.Message> responseObserver = new StreamObserver<>() {

            @Override
            public void onNext(Kata.Message transactionMessage) {
                String typeURL = transactionMessage.getBody().getTypeUrl();
                System.out.printf("Response in Java %s [%s]\n",
                        transactionMessage.getId(),
                        typeURL);
                int lastSlashIndex = typeURL.lastIndexOf("/");
                String typeName = typeURL;
                if (lastSlashIndex != -1) {
                    typeName = typeURL.substring(lastSlashIndex + 1);
                }
                if (typeName.equals(
                        "github.com.kaleido_io.paladin.kata.transaction.SubmitTransactionResponse")) {
                    String requestId = transactionMessage.getCorrelationId();
                    Request request = inflightRequests.remove(requestId);
                    if (request != null) {
                        request.getResponseHandler().onResponse(transactionMessage);
                        System.err.printf("Transaction submitted %s\n", transactionMessage.getBody());
                        synchronized (drainMonitor) {
                            if (inflightRequests.isEmpty()) {
                                drainMonitor.notifyAll();
                            }
                        }
                    }
                }
            }

            @Override
            public void onError(Throwable e) {
                e.printStackTrace(System.err);
            }

            @Override
            public void onCompleted() {
                System.err.println("Response stream in Java shut down");
            }
        };

        System.out.println("listening");
        this.listenerContext = Context.current().withCancellation();
        this.listenerContext.run(() -> {
            asyncStub.listen(ListenRequest.newBuilder().setDestination(this.destinationName).build(), responseObserver);
        });

        System.out.println("listener stopped");

    }

    public void stop() throws InterruptedException {
        System.out.println("quiescing");

        // Quiesce the server, giving a little time for in-flight requests to run to
        // completion
        synchronized (drainMonitor) {
            if (!inflightRequests.isEmpty()) {
                drainMonitor.wait(5000 /* TODO: Configurable */);
            }
        }
        System.out.println("stopping");

        if (this.listenerContext != null) {
            this.listenerContext.cancel(null);
        }

        if (this.channel != null) {
            this.channel.shutdown();
            this.channel.awaitTermination(1, TimeUnit.MINUTES);
        }
        if (this.eventLoopGroup != null) {
            this.eventLoopGroup.shutdownGracefully();
        }
    }

    private void waitGRPCReady() {
        boolean started = false;
        while (!started) {
            try {
                Thread.sleep(500);
                started = getStatus().getOk();
            } catch (Exception e) {
                e.printStackTrace(System.err);
                System.out.println("not yet started");
            }
        }
        System.out.println("gRPC server ready");
    }

    public Kata.StatusResponse getStatus() {
        KataMessageServiceGrpc.KataMessageServiceBlockingStub blockingStub = KataMessageServiceGrpc
                .newBlockingStub(channel);
        return blockingStub.status(Kata.StatusRequest.newBuilder().build());
    }

    public void submitTransaction(Request request) throws Exception {
        System.out.println("submitTransaction");

        try {
            String requestId = request.getId();
            KataMessageServiceGrpc.KataMessageServiceBlockingStub blockingStub = KataMessageServiceGrpc
                    .newBlockingStub(channel);
            blockingStub.sendMessage(request.getRequestMessage());
            this.inflightRequests.put(requestId, request);

        } catch (Exception e) {
            e.printStackTrace();
        }
        System.out.println("Transaction submitted ");

    }

}