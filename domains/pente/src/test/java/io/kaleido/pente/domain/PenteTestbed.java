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

package io.kaleido.pente.domain;

import io.kaleido.paladin.Main;
import io.kaleido.paladin.toolkit.JsonRpcClient;

import java.io.File;
import java.io.IOException;
import java.net.ServerSocket;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeoutException;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class PenteTestbed {

    CompletableFuture<Integer> rc;

    void start() throws Exception {
        // Generate config that listens on an available RPC port
        ServerSocket s = new ServerSocket(0);
        int availableRPCPort = s.getLocalPort();
        s.close();
        String yamlContent = """
db:
  type: sqlite
  sqlite:
    uri:           ":memory:"
    autoMigrate:   true
    migrationsDir: %s
    debugQueries:  true
signer:
  keyStore:
    type: static
    static:
      keys:
        seed:
          encoding: none
          inline: '17250abf7976eae3c964e9704063f1457a8e1b4c0c0bd8b21ec8db5b88743c10'
rpcServer:
  http:
    port: %s
    shutdownTimeout: 0s
  ws:
    disabled: true
    shutdownTimeout: 0s
grpc:
    shutdownTimeout: 0s
blockchain:
   http:
     url: http://localhost:8545
   ws:
     url: ws://localhost:8546
loader:
  debug: true
log:
  level: debug
domains:
  pente:
    plugin:
      type: jar
      class: %s
    config:
      address: %s
""".formatted(
        new File("../../kata/db/migrations/sqlite").getAbsolutePath(),
                availableRPCPort,
                PenteDomainFactory.class.getName(),
                "0x107A104E72fC31D9571a3B9a4eac91b9B530Ce99"
        );
        final File configFile = File.createTempFile("paladin-ut-", ".yaml");
        Files.writeString(configFile.toPath(), yamlContent);

        // Kick off the load in the background
        rc = CompletableFuture.supplyAsync(() -> Main.run(new String[]{
            configFile.getAbsolutePath(),
            "testbed",
        }));

        // Spin trying to connect to the RPC endpoint
        long startTime = System.currentTimeMillis();
        boolean connected = false;
        while (!connected) {
            try (JsonRpcClient rpcClient = new JsonRpcClient("http://127.0.0.1:%d".formatted(availableRPCPort))) {
                List<String> domains = rpcClient.request("testbed_listDomains");
                assertEquals(1, domains.size());
                connected = true;
            } catch(IOException e) {
                System.err.printf("Waiting to connect: %s\n", e);
            }
            assertTrue(System.currentTimeMillis()-startTime < 5000, "Startup took too too long");
            Thread.sleep(250);
        }
    }

    void stop() throws Exception {
        Main.stop();
        assertEquals(rc.get(), 0);
    }
}
