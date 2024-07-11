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

import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.grpc.stub.StreamObserver;
import io.netty.channel.kqueue.KQueueDomainSocketChannel;
import io.netty.channel.kqueue.KQueueEventLoopGroup;
import io.netty.channel.unix.DomainSocketAddress;
import paladin.transaction.PaladinTransactionServiceGrpc;
import paladin.transaction.Transaction;

public class TransactionHandler {

    private KQueueEventLoopGroup kqueue;
    private String socketAddress;
    private ManagedChannel channel;
    StreamObserver<Transaction.TransactionMessage> messageStream;
    final CountDownLatch finishLatch = new CountDownLatch(1);
    // Declare the ConcurrentHashMap
    private ConcurrentHashMap<String, TransactionRequest> inflightRequests;


    public TransactionHandler(String socketAddress) {
        this.socketAddress = socketAddress;
        this.kqueue = new KQueueEventLoopGroup();
        inflightRequests = new ConcurrentHashMap<String, TransactionRequest>();
    }

    public void start() {
        System.out.println("start" + this.socketAddress);

        this.channel = NettyChannelBuilder.forAddress(new DomainSocketAddress(this.socketAddress))
                .eventLoopGroup(this.kqueue)
                .channelType(KQueueDomainSocketChannel.class)
                .usePlaintext()
                .build();

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
                    TransactionRequest request = inflightRequests.get(requestId);
                    request.getResponseHandler().onResponse(transactionMessage.getResponse());
                    if (transactionMessage.getResponse().getType() == Transaction.RESPONSE_TYPE.SUBMIT_TRANSACTION_RESPONSE) {
                        System.err.printf("Transaction submitted %s\n",
                                transactionMessage.getResponse().getSubmitTransactionResponse().getTransactionId());
                    }
                }
                finishLatch.countDown();
            }

            @Override
            public void onError(Throwable e) {
                e.printStackTrace(System.err);
                finishLatch.countDown();
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
        System.out.println("stop");

        boolean ok = this.finishLatch.await(1, TimeUnit.MINUTES);
        if (!ok) {
            throw new RuntimeException("Failed");
        }

        this.messageStream.onCompleted();

        if (this.channel != null) {
            this.channel.shutdown();
            this.channel.awaitTermination(1, TimeUnit.MINUTES);
        }
        if (this.kqueue != null) {
            this.kqueue.shutdownGracefully();
        }
    }

    public void waitStarted() throws Exception {
        boolean started = false;
        while (!started) {
            try {
                Thread.sleep(500);
                PaladinTransactionServiceGrpc.PaladinTransactionServiceBlockingStub blockingStub = PaladinTransactionServiceGrpc
                        .newBlockingStub(channel);
                Transaction.StatusResponse response = blockingStub.status(Transaction.StatusRequest.newBuilder()
                        .build());
                started = response.getOk();
            } catch (Exception e) {
                System.out.printf("not yet started: %s\n", e);
            }
        }
        System.out.println("gRPC server ready");
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