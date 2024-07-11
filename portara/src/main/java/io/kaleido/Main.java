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
import io.kaleido.transaction.TransactionHandler;

import java.io.File;
import java.io.IOException;

public class Main {
    public static void main(String[] args) throws Exception {
        System.err.println("DYLD_LIBRARY_PATH: " + System.getenv("DYLD_LIBRARY_PATH"));
        File f = File.createTempFile("paladin", ".sock");
        if (!f.delete()) {
            throw new IOException(
                    String.format("Failed to deleted socket placeholder after creation: %s", f.getAbsolutePath()));
        }
        String socketFilename = f.getAbsolutePath();
        new PaladinJNA().start(socketFilename);

        TransactionHandler transactionHandler = new TransactionHandler(socketFilename);
        transactionHandler.start();
        transactionHandler.waitStarted();

        // Add a shutdown hook to wait for a signal to exit
        final Thread mainThread = Thread.currentThread();
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutdown signal received.");
            mainThread.interrupt();
        }));

        // in lieu of a JSONRCP listener, just submit a single transaction to prove
        // things work for now
        transactionHandler.submitTransaction(null);

        try {
            // Keep the main thread alive until it's interrupted
            while (!Thread.interrupted()) {
                Thread.sleep(1000);
            }
        } catch (InterruptedException e) {
            System.out.println("Main thread interrupted, exiting.");
        }
        transactionHandler.stop();
    }
}
