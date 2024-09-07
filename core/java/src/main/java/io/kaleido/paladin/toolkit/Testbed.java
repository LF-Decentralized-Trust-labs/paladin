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

package io.kaleido.paladin.toolkit;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
import io.kaleido.paladin.Main;

import java.io.Closeable;
import java.io.File;
import java.io.IOException;
import java.net.ServerSocket;
import java.nio.file.Files;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeoutException;

public class Testbed implements Closeable {

    private final String yamlConfigMerged;

    private final long availableRPCPort;

    private final ConfigDomain[] configuredDomains;

    private CompletableFuture<Integer> mainRun;

    private final Setup testbedSetup;

    private JsonRpcClient rpcClient;

    public record Setup(
            String dbMigrationsDir,
            long startTimeoutMS
    ) {}

    public Testbed(Setup testbedSetup, ConfigDomain... domains) throws Exception {
        this.testbedSetup = testbedSetup;
        this.configuredDomains = domains;
        // Assign ourselves a free port
        try (ServerSocket s = new ServerSocket(0);) {
            availableRPCPort = s.getLocalPort();
        }

        // Build the config
        ObjectMapper objectMapper = new ObjectMapper(YAMLFactory.builder().build());
        Map<String, Object> configMap = objectMapper.readValue(baseConfig(), new TypeReference<>() {
        });
        Map<String, Object> domainMap = new HashMap<>();
        for (ConfigDomain domain : domains) {
            domainMap.put(domain.name(), domain);
        }
        configMap.put("domains", domainMap);
        yamlConfigMerged = objectMapper.writerWithDefaultPrettyPrinter().writeValueAsString(configMap);
        try {
            start();
        } catch(Exception e) {
            close();
            throw e;
        }
    }

    public record ConfigDomain(
            String name,
            @JsonProperty
            ConfigPlugin plugin,
            @JsonProperty
            Map<String, Object> config
    ) {
    }

    public record ConfigPlugin(
            @JsonProperty
            String type,
            @JsonProperty
            String library,
            @JsonProperty("class")
            @JsonInclude(JsonInclude.Include.NON_DEFAULT)
            String clazz
    ) {
    }

    private String baseConfig() {
        return """
                nodeName: node1
                db:
                  type: sqlite
                  sqlite:
                    uri:           ":memory:"
                    autoMigrate:   true
                    migrationsDir: %s
                    debugQueries:  true
                signer:
                  keyDerivation:
                    type: bip32
                  keyStore:
                    type: static
                    static:
                      keys:
                        seed:
                          encoding: hex
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
                  level: trace
                """.formatted(new File(testbedSetup.dbMigrationsDir).getAbsolutePath(), availableRPCPort);
    }

    private void start() throws Exception {
        final File configFile = File.createTempFile("paladin-ut-", ".yaml");
        Files.writeString(configFile.toPath(), yamlConfigMerged);

        // Kick off the load in the background
        setMainRun(CompletableFuture.supplyAsync(() -> Main.run(new String[]{
                configFile.getAbsolutePath(),
                "testbed",
        })));

        // Spin trying to connect to the RPC endpoint
        long startTime = System.currentTimeMillis();
        boolean connected = false;
        while (!connected) {
            long timeStarting = System.currentTimeMillis() - startTime;
            if (timeStarting > testbedSetup.startTimeoutMS) {
                throw new TimeoutException("timed out start after %dms".formatted(timeStarting));
            }
            Thread.sleep(250);

            rpcClient = new JsonRpcClient("http://127.0.0.1:%d".formatted(availableRPCPort));
            try {
                List<String> domains = rpcClient.request("testbed_listDomains");
                if (domains.size() != configuredDomains.length) {
                    throw new IllegalStateException("expected %d domains, found %d".formatted(configuredDomains.length, domains.size()));
                }
                connected = true;
            } catch (IOException e) {
                System.err.printf("Waiting to connect: %s\n", e);
                rpcClient.close();
                rpcClient = null;
            }
        }
    }

    private synchronized void setMainRun(CompletableFuture<Integer> mainRun) {
        this.mainRun = mainRun;
    }

    private synchronized CompletableFuture<Integer> getMainRun() {
        return this.mainRun;
    }

    public void stop() throws ExecutionException, InterruptedException {
        CompletableFuture<Integer> mainRun = getMainRun();
        if (mainRun != null) {
            Main.stop();
            int exitRC = mainRun.get();
            if (exitRC != 0) {
                throw new IllegalStateException("failed with RC=%d".formatted(exitRC));
            }
        }
    }

    public void close() {
        try {
            stop();
            if (rpcClient != null) {
                rpcClient.close();
            }
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    public JsonRpcClient getRpcClient() {
        return rpcClient;
    }
}
