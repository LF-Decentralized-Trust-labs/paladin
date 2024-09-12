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
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.web3j.abi.TypeDecoder;
import org.web3j.abi.TypeEncoder;
import org.web3j.abi.TypeReference;
import org.web3j.abi.datatypes.*;
import org.web3j.abi.datatypes.generated.Uint256;
import org.web3j.abi.datatypes.reflection.Parameterized;

import java.io.*;
import java.math.BigInteger;
import java.nio.ByteBuffer;
import java.util.*;

/**
 * Provides thread safe access to the configuration of the domain from the functions that are called
 * on any thread.
 **/
public class PenteConfiguration {
    private static final Logger LOGGER = LogManager.getLogger(PenteConfiguration.class);

    private final JsonABI factoryContractABI;

    private final JsonABI privacyGroupABI;

    private long chainId;

    private Address address;

    private String schemaId_AccountState_v24_9_0;

    record Schema(String id, String signature, JsonABI.Parameter def) {}

    private final Map<String, Schema> schemasByID = new HashMap<>();

    PenteConfiguration() {
        try {
            factoryContractABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "abi");
            privacyGroupABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/domains/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json",
                    "abi");
        } catch (Exception t) {
            LOGGER.error("failed to initialize configuration", t);
            throw new RuntimeException(t);
        }
    }

    public static final String ENDORSEMENT_TYPE_GROUP_SCOPED_KEYS = "groupScopedKeys";

    @JsonIgnoreProperties(ignoreUnknown = true)
    record GroupTupleJSON(
            @JsonProperty
            Bytes32 salt,
            @JsonProperty
            String[] members
    ) {}

    private static JsonABI.Parameter abiTuple_group() {
        return JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                JsonABI.newParameter("salt", "bytes32"),
                JsonABI.newParameter("members", "string[]")
        ));
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record PrivacyGroupConstructorParamsJSON(
            @JsonProperty
            GroupTupleJSON group,
            @JsonProperty
            String evmVersion,
            @JsonProperty
            String endorsementType
    ) {}

    public static String ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES =
            "group_scoped_identities";

    JsonABI.Entry abiEntry_privateConstructor() {
        return JsonABI.newConstructor(JsonABI.newParameters(
                abiTuple_group(),
                JsonABI.newParameter("evmVersion", "string"),
                JsonABI.newParameter("endorsementType", "string")
        ));
    }

    public static final String FUNCTION_NAME_INVOKE = "invoke";

    public static final String FUNCTION_NAME_DEPLOY = "deploy";

    record ParsedInvokeInputs(
        @JsonProperty
        GroupTupleJSON group,
        @JsonProperty
        String from,
        @JsonProperty
        String to,
        @JsonProperty
        BigInteger gas, // jackson supports the decimal string format we normalize to before passing to domain
        @JsonProperty
        BigInteger value,
        @JsonProperty
        JsonHex.Bytes data, // for FUNCTION_NAME_INVOKE only - where the data is passed directly
        @JsonProperty
        JsonHex.Bytes bytecode, // for FUNCTION_NAME_DEPLOY only - where the inputs are encoded after the bytecode
        @JsonProperty
        JsonNode inputs // leave this unparsed as we will push it back ot paladin to parse for us
    ) {}

    JsonABI.Parameter abiTuple_AccountState_v24_9_0() {
        return JsonABI.newTuple("AccountState_v24_9_0", "AccountState_v24_9_0", JsonABI.newParameters(
            JsonABI.newIndexedParameter("version", "string"),
            JsonABI.newIndexedParameter("address", "address"),
            JsonABI.newParameter("nonce", "uint256"),
            JsonABI.newParameter("balance", "uint256"),
            JsonABI.newParameter("codeHash", "bytes32"),
            JsonABI.newParameter("code", "bytes"),
            JsonABI.newParameter("storageRoot", "bytes32"),
            JsonABI.newParameter("storage", "bytes32[2][]")
        ));
    }

    record NewPrivacyGroupFactoryParams(
            @JsonProperty()
            Bytes32 transactionId,
            @JsonProperty()
            JsonHex.Bytes config
    ) {}

    @JsonIgnoreProperties(ignoreUnknown = true)
    private record PenteYAMLConfig(
            @JsonProperty
            String address
    ) {}

    public static byte[] intToBytes4(int val) {
        return ByteBuffer.allocate(4).putInt(val).array();
    }

    public static int bytes4ToInt(byte[] data, int offset, int len) {
        return ByteBuffer.wrap(data, offset, len).getInt();
    }

    public static final int PenteConfigID_Endorsement_V0 = 0x00010000;

    interface OnChainConfig {
        String evmVersion();
    }

    public static class Endorsement_V0 extends DynamicStruct implements OnChainConfig {
        public Utf8String evmVersion;
        public Uint256 threshold;
        public DynamicArray<org.web3j.abi.datatypes.Address> addresses;
        public Endorsement_V0(
                Utf8String evmVersion,
                Uint256 threshold,
                @Parameterized(type = org.web3j.abi.datatypes.Address.class)
                DynamicArray<org.web3j.abi.datatypes.Address> addresses) {
            super(evmVersion, threshold, addresses);
            this.evmVersion = evmVersion;
            this.threshold = threshold;
            this.addresses = addresses;
        }

        public String evmVersion() {
            return evmVersion.getValue();
        }
    }

    public static JsonHex.Bytes abiEncoder_Endorsement_V0(String evmVersion, int threshold, List<JsonHex.Address> endorsers) {
        var w3Addresses = new ArrayList<org.web3j.abi.datatypes.Address>(endorsers.size());
        for (var addr : endorsers) {
            org.web3j.abi.datatypes.Address w3Address = new org.web3j.abi.datatypes.Address(addr.to0xHex());
            w3Addresses.add(w3Address);
        }
        var w3AddressArray = new DynamicArray<>(org.web3j.abi.datatypes.Address.class, w3Addresses);
        return new JsonHex.Bytes(TypeEncoder.encode(new Endorsement_V0(
                new org.web3j.abi.datatypes.Utf8String(evmVersion),
                new Uint256(threshold),
                w3AddressArray
        )));
    }

    public static Endorsement_V0 abiDecoder_Endorsement_V0(JsonHex data, int offset) throws ClassNotFoundException {
        return TypeDecoder.decodeDynamicStruct(data.to0xHex(), 2 + (2 * offset), TypeReference.create(Endorsement_V0.class));
    }

    static OnChainConfig decodeConfig(byte[] constructorConfig) throws IllegalArgumentException, ClassNotFoundException {
        if (constructorConfig.length < 4) {
            throw new IllegalArgumentException("on-chain configuration must be at least 4 bytes");
        }
        return switch (bytes4ToInt(constructorConfig, 0, 4)) {
            case PenteConfigID_Endorsement_V0 -> abiDecoder_Endorsement_V0(JsonHex.wrap(constructorConfig), 4);
            default -> throw new IllegalArgumentException("unknown config ID: %s".formatted(JsonHex.from(constructorConfig, 0, 4)));
        };
    }

    synchronized JsonABI getFactoryContractABI() {
        return factoryContractABI;
    }

    synchronized JsonABI getPrivacyGroupABI() {
        return privacyGroupABI;
    }

    synchronized Address getAddress() {
        return address;
    }

    synchronized long getChainId() {
        return chainId;
    }

    synchronized void initFromConfig(ToDomain.ConfigureDomainRequest configReq) {
        try {
            var mapper = new ObjectMapper();
            var config = mapper.readValue(new StringReader(configReq.getConfigJson()), PenteYAMLConfig.class);
            this.address = new Address(config.address());
            this.chainId = configReq.getChainId();
        } catch(IOException e) {
            throw new IllegalArgumentException(e);
        }
    }

    List<String> allPenteSchemas() {
        return Arrays.asList(abiTuple_AccountState_v24_9_0().toString());
    }

    synchronized void schemasInitialized(List<ToDomain.StateSchema> schemas) {
        var schemaDefs = allPenteSchemas();
        if (schemas.size() != schemaDefs.size()) {
            throw new IllegalStateException("expected %d schemas, received %d".formatted(schemaDefs.size(), schemas.size()));
        }
        schemaId_AccountState_v24_9_0 = schemas.getFirst().getId();
        schemasByID.put(schemaId_AccountState_v24_9_0, new Schema(
                schemas.getFirst().getId(),
                schemas.getFirst().getSignature(),
                abiTuple_AccountState_v24_9_0()
        ));
    }

    synchronized String schemaId_AccountStateLatest() {
        return schemaId_AccountState_v24_9_0;
    }

}
