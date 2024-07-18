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

public class Main {
    public static void main(String[] args) throws Exception {
        System.out.println("TODO - start all the components");
        System.out.println("DYLD_LIBRARY_PATH: " + System.getenv("DYLD_LIBRARY_PATH"));

        String kataConfigFilePath = System.getenv("KATA_CONFIG_FILE");
        System.out.println("KATA_CONFIG_FILE: " + kataConfigFilePath);

        new KataJNA().start(kataConfigFilePath);

        
        // Add a shutdown hook to wait for a signal to exit
        final Thread mainThread = Thread.currentThread();
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutdown signal received.");
            mainThread.interrupt();
        }));


        try {
            // Keep the main thread alive until it's interrupted
            while (!Thread.interrupted()) {
                Thread.sleep(1000);
            }
        } catch (InterruptedException e) {
            System.out.println("Main thread interrupted, exiting.");
        }
        
    }
}
