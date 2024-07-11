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
import java.util.concurrent.TimeUnit;

import io.grpc.ManagedChannel;
import io.grpc.netty.NettyChannelBuilder;
import io.netty.channel.kqueue.KQueueDomainSocketChannel;
import io.netty.channel.kqueue.KQueueEventLoopGroup;
import io.netty.channel.unix.DomainSocketAddress;
import paladin.transaction.PaladinTransactionServiceGrpc;
import paladin.transaction.Transaction;

public class TransactionHandler {

    private KQueueEventLoopGroup kqueue;
    private String socketAddress;

    public TransactionHandler(String socketAddress) {
        this.socketAddress = socketAddress;
        this.kqueue = new KQueueEventLoopGroup();
    }

    public ManagedChannel createChannel() {
        return NettyChannelBuilder.forAddress(new DomainSocketAddress(this.socketAddress))
                .eventLoopGroup(this.kqueue)
                .channelType(KQueueDomainSocketChannel.class)
                .usePlaintext()
                .build();
    }

    public void shutdownChannel(ManagedChannel channel) throws InterruptedException {
        if (channel != null) {
            channel.shutdown();
            channel.awaitTermination(1, TimeUnit.MINUTES);
        }
        if (this.kqueue != null) {
            this.kqueue.shutdownGracefully();
        }
    }

    public void submitTransaction() throws Exception {
        ManagedChannel channel = createChannel();

        PaladinTransactionServiceGrpc.PaladinTransactionServiceBlockingStub blockingStub =
                PaladinTransactionServiceGrpc.newBlockingStub(channel);
        Transaction.SubmitTransactionResponse response = blockingStub.submit(Transaction.SubmitTransactionRequest.newBuilder()
                .setContractAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
                .setFrom("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
                .setIdempotencyKey(UUID.randomUUID().toString())
                .setPayloadJSON("{\"method\":\"foo\",\"params\":[\"bar\",\"quz\"]}")
                .build());

        shutdownChannel(channel);

        System.out.println("Transaction submitted: " + response.getTransactionId());
    }
}