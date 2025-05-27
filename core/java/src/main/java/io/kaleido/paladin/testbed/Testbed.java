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


 package io.kaleido.paladin.testbed;

 import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
 import com.fasterxml.jackson.databind.JsonNode;
 import io.kaleido.paladin.logging.PaladinLogging;
 import io.kaleido.paladin.toolkit.JsonABI;
 import io.kaleido.paladin.toolkit.JsonHex;
 import io.kaleido.paladin.toolkit.JsonRpcClient;
 
 import com.fasterxml.jackson.annotation.JsonInclude;
 import com.fasterxml.jackson.annotation.JsonProperty;
 import com.fasterxml.jackson.core.type.TypeReference;
 import com.fasterxml.jackson.databind.ObjectMapper;
 import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
 import io.kaleido.paladin.Main;
 import org.apache.logging.log4j.Logger;
 
 import java.io.Closeable;
 import java.io.File;
 import java.io.IOException;
 import java.net.ServerSocket;
 import java.nio.file.Files;
 import java.util.HashMap;
 import java.util.List;
 import java.util.Map;
 import java.util.UUID;
 import java.util.concurrent.CompletableFuture;
 import java.util.concurrent.ExecutionException;
 import java.util.concurrent.TimeoutException;
 
 public class Testbed implements Closeable {
 
     private final Logger LOGGER = PaladinLogging.getLogger(Testbed.class);
 
     private final String yamlConfigMerged;
 
     private final long availableRPCPort;
 
     private final ConfigDomain[] configuredDomains;
 
     private CompletableFuture<Integer> mainRun;
 
     private final Setup testbedSetup;
 
     private JsonRpcClient rpcClient;
 
     public record Setup(
             String dbMigrationsDir,
             String logFile,
             long startTimeoutMS
     ) {
     }
 
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     public record StateEncoded(
             @JsonProperty
             JsonHex.Bytes id,
             @JsonProperty
             String domain,
             @JsonProperty
             JsonHex.Bytes32 schema,
             @JsonProperty
             JsonHex.Address contractAddress,
             @JsonProperty
             JsonHex.Bytes data
     ) {
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     public record TransactionInput(
             @JsonProperty
             String type,
             @JsonProperty
             String domain,
             @JsonProperty
             String from,
             @JsonProperty
             JsonHex.Address to,
             @JsonProperty
             Map<String, Object> data,
             @JsonProperty
             JsonABI abi,
             @JsonProperty
             String function
     ) {
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     public record TransactionResult(
             @JsonProperty
             String id,
             @JsonProperty
             JsonHex.Bytes encodedCall,
             @JsonProperty
             TransactionInput preparedTransaction,
             @JsonProperty
             JsonNode preparedMetadata,
             @JsonProperty
             List<StateEncoded> inputStates,
             @JsonProperty
             List<StateEncoded> outputStates,
             @JsonProperty
             List<StateEncoded> readStates,
             @JsonProperty
             List<StateEncoded> infoStates,
             @JsonProperty
             JsonNode domainReceipt
     ) {
     }
 
     public Testbed(Setup testbedSetup, ConfigDomain... domains) throws Exception {
         this.testbedSetup = testbedSetup;
         this.configuredDomains = domains;
         // Assign ourselves a free port
         try (ServerSocket s = new ServerSocket(0);) {
             availableRPCPort = s.getLocalPort();
         }
 
         // Build the config
         ObjectMapper objectMapper = new ObjectMapper(YAMLFactory.builder().build());
         var baseConfig = baseConfig();
         Map<String, Object> configMap = objectMapper.readValue(baseConfig, new TypeReference<>() {
         });
         Map<String, Object> domainMap = new HashMap<>();
         for (ConfigDomain domain : domains) {
             domainMap.put(domain.name(), domain);
         }
         configMap.put("domains", domainMap);
         yamlConfigMerged = objectMapper.writerWithDefaultPrettyPrinter().writeValueAsString(configMap);
         try {
             start();
         } catch (Exception e) {
             close();
             throw e;
         }
     }
 
     public record ConfigDomain(
             String name,
             @JsonProperty
             JsonHex.Address registryAddress,
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
                     dsn:           ":memory:"
                     autoMigrate:   true
                     migrationsDir: %s
                     debugQueries:  false
                 wallets:
                 - name: wallet1
                   keySelector: .*
                   signer:
                     keyDerivation:
                       type: "bip32"
                     keyStore:
                       type: "static"
                       static:
                         keys:
                           seed:
                             encoding: hex
                             inline: '%s'                    
                 rpcServer:
                   http:
                     port: %s
                     shutdownTimeout: 0s
                   ws:
                     disabled: true
                     shutdownTimeout: 0s
                 grpc:
                     shutdownTimeout: 0s
                 blockIndexer:
                   fromBlock: latest
                 blockchain:
                    http:
                      url: http://localhost:8545
                    ws:
                      url: ws://localhost:8546
                 loader:
                   debug: true
                 log:
                   level: debug
                   output: file
                   file:
                     filename: %s
                 """.formatted(
                 new File(testbedSetup.dbMigrationsDir).getAbsolutePath(),
                 JsonHex.randomBytes32(),
                 availableRPCPort,
                 new File(testbedSetup.logFile).getAbsolutePath()
         );
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
         LOGGER.info("Stopping testbed");
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
 
 