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
import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmrunner.EVMVersion;
import io.kaleido.paladin.toolkit.Algorithms;
import io.kaleido.paladin.toolkit.DomainInstance;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.hyperledger.besu.evm.internal.EvmConfiguration;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.math.BigInteger;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CompletableFuture;

/**
 * The most important part of the external interface of Pente is the way you construct transactions.
 * The ABI has to be constructed by the caller to describe the private transaction operation to be performed in the
 * privacy group, alongside providing the inputs for that ABI.
 * <p>
 * This needs to be capable of deploying new smart contracts, and invoking any existing smart contracts.
 * <p>
 * We support pre-encoded "data", or a set of "inputs" in JSON format that are parsed according to the
 * supplied ABI description of the ABI of the function.
 * // TODO: ensure that this feels consistent in naming between the external domain transaction interface, and the nested Pente TX description
 * <p>
 * You specify an ABI entry that can be one of:
 * - The special "invoke" function name combined with "data" param, which will be processed like the "data" of eth_sendTransaction
 * - The special "deploy" function name combined with "bytecode" + "inputs" params
 * - Any other function name _without_ "data" or "bytecode", and with a special input called "inputs" describing the function inputs to encode
 * All transaction must always have the following inputs:
 *  // TODO: consider paladin providing privacy group storage to avoid needing the "group" parameter
 * - group:    { "name": "group",   "type": "tuple", "components": [ { "name": "salt", "type": "bytes32" }, { "members": "type": "string[]" } ] }
 * Optionally you can have these additional top-level fields:
 * - to:       { "name": "to",      "type": "string" } OR { "name": "to", "type": "address" } // exclude this for deployments
 * - gas:      { "name": "gas",     "type": "uint256" } // if you exclude this then gas estimation will be performed for you by Pente
 * - value:    { "name": "value",   "type": "uint256" } // only if you want to transfer tokens in the privacy group (each group has separate base "eth" tokens)
 * If you want to pre-encode your calldata (or bytecode + params) yourself, then you can add an ABI param as follows:
 * - data:     { "name": "data",    "type": "bytes" }
 * For deployment with Paladin encoding the inputs parameters to your constructor add:
 * - bytecode: { "name": "bytecode", "type": "bytes" }
 * For invoking your own contract function (with the function name set to your own function name), or with the "bytecode" option, add your inputs:
 * - inputs:   { "name": "inputs",   "type": "tuple", "components": [ ... your function input definitions go here ] }
 * If you are performing a call (rather than an invoke) of an existing function, then you can add "outputs" too:
 * - outputs:  { "name": "outputs",  "type": "tuple", "components": [ ... your function output definitions go here ] }
 */
class PenteTransaction {
    private static final Logger LOGGER = LogManager.getLogger(PenteTransaction.class);

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record Values (
        @JsonProperty
        PenteConfiguration.GroupTupleJSON group,
        @JsonProperty
        String to,
        @JsonProperty
        BigInteger gas,
        @JsonProperty
        BigInteger data,
        @JsonProperty
        BigInteger value,
        @JsonProperty
        JsonHex.Bytes bytecode,
        @JsonProperty
        JsonNode inputs
    ) {}

    private enum ABIEntryType { INVOKE, DEPLOY, CUSTOM_FUNCTION }
    private final ABIEntryType abiEntryType;

    static class ABIDefinitions {
        JsonABI.Parameter group = null;
        JsonABI.Parameter to = null;
        JsonABI.Parameter gas = null;
        JsonABI.Parameter value = null;
        JsonABI.Parameter data = null;
        JsonABI.Parameter bytecode = null;
        JsonABI.Parameter inputs = null; // this level is fully customizable
        JsonABI.Parameter outputs = null; // this level is fully customizable
    }

    private final ABIDefinitions defs = new ABIDefinitions();

    private final PenteDomain domain;
    private final JsonABI.Entry functionDef;
    private final byte[] contractConfig;
    private final String from;
    private final String jsonParams;
    private final long baseBlock;
    private final Address contract;
    private Values values;

    PenteTransaction(PenteDomain domain, ToDomain.TransactionSpecification tx) throws IOException, IllegalArgumentException {
        this.domain = domain;
        contract = new Address(tx.getContractAddress());
        contractConfig = tx.getContractConfig().toByteArray();
        from = tx.getFrom();
        baseBlock = tx.getBaseBlock();
        // Check the ABI params we expect at the top level (we don't mind the order)
        functionDef = new ObjectMapper().readValue(tx.getFunctionAbiJson(), JsonABI.Entry.class);
        for (JsonABI.Parameter param : functionDef.inputs()) {
            switch (param.name()) {
                case "group" -> defs.group = checkGroup(param);
                case "to" -> defs.to = checkABIMatch(param, "string","address");
                case "gas" -> defs.gas = checkABIMatch(param, "uint256");
                case "value" -> defs.value = checkABIMatch(param, "uint256");
                case "data" -> defs.data = checkABIMatch(param, "bytes");
                case "bytecode" -> defs.bytecode = checkABIMatch(param, "bytes");
                case "inputs" -> defs.inputs = checkABIMatch(param, "tuple");
                case "outputs" -> defs.outputs = checkABIMatch(param, "tuple");
                default -> throw new IllegalArgumentException("ABI param '%s' is not in expected list".formatted(param.name()));
            }
        }
        // Check we have one of the combinations we support
        if (from.isBlank()) {
            throw new IllegalArgumentException("Value for 'from' is required");
        }
        if (defs.group == null) {
            throw new IllegalArgumentException("ABI params 'group' (tuple) and 'from' (address or string) are required");
        }
        if (functionDef.name().equals("invoke") && defs.data != null) {
            abiEntryType = ABIEntryType.INVOKE;
        } else if (functionDef.name().equals("deploy") && defs.bytecode != null && defs.inputs != null) {
            abiEntryType = ABIEntryType.DEPLOY;
        } else if (defs.inputs != null) {
            abiEntryType = ABIEntryType.CUSTOM_FUNCTION;
        } else {
            throw new IllegalArgumentException("ABI definition must be 'invoke' with 'data' param, 'deploy' with 'bytecode'+'inputs' params, or any function name with an 'inputs' param");
        }
        jsonParams = tx.getFunctionParamsJson();
    }

    Values getValues() throws IOException, IllegalArgumentException {
        if (values == null) {
            values = checkValues(new ObjectMapper().readValue(jsonParams, Values.class));
        }
        return values;
    }

    private Values checkValues(Values values) throws IllegalArgumentException {
        if (values.group == null || values.group.salt() == null ||
                values.group.members() == null || values.group.members().length == 0) {
            throw new IllegalArgumentException("Value for 'group.salt' and 'group.members' (with at least one member) is required");
        }
        if (defs.gas != null && values.gas == null) {
            throw new IllegalArgumentException("Value for 'gas' is required when the ABI includes a gas parameter");
        }
        if (defs.value != null && values.value == null) {
            throw new IllegalArgumentException("Value for 'value' is required when the ABI includes a value parameter");
        }
        if (abiEntryType == ABIEntryType.INVOKE) {
            if (values.data == null) {
                throw new IllegalArgumentException("Value for 'data' is required for 'invoke' transactions (can be zero-length bytes for simple transfers)");
            }
            if (values.inputs != null) {
                throw new IllegalArgumentException("Value for 'inputs' cannot be specified for 'invoke' transactions");
            }
        }
        if (abiEntryType == ABIEntryType.CUSTOM_FUNCTION) {
            if (values.to == null || values.to.isBlank()) {
                throw new IllegalArgumentException("Value for 'to' is required for function invocations");
            }
            if (values.bytecode != null) {
                throw new IllegalArgumentException("Value for 'bytecode cannot be specified for function invocations");
            }
        }
        if (abiEntryType == ABIEntryType.DEPLOY) {
            if (values.bytecode == null || values.bytecode.getBytes().length == 0) {
                throw new IllegalArgumentException("Non-empty bytes value for 'bytecode' is required for 'deploy' transactions");
            }
            if (values.to != null) {
                throw new IllegalArgumentException("Value for 'to' cannot be specified for 'deploy' transactions");
            }
        }
        if (abiEntryType == ABIEntryType.DEPLOY || abiEntryType == ABIEntryType.CUSTOM_FUNCTION) {
            if (values.inputs == null || !(values.inputs.isObject() || values.inputs.isArray())) {
                throw new IllegalArgumentException("Object or array value for 'inputs' must be specified for function invocations and 'deploy' transactions");
            }
        }
        return values;
    }

    PenteConfiguration.OnChainConfig getConfig() throws ClassNotFoundException {
        return PenteConfiguration.decodeConfig(this.contractConfig);
    }

    EVMRunner getEVM(long chainId, long blockNumber) throws ClassNotFoundException, IOException {
        var evmConfig = EvmConfiguration.DEFAULT;
        var evmVersionStr = getConfig().evmVersion();
        EVMVersion evmVersion = switch (evmVersionStr) {
            case "london" -> EVMVersion.London(chainId, evmConfig);
            case "paris" -> EVMVersion.Paris(chainId, evmConfig);
            case "shanghai" -> EVMVersion.Shanghai(chainId, evmConfig);
            default -> throw new IllegalArgumentException("unknown EVM version '%s'".formatted(evmVersionStr));
        };
        return new EVMRunner(evmVersion, domain.accountLoader(), blockNumber);
    }

    boolean requiresABIEncoding() {
        return (abiEntryType == ABIEntryType.DEPLOY || abiEntryType == ABIEntryType.CUSTOM_FUNCTION);
    }

    enum ABIEncodingRequestType {
        CONSTRUCTOR_PARAMS("constructorParams"),
        FUNCTION_PARAMS("functionParams")
        ;
        private final String text;
        @Override
        public String toString() { return text; }
        ABIEncodingRequestType(final String text) { this.text = text; }
    }

    ToDomain.ABIEncodingRequest getABIEncodingRequest() throws IOException, IllegalStateException {
        String paramsJSON = new ObjectMapper().writeValueAsString(getValues().inputs);
        switch (abiEntryType) {
        case ABIEntryType.DEPLOY -> {
            return ToDomain.ABIEncodingRequest.newBuilder().
                    setName(ABIEncodingRequestType.CONSTRUCTOR_PARAMS.text).
                    setAbiEncodingType(ToDomain.ABIEncodingRequest.ABIEncodingType.TUPLE).
                    setAbiEntry(defs.inputs.toJSON(false)).
                    setParamsJson(paramsJSON).
                    build();
        }
        case ABIEntryType.CUSTOM_FUNCTION -> {
            JsonABI.Entry functionEntry = JsonABI.newFunction(functionDef.name(), defs.inputs.components(), JsonABI.newParameters());
            return ToDomain.ABIEncodingRequest.newBuilder().
                    setName(ABIEncodingRequestType.FUNCTION_PARAMS.text).
                    setAbiEncodingType(ToDomain.ABIEncodingRequest.ABIEncodingType.TUPLE).
                    setAbiEntry(functionEntry.toJSON(false)).
                    setParamsJson(paramsJSON).
                    build();
        }
        default -> throw new IllegalStateException("no ABI encoding required for %s".formatted(abiEntryType));
        }
    }

    byte[] getABIEncodedData(List<ToDomain.ABIEncodedData> abiEncodedData, ABIEncodingRequestType type) {
        for (var data : abiEncodedData) {
            if (data.getName().equals(type.text)) {
                return data.getData().toByteArray();
            }
        }
        throw new IllegalArgumentException("missing ABI encoded data '%s'".formatted(type.text));
    }

    private JsonABI.Parameter checkABIMatch(JsonABI.Parameter param, String ...expectedTypes) throws IllegalArgumentException {
        for (String expectedType : expectedTypes) {
            if (param.type().equals(expectedType)) {
                return param;
            }
        }
        throw new IllegalArgumentException("ABI param '%s' should be one of the following types: %s".formatted(param.name(), Arrays.asList(expectedTypes)));
    }

    private JsonABI.Parameter checkGroup(JsonABI.Parameter param) throws IllegalArgumentException {
        if (!param.type().equals("tuple") ||
                param.components() == null || param.componentsOrEmpty().size() != 2 ||
                !param.components().getFirst().name().equals("salt") ||
                !param.components().getFirst().type().equals("bytes32") ||
                !param.components().get(1).name().equals("members") ||
                !param.components().get(1).type().equals("string[]")
        ) {
            throw new IllegalArgumentException("ABI param 'group' must be a tuple with two components - 'salt' (bytes32) and 'members' (string[])");
        }
        return param;
    }

    ABIDefinitions getABIDefinitions() {
        return defs;
    }

    Address getFromVerifier(List<ToDomain.ResolvedVerifier> verifiers) {
        for (var verifier : verifiers) {
            if (verifier.getAlgorithm().equals(Algorithms.ECDSA_SECP256K1_PLAINBYTES) && verifier.getLookup().equals(from)) {
                return new Address(verifier.getVerifier());
            }
        }
        throw new IllegalArgumentException("missing resolved %s verifier for '%s'".formatted(Algorithms.ECDSA_SECP256K1_PLAINBYTES, from));
    }

    String getFrom() { return from; }

    long getBaseBlock() { return baseBlock; }

}
