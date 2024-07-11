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
package io.kaleido.transaction;

import java.util.Properties;
import java.util.UUID;
import java.util.concurrent.*;

import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.netty.channel.Channel;
import io.netty.channel.MultithreadEventLoopGroup;
import io.netty.channel.kqueue.KQueue;
import io.netty.channel.kqueue.KQueueDomainSocketChannel;
import io.netty.channel.kqueue.KQueueEventLoopGroup;
import io.netty.channel.epoll.EpollSocketChannel;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.epoll.Epoll;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioSocketChannel;
import io.netty.channel.unix.DomainSocketAddress;
import paladin.transaction.PaladinTransactionServiceGrpc;
import paladin.transaction.Transaction;

public class TransactionHandler {

    private final MultithreadEventLoopGroup eventLoopGroup;
    private final Class<? extends Channel> channelBuilder;
    private String socketAddress;
    private ManagedChannel channel;
    StreamObserver<Transaction.TransactionMessage> messageStream;
    // Declare the ConcurrentHashMap

    private static final class DrainMonitor {}
    private final DrainMonitor drainMonitor = new DrainMonitor();

    private ConcurrentHashMap<String, TransactionRequest> inflightRequests;


    public TransactionHandler(String socketAddress) {
        this.socketAddress = socketAddress;
        if (KQueue.isAvailable()) {
            this.eventLoopGroup = new KQueueEventLoopGroup();
            this.channelBuilder = KQueueDomainSocketChannel.class;
        } else if (Epoll.isAvailable()) {
            this.eventLoopGroup = new EpollEventLoopGroup();
            this.channelBuilder = EpollSocketChannel.class;
        } else {
            // TODO: Move to loopback TCP/IP in this case
//            this.eventLoopGroup = new NioEventLoopGroup();
//            this.channelBuilder = NioSocketChannel.class;
            throw new RuntimeException(String.format("Platform combination not supported %s/%s", System.getProperty("os.name"), System.getProperty("os.arch")));
        }
        inflightRequests = new ConcurrentHashMap<>();
    }

    public void start() {
        System.out.println("start" + this.socketAddress);

        this.channel = NettyChannelBuilder.forAddress(new DomainSocketAddress(this.socketAddress))
                .eventLoopGroup(this.eventLoopGroup)
                .channelType(this.channelBuilder)
                .usePlaintext()
                .build();

        waitGRPCReady();

        PaladinTransactionServiceGrpc.PaladinTransactionServiceStub asyncStub = PaladinTransactionServiceGrpc
                .newStub(this.channel);

        StreamObserver<Transaction.TransactionMessage> responseObserver = new StreamObserver<>() {

            @Override
            public void onNext(Transaction.TransactionMessage transactionMessage) {
                System.err.printf("Response in Java %s [%s]\n",
                        transactionMessage.getId(),
                        transactionMessage.getType());
                if (transactionMessage.getType() == Transaction.MESSAGE_TYPE.RESPONSE_MESSAGE) {
                    String requestId = transactionMessage.getResponse().getRequestId();
                    TransactionRequest request = inflightRequests.remove(requestId);
                    if (request != null) {
                        request.getResponseHandler().onResponse(transactionMessage.getResponse());
                        if (transactionMessage.getResponse().getType() == Transaction.RESPONSE_TYPE.SUBMIT_TRANSACTION_RESPONSE) {
                            System.err.printf("Transaction submitted %s\n",
                                    transactionMessage.getResponse().getSubmitTransactionResponse().getTransactionId());
                        }
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
        this.messageStream = asyncStub.listen(responseObserver);
        System.out.println("listener stopped: " + this.messageStream.toString());

    }

    public void stop() throws InterruptedException {
        System.out.println("quiescing");

        // Quiesce the server, giving a little time for in-flight requests to run to completion
        synchronized (drainMonitor) {
           if (!inflightRequests.isEmpty()) {
               drainMonitor.wait(5000 /* TODO: Configurable */);
           }
        }
        System.out.println("stopping");

        this.messageStream.onCompleted();

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
                System.out.printf("not yet started: %s\n", e);
            }
        }
        System.out.println("gRPC server ready");
    }

    public Transaction.StatusResponse getStatus() {
        PaladinTransactionServiceGrpc.PaladinTransactionServiceBlockingStub blockingStub = PaladinTransactionServiceGrpc
                .newBlockingStub(channel);
        return blockingStub.status(Transaction.StatusRequest.newBuilder().build());
    }

    public void submitTransaction(TransactionRequest request) throws Exception {
        System.out.println("submitTransaction");

        try {
            String requestId = UUID.randomUUID().toString();
            this.messageStream.onNext(Transaction.TransactionMessage.newBuilder()
                    .setId(requestId)
                    .setType(Transaction.MESSAGE_TYPE.REQUEST_MESSAGE)
                    .setRequest(request.getRequestMessage())
                    .build());
            this.inflightRequests.put(requestId, request);

        } catch (Exception e) {
            e.printStackTrace();
        }
        System.out.println("Transaction submitted ");

    }

}