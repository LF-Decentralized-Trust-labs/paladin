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

 package io.kaleido.paladin.pente.domain;

 import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
 import com.fasterxml.jackson.annotation.JsonInclude;
 import com.fasterxml.jackson.annotation.JsonProperty;
 import com.fasterxml.jackson.databind.JsonNode;
 import io.kaleido.paladin.logging.PaladinLogging;
 import io.kaleido.paladin.toolkit.*;
 import io.kaleido.paladin.toolkit.JsonHex.Address;
 import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
 import org.apache.logging.log4j.Logger;
 import org.web3j.abi.TypeDecoder;
 import org.web3j.abi.TypeEncoder;
 import org.web3j.abi.TypeReference;
 import org.web3j.abi.datatypes.Bool;
 import org.web3j.abi.datatypes.DynamicArray;
 import org.web3j.abi.datatypes.DynamicStruct;
 import org.web3j.abi.datatypes.Utf8String;
 import org.web3j.abi.datatypes.generated.Uint256;
 import org.web3j.abi.datatypes.reflection.Parameterized;
 
 import java.nio.ByteBuffer;
 import java.util.ArrayList;
 import java.util.List;
 
 /**
  * Provides thread safe access to the configuration of the domain from the functions that are called
  * on any thread.
  **/
 public class PenteConfiguration {
     private static final Logger LOGGER = PaladinLogging.getLogger(PenteConfiguration.class);

     public static String ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES = "group_scoped_identities";
     public static final String DEFAULT_EVM_VERSION = "shanghai";
     public static final String DEFAULT_ENDORSEMENT_TYPE = ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES;

     public static final String transferSignature = "event PenteTransition(bytes32 txId, bytes32[] inputs, bytes32[] reads, bytes32[] outputs, bytes32[] info)";
     public static final String approvalSignature = "event PenteApproved(bytes32 txId, address delegate, bytes32 transitionHash)";
 
     private final JsonABI factoryContractABI;
 
     private final JsonABI privacyGroupABI;
 
     private final JsonABI eventsABI;
 
     private final JsonABI.Entry externalCallEventABI;
 
     // Topic generated from event "PenteExternalCall(address,bytes)"
     private final JsonHex.Bytes32 externalCallTopic = new JsonHex.Bytes32("0xcac03685d5ba4ab3e1465a8ee1b2bb21094ddbd612a969fd34f93a5be7a0ac4f");
 
     private String domainName;
     private long chainId;
 
     private String schemaId_AccountState_v24_10_0;
 
     private String schemaId_TransactionInfoState_v24_10_0;
 
     PenteConfiguration() {
         try {
             factoryContractABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                     "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                     "abi");
             privacyGroupABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                     "contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json",
                     "abi");
             // Note that technically we could just supply the IPente interface ABI for the events, but instead we
             // grab the events we need from the full privacy group ABI along with the error definitions.
             // This means that Paladin has our full list of error definitions to decode on-chain errors if things go wrong.
             eventsABI = new JsonABI();
             eventsABI.addAll(privacyGroupABI.stream().filter(e ->
                     e.type().equals("error") ||
                             (e.type().equals("event") && (e.name().equals("PenteApproved") || e.name().equals("PenteTransition")))
             ).toList());
             var externalCallABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                     "contracts/domains/interfaces/IPenteExternalCall.sol/IPenteExternalCall.json",
                     "abi");
             externalCallEventABI = externalCallABI.getABIEntry("event", "PenteExternalCall");
         } catch (Exception t) {
             LOGGER.error("failed to initialize configuration", t);
             throw new RuntimeException(t);
         }
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
    public record GroupTupleJSON(
             @JsonProperty
             Bytes32 salt,
             @JsonProperty
             String[] members
     ) {
     }

     /** JSON decoding for the parameters that can be supplied directly to a transaction initializing a group,
      *  and also used for JSON decoding of the "pente" input properties that can be optionally supplied
      *  when using the privacy group feature of Paladin to manage group genesis state distribution. */
     @JsonIgnoreProperties(ignoreUnknown = true)
     public record PrivacyGroupConstructorParamsJSON(
             @JsonProperty
             @JsonInclude(JsonInclude.Include.NON_NULL) // so we do not serialize when un-used in pente group settings
             GroupTupleJSON group,
             @JsonProperty
             String evmVersion,
             @JsonProperty
             String endorsementType,
             @JsonProperty
             boolean externalCallsEnabled
     ) {
     }

     @JsonIgnoreProperties(ignoreUnknown = true)
     record PenteTransitionParams(
             @JsonProperty
             JsonHex.Bytes32 txId,
             @JsonProperty
             JsonNode states,
             @JsonProperty
             JsonNode externalCalls,
             @JsonProperty
             List<JsonHex.Bytes> signatures
     ) {
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     record PenteApprovalParams(
             @JsonProperty
             JsonHex.Bytes32 transitionHash,
             @JsonProperty
             List<JsonHex.Bytes> signatures
     ) {
     }
 
     record PentePublicTransaction(
             @JsonProperty
             JsonABI.Entry functionABI,
             @JsonProperty
             String paramsJSON,
             @JsonProperty
             JsonHex.Bytes encodedCall
     ) {
     }
 
     record PenteTransitionMetadata(
             @JsonProperty
             PenteApprovalParams approvalParams,
             @JsonProperty
             PentePublicTransaction transitionWithApproval
     ) {
     }

     public static final String FUNCTION_NAME_INVOKE = "invoke";
 
     public static final String FUNCTION_NAME_DEPLOY = "deploy";
 
     JsonABI.Parameter abiTuple_AccountState_v24_10_0() {
         return JsonABI.newTuple("AccountState_v24_10_0", "AccountState_v24_10_0", JsonABI.newParameters(
                 JsonABI.newIndexedParameter("version", "string"),
                 JsonABI.newIndexedParameter("address", "address"),
                 JsonABI.newParameter("salt", "bytes32"),
                 JsonABI.newParameter("nonce", "uint256"),
                 JsonABI.newParameter("balance", "uint256"),
                 JsonABI.newParameter("codeHash", "bytes32"),
                 JsonABI.newParameter("code", "bytes"),
                 JsonABI.newParameter("storageRoot", "bytes32"),
                 JsonABI.newParameter("storage", "bytes32[2][]")
         ));
     }
 
     JsonABI.Parameter abiTuple_TransactionInfoState_v24_10_0() {
         return JsonABI.newTuple("TransactionInfoState_v24_10_0", "TransactionInfoState_v24_10_0", JsonABI.newParameters(
                 JsonABI.newParameter("salt", "bytes32"),
                 JsonABI.newParameter("rawTransaction", "bytes"),
                 JsonABI.newParameter("evmVersion", "string"),
                 JsonABI.newParameter("baseBlock", "uint64"),
                 JsonABI.newParameter("bytecodeLength", "uint32")
         ));
     }
 
     record NewPrivacyGroupFactoryParams(
             @JsonProperty()
             Bytes32 transactionId,
             @JsonProperty()
             JsonHex.Bytes data
     ) {
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     record TransactionExternalCall(
             @JsonProperty
             Address contractAddress,
             @JsonProperty
             JsonHex.Bytes encodedCall
     ) {
     }
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     public record DomainData(
             @JsonProperty
             Address contractAddress,
             @JsonProperty
             List<TransactionExternalCall> externalCalls
     ) {
     }
 
     public static byte[] intToBytes4(int val) {
         return ByteBuffer.allocate(4).putInt(val).array();
     }
 
     public static int bytes4ToInt(byte[] data, int offset, int len) {
         return ByteBuffer.wrap(data, offset, len).getInt();
     }
 
     public static final int PenteConfigID_V0 = 0x00010000;
 
     @JsonIgnoreProperties(ignoreUnknown = true)
     record ContractConfig(
             @JsonProperty()
             String evmVersion
     ) {
     }
 
     interface OnChainConfig {
         String evmVersion();
     }
 
     public static class PenteConfig_V0 extends DynamicStruct implements OnChainConfig {
         public Utf8String evmVersion;
         public Uint256 threshold;
         public DynamicArray<org.web3j.abi.datatypes.Address> addresses;
         public Bool externalCallsEnabled;
 
         public PenteConfig_V0(
                 Utf8String evmVersion,
                 Uint256 threshold,
                 @Parameterized(type = org.web3j.abi.datatypes.Address.class)
                 DynamicArray<org.web3j.abi.datatypes.Address> addresses,
                 Bool externalCallsEnabled) {
             super(evmVersion, threshold, addresses);
             this.evmVersion = evmVersion;
             this.threshold = threshold;
             this.addresses = addresses;
             this.externalCallsEnabled = externalCallsEnabled;
         }
 
         public String evmVersion() {
             return evmVersion.getValue();
         }
     }

     public static JsonHex.Bytes abiEncoder_Config_V0(String evmVersion, int threshold, List<JsonHex.Address> endorsers, boolean externalCallsEnabled) {
         var w3Addresses = new ArrayList<org.web3j.abi.datatypes.Address>(endorsers.size());
         for (var addr : endorsers) {
             org.web3j.abi.datatypes.Address w3Address = new org.web3j.abi.datatypes.Address(addr.to0xHex());
             w3Addresses.add(w3Address);
         }
         var w3AddressArray = new DynamicArray<>(org.web3j.abi.datatypes.Address.class, w3Addresses);
         return new JsonHex.Bytes(TypeEncoder.encode(new PenteConfig_V0(
                 new org.web3j.abi.datatypes.Utf8String(evmVersion),
                 new Uint256(threshold),
                 w3AddressArray,
                 new Bool(externalCallsEnabled)
         )));
     }
 
     public static PenteConfiguration.PenteConfig_V0 abiDecoder_Config_V0(JsonHex data, int offset) throws ClassNotFoundException {
         return TypeDecoder.decodeDynamicStruct(data.to0xHex(), 2 + (2 * offset), TypeReference.create(PenteConfiguration.PenteConfig_V0.class));
     }
 
     static OnChainConfig decodeConfig(byte[] constructorConfig) throws IllegalArgumentException, ClassNotFoundException {
         if (constructorConfig.length < 4) {
             throw new IllegalArgumentException("on-chain configuration must be at least 4 bytes");
         }
         return switch (bytes4ToInt(constructorConfig, 0, 4)) {
             case PenteConfigID_V0 -> abiDecoder_Config_V0(JsonHex.wrap(constructorConfig), 4);
             default ->
                     throw new IllegalArgumentException("unknown config ID: %s".formatted(JsonHex.from(constructorConfig, 0, 4)));
         };
     }
 
     synchronized JsonABI getFactoryContractABI() {
         return factoryContractABI;
     }
 
     synchronized JsonABI getPrivacyGroupABI() {
         return privacyGroupABI;
     }
 
     synchronized JsonABI getEventsABI() { return eventsABI; }
 
     synchronized JsonABI.Entry getExternalCallEventABI() { return externalCallEventABI; }
 
     synchronized JsonHex.Bytes32 getExternalCallTopic() { return externalCallTopic; }
 
     synchronized long getChainId() {
         return chainId;
     }
 
     synchronized String getDomainName() {
         return domainName;
     }
 
     synchronized void initFromConfig(ConfigureDomainRequest configReq) {
         this.domainName = configReq.getName();
         this.chainId = configReq.getChainId();
     }
 
     List<String> allPenteSchemas() {
         return List.of(
                 abiTuple_AccountState_v24_10_0().toString(),
                 abiTuple_TransactionInfoState_v24_10_0().toString()
         );
     }
 
     synchronized void schemasInitialized(List<StateSchema> schemas) {
         var schemaDefs = allPenteSchemas();
         if (schemas.size() != schemaDefs.size()) {
             throw new IllegalStateException("expected %d schemas, received %d".formatted(schemaDefs.size(), schemas.size()));
         }
         var schema = schemas.getFirst();
         schemaId_AccountState_v24_10_0 = schema.getId();
         schema = schemas.get(1);
         schemaId_TransactionInfoState_v24_10_0 = schema.getId();
     }
 
     synchronized String schemaId_AccountStateLatest() {
         return schemaId_AccountState_v24_10_0;
     }
 
     synchronized String schemaId_TransactionInputStateLatest() {
         return schemaId_TransactionInfoState_v24_10_0;
     }
 
 }
 