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

import io.kaleido.transaction.SubmitTransactionRequest;
import io.kaleido.transaction.TransactionHandler;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.io.IOException;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

public class SimpleTXSubmissionTest {

    @Test
    void simpleTXSubmission() throws Exception {
        System.out.println(System.getProperty("os.name"));
        System.out.println(System.getProperty("os.arch"));

        File f = File.createTempFile("paladin", ".sock");
        if (!f.delete() ){
            throw new IOException(String.format("Failed to deleted socket placeholder after creation: %s", f.getAbsolutePath()));
        }
        String socketFilename = f.getAbsolutePath();
        KataJNA kata = new KataJNA();
        kata.start(socketFilename);

        TransactionHandler transactionHandler = new TransactionHandler(socketFilename);
        transactionHandler.start();

        // Add a shutdown hook to wait for a signal to exit
        final Thread mainThread = Thread.currentThread();
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutdown signal received.");
            mainThread.interrupt();
        }));

        // in lieu of a JSONRCP listener, just submit a single transaction to prove things work for now
        CountDownLatch latch = new CountDownLatch(1);
        transactionHandler.submitTransaction(new SubmitTransactionRequest(
                transactionHandler,
                response -> {
                    System.out.println("response received");
                    latch.countDown();
                },
                "contract1",
                "from1",
                "idem1",
                "{}"
        ));

        if (!latch.await(5, TimeUnit.SECONDS)) {
            throw new Exception("timed out waiting for response");
        }
        transactionHandler.stop();
        kata.stop(socketFilename);

        System.out.println("main completed");
    }
}
