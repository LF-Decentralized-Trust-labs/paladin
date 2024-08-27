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

package io.kaleido.paladin;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.io.IOException;
import java.net.ServerSocket;
import java.net.URI;
import java.net.URL;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.sql.Time;
import java.time.Duration;
import java.time.LocalTime;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class RunTestbedTest {

    @Test
    void runTestbed() throws Exception {
        System.out.println(System.getProperty("os.name"));
        System.out.println(System.getProperty("os.arch"));
        System.out.println(System.getProperty("java.library.path"));

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
  ws:
    disabled: true
blockchain:
   http:
     url: http://localhost:8545
   ws:
     url: ws://localhost:8546
loader:
  debug: true
""".formatted(new File("../kata/db/migrations/sqlite").getAbsolutePath(), availableRPCPort);
        final File configFile = File.createTempFile("paladin-ut-", ".yaml");
        Files.writeString(configFile.toPath(), yamlContent);

        // Kick off the load in the background
        CompletableFuture<Integer> rc =
                CompletableFuture.supplyAsync(() -> Main.run(new String[]{
                        configFile.getAbsolutePath(),
                        "testbed",
                }));

        // Spin trying to connect to the RPC endpoint
        long startTime = System.currentTimeMillis();
        boolean connected = false;
        while (!connected) {
            final URI testbedRPC = new URI(String.format("http://127.0.0.1:%d", availableRPCPort));
            try (HttpClient rpcClient = HttpClient.newBuilder().build()) {
                HttpRequest req = HttpRequest.newBuilder()
                    .timeout(Duration.ofSeconds(1))
                    .uri(testbedRPC)
                        .POST(HttpRequest.BodyPublishers.ofString("""
                            {
                               "jsonrpc": "2.0",
                               "id": 1,
                               "method": "testbed_listDomains",
                               "params": []
                            }
                        """))
                        .build();
                HttpResponse<String> res = rpcClient.send(req, HttpResponse.BodyHandlers.ofString());
                assertEquals(200, res.statusCode(), res.body());
                connected = true;
            } catch(IOException e) {
                System.err.printf("Waiting to connect: %s\n", e);
            }
            assertTrue(System.currentTimeMillis()-startTime < 5000, "Test ran too long");
            if (rc.isDone()) {
                assertEquals(0, rc.get());
            }
            Thread.sleep(250);
        }
    }
}
