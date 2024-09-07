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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.web3j.abi.TypeEncoder;
import org.web3j.abi.datatypes.DynamicArray;
import org.web3j.abi.datatypes.DynamicStruct;
import org.web3j.abi.datatypes.generated.Uint256;

import java.io.IOException;
import java.io.StringReader;
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

    private Address address;

    private String schemaId_AccountStateV20240902;

    record Schema(String id, String signature, JsonABI.Parameter def) {}

    private final Map<String, Schema> schemasByID = new HashMap<>();

    PenteConfiguration() {
        try {
            factoryContractABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/pente/PenteFactory.sol/PenteFactory.json",
                    "abi");
            privacyGroupABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/pente/PentePrivacyGroup.sol/PentePrivacyGroup.json",
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
            String endorsementType
    ) {}

    public static String ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES =
            "group_scoped_identities";

    JsonABI.Entry abiEntry_privateConstructor() {
        return JsonABI.newConstructor(JsonABI.newParameters(
                abiTuple_group(),
                JsonABI.newParameter("endorsementType", "string")
        ));
    }

    JsonABI.Entry abiEntry_privateTransactionInvoke() {
        return JsonABI.newConstructor(JsonABI.newParameters(
                abiTuple_group(),
                JsonABI.newParameter("from", "string"),
                JsonABI.newParameter("to", "string"),
                JsonABI.newParameter("gas", "uint256"),
                JsonABI.newParameter("value", "uint256"),
                JsonABI.newParameter("data", "bytes")
        ));
    }

    JsonABI.Parameter abiTuple_AccountStateV20240902() {
        return JsonABI.newTuple("AccountStateV20240902", "AccountStateV20240902", JsonABI.newParameters(
            JsonABI.newIndexedParameter("address", "address"),
            JsonABI.newParameter("nonce", "uint256"),
            JsonABI.newParameter("balance", "uint256"),
            JsonABI.newParameter("codeHash", "bytes32"),
            JsonABI.newParameter("code", "bytes"),
            JsonABI.newParameter("storageRoot", "bytes32"),
            JsonABI.newTupleArray("storage", "StorageTrie", JsonABI.newParameters(
                JsonABI.newParameter("key", "bytes32"),
                JsonABI.newParameter("value", "bytes32")
            ))
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

    public static byte[] fourByteSelector(int val) {
        return ByteBuffer.allocate(4).putInt(val).array();
    }

    public static final byte[] PenteConfigID_Endorsement_V0 = fourByteSelector(0x00010000);;

    record Endorsement_V0(
            int threshold,
            List<Address> addresses
    ) {}

     static byte[] abiEncoder_Endorsement_V0(Endorsement_V0 config) {
        Uint256 w3Threshold = new Uint256(config.threshold());
        List<org.web3j.abi.datatypes.Address> w3Addresses = new ArrayList<>(config.addresses().size());
        for (Address addr : config.addresses()) {
            org.web3j.abi.datatypes.Address w3Address = new org.web3j.abi.datatypes.Address(addr.to0xHex());
            w3Addresses.add(w3Address);
        }
        DynamicArray<org.web3j.abi.datatypes.Address> w3AddressArray =
                new DynamicArray<>(org.web3j.abi.datatypes.Address.class, w3Addresses);
        return TypeEncoder.encode(new DynamicStruct(w3Threshold, w3AddressArray)).getBytes();
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

    synchronized void initFromJSON(String yamlConfig) {
        try {
            final ObjectMapper mapper = new ObjectMapper();
            PenteYAMLConfig config = mapper.readValue(new StringReader(yamlConfig), PenteYAMLConfig.class);
            this.address = new Address(config.address());
        } catch(IOException e) {
            throw new IllegalArgumentException(e);
        }
    }

    List<String> allPenteSchemas() {
        return Arrays.asList(abiTuple_AccountStateV20240902().toString());
    }

    synchronized void schemasInitialized(List<ToDomain.StateSchema> schemas) {
        List<String> schemaDefs = allPenteSchemas();
        if (schemas.size() != schemaDefs.size()) {
            throw new IllegalStateException("expected %d schemas, received %d".formatted(schemaDefs.size(), schemas.size()));
        }
        schemaId_AccountStateV20240902 = schemas.getFirst().getId();
        schemasByID.put(schemaId_AccountStateV20240902, new Schema(
                schemas.getFirst().getId(),
                schemas.getFirst().getSignature(),
                abiTuple_AccountStateV20240902()
        ));
    }

    synchronized Schema schema_AccountStateV20240902() {
        return schemasByID.get(schemaId_AccountStateV20240902);
    }

    synchronized Schema schema_AccountStateLatest() {
        return schema_AccountStateV20240902();
    }

}
