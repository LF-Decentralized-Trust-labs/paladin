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
import com.fasterxml.jackson.databind.annotation.JsonSerialize;
import com.fasterxml.jackson.databind.ser.std.ToStringSerializer;
import com.google.protobuf.ByteString;
import github.com.kaleido_io.paladin.toolkit.FromDomain;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmrunner.EVMVersion;
import io.kaleido.paladin.pente.evmstate.AccountLoader;
import io.kaleido.paladin.pente.evmstate.DynamicLoadWorldState;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import io.kaleido.paladin.toolkit.Algorithms;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.Keccak;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;

import java.io.IOException;
import java.math.BigInteger;
import java.util.*;
import java.util.concurrent.ExecutionException;

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
        Address to,
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
    private Values values;
    private PenteDomain.AssemblyAccountLoader accountLoader;

    PenteTransaction(PenteDomain domain, ToDomain.TransactionSpecification tx) throws IOException, IllegalArgumentException {
        this.domain = domain;
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
            if (values.to == null) {
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

    EVMRunner getEVM(long chainId, long blockNumber, AccountLoader accountLoader) throws ClassNotFoundException {
        var evmConfig = EvmConfiguration.DEFAULT;
        var evmVersionStr = getConfig().evmVersion();
        EVMVersion evmVersion = switch (evmVersionStr) {
            case "london" -> EVMVersion.London(chainId, evmConfig);
            case "paris" -> EVMVersion.Paris(chainId, evmConfig);
            case "shanghai" -> EVMVersion.Shanghai(chainId, evmConfig);
            default -> throw new IllegalArgumentException("unknown EVM version '%s'".formatted(evmVersionStr));
        };
        return new EVMRunner(evmVersion, accountLoader, blockNumber);
    }

    boolean requiresABIEncoding() {
        return (abiEntryType == ABIEntryType.DEPLOY || abiEntryType == ABIEntryType.CUSTOM_FUNCTION);
    }

    byte[] getEncodedCallData() throws IOException, IllegalStateException, ExecutionException, InterruptedException {
        String paramsJSON = new ObjectMapper().writeValueAsString(getValues().inputs);
        FromDomain.EncodeDataRequest request;
        switch (abiEntryType) {
        case ABIEntryType.DEPLOY -> {
            request = FromDomain.EncodeDataRequest.newBuilder().
                    setEncodingType(FromDomain.EncodeDataRequest.EncodingType.TUPLE).
                    setDefinition(defs.inputs.toJSON(false)).
                    setBody(paramsJSON).
                    build();
        }
        case ABIEntryType.CUSTOM_FUNCTION -> {
            JsonABI.Entry functionEntry = JsonABI.newFunction(functionDef.name(), defs.inputs.components(), JsonABI.newParameters());
            request = FromDomain.EncodeDataRequest.newBuilder().
                    setEncodingType(FromDomain.EncodeDataRequest.EncodingType.FUNCTION_CALL_DATA).
                    setDefinition(functionEntry.toJSON(false)).
                    setBody(paramsJSON).
                    build();
        }
        default -> throw new IllegalStateException("no ABI encoding required for %s".formatted(abiEntryType));
        }
        var response = domain.encodeData(request).get();
        return response.getData().toByteArray();
    }

    /** The sub-set of Ethereum JSON fields for a transaction built on-demand from input, that we include in the signature that is verified by endorsement */
    record EndorsableEthTransactionJson(
        @JsonProperty
        Address to,
        @JsonProperty
        long nonce,
        @JsonProperty
        @JsonSerialize(using = ToStringSerializer.class)
        BigInteger gas,
        @JsonProperty
        @JsonSerialize(using = ToStringSerializer.class)
        BigInteger value,
        @JsonProperty
        JsonHex.Bytes data

        // Note no gas price used/supported in Pente transactions
    ) {}

    byte[] getEncodedTransaction(long nonce, byte[] calldata) throws IOException, IllegalStateException, ExecutionException, InterruptedException {
        var values = getValues();
        var ethTXJson = new EndorsableEthTransactionJson(
                values.to,
                nonce,
                values.gas,
                values.value,
                JsonHex.wrap(calldata)
        );
        var request = FromDomain.EncodeDataRequest.newBuilder().
                        setEncodingType(FromDomain.EncodeDataRequest.EncodingType.ETH_TRANSACTION).
                        setDefinition(defs.inputs.toJSON(false)).
                        setBody(new ObjectMapper().writeValueAsString(ethTXJson)).
                        setDefinition("eip-1559").
                        build();
        var response = domain.encodeData(request).get();
        return response.getData().toByteArray();
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

    record EVMStateResult(
            List<PersistedAccount> newAccountStates,
            ToDomain.AssembledTransaction assembledTransaction
    ) {}

    ToDomain.AssembledTransaction buildAssembledTransaction(EVMRunner evm, PenteDomain.AssemblyAccountLoader accountLoader) throws IOException {
        var latestSchemaId = domain.getConfig().schemaId_AccountStateLatest();
        var result = ToDomain.AssembledTransaction.newBuilder();
        var committedUpdates = evm.getWorld().getCommittedAccountUpdates();
        var loadedAccountStates = accountLoader.getLoadedAccountStates();
        var lookups = buildGroupScopeIdentityLookups(getValues().group().salt(), getValues().group().members());
        var inputStates = new ArrayDeque<ToDomain.StateRef>();
        var readStates = new ArrayDeque<ToDomain.StateRef>();
        var outputStates = new ArrayDeque<ToDomain.NewState>();
        for (var loadedAccount : loadedAccountStates.keySet()) {
            var inputState = loadedAccountStates.get(loadedAccount);
            var lastOp = committedUpdates.get(loadedAccount);
            if (lastOp == DynamicLoadWorldState.LastOpType.DELETED || lastOp == DynamicLoadWorldState.LastOpType.UPDATED) {
                if (inputState != null) {
                    inputStates.add(ToDomain.StateRef.newBuilder().
                            setSchemaId(inputState.getSchemaId()).
                            setId(inputState.getId()).
                            build());
                }
                if (lastOp == DynamicLoadWorldState.LastOpType.UPDATED) {
                    LOGGER.info("Writing new state for account {} (existing={})", loadedAccount, inputState);
                    var updatedAccount = evm.getWorld().get(loadedAccount);
                    outputStates.add(ToDomain.NewState.newBuilder().
                            setSchemaId(latestSchemaId).
                            setStateDataJsonBytes(ByteString.copyFrom(updatedAccount.serialize())).
                            addAllDistibutionList(lookups).
                            build());
                } else {
                    LOGGER.info("Deleting account {} (existing={})", loadedAccount, inputState);
                }
            } else if (loadedAccount != null) {
                // Note a read of an account with no state at this block is not tracked on-chain
                LOGGER.info("Read of state for account {} (existing={})", loadedAccount, inputState);
                readStates.add(ToDomain.StateRef.newBuilder().
                        setSchemaId(inputState.getSchemaId()).
                        setId(inputState.getId()).
                        build());
            }
        }
        result.addAllInputStates(inputStates);
        result.addAllReadStates(readStates);
        result.addAllOutputStates(outputStates);
        return result.build();
    }

    String getFrom() { return from; }

    long getBaseBlock() { return baseBlock; }

    static List<String> buildGroupScopeIdentityLookups(JsonHex.Bytes32 salt, String [] members) throws IllegalArgumentException {
        // Salt must be a 32byte hex string
        if (salt == null || members == null) throw new IllegalArgumentException("salt and members are required for group");
        var saltHex = salt.toHex();

        // To deploy a new Privacy Group we need to collect unique endorsement addresses that
        // mask the identities of all the participants.
        // We use the salt of the privacy group to do this. This salt is basically a shared
        // secret between all parties that is used to mask their identities.
        List<String> lookups = new ArrayList<>(members.length);
        for (String member : members) {
            String[] locatorSplit = member.split("@");
            switch (locatorSplit.length) {
                case 1 -> {
                    lookups.add(locatorSplit[0] + "." + saltHex);
                }
                case 2 -> {
                    lookups.add(locatorSplit[0] + "." + saltHex + "@" + locatorSplit[1]);
                }
                default -> throw new IllegalArgumentException("invalid identity locator '%s'".formatted(member));
            }
        }
        return lookups;
    }

    record EVMExecutionResult(
            EVMRunner evm,
            org.hyperledger.besu.datatypes.Address senderAddress,
            byte[] txPayload,
            JsonHex.Bytes32 txPayloadHash
    ) {}

    static class EVMExecutionException extends Exception { EVMExecutionException(String message) { super(message); } }

    EVMExecutionResult executeEVM(long chainId, Address fromAddr, AccountLoader accountLoader) throws IOException, ExecutionException, InterruptedException, ClassNotFoundException, EVMExecutionException {
        var evm = getEVM(chainId, getBaseBlock(), accountLoader);
        var senderAddress = org.hyperledger.besu.datatypes.Address.wrap(Bytes.wrap(fromAddr.getBytes()));
        var calldata = getEncodedCallData();
        var sender = evm.getWorld().getUpdater().getOrCreate(senderAddress);
        var nonce = sender.getNonce();
        sender.setNonce(nonce+1);
        MessageFrame execResult;
        if (getValues().to() == null) {
            execResult = evm.runContractDeploymentBytes(
                    senderAddress,
                    null,
                    Bytes.wrap(getValues().bytecode().getBytes()),
                    Bytes.wrap(calldata)
            );
        } else {
            execResult = evm.runContractInvokeBytes(
                    senderAddress,
                    org.hyperledger.besu.datatypes.Address.wrap(Bytes.wrap(getValues().to().getBytes())),
                    Bytes.wrap(calldata)
            );
        }
        if (execResult.getState() != MessageFrame.State.COMPLETED_SUCCESS) {
            throw new EVMExecutionException("transaction reverted: %s".formatted(execResult.getRevertReason()));
        }
        var txPayload = getEncodedTransaction(nonce, calldata);
        return new EVMExecutionResult(
                evm,
                senderAddress,
                txPayload,
                Keccak.Hash(txPayload)
        );
    }

    byte[] eip712TypedDataEndorsementPayload(List<String> inputs, List<String> reads, List<String> outputs) throws IOException, ExecutionException, InterruptedException {
        var typedDataRequest = new HashMap<String, Object>(){{
            put("types", new HashMap<String, Object>(){{
                put("Transition", new ArrayDeque<Map<String, Object>>(){{
                    add(new HashMap<>(){{put("name", "inputs"); put("type", "bytes32[]");}});
                    add(new HashMap<>(){{put("name", "reads"); put("type", "bytes32[]");}});
                    add(new HashMap<>(){{put("name", "outputs"); put("type", "bytes32[]");}});
                }});
                put("EIP712Domain", new ArrayDeque<Map<String, Object>>(){{
                    add(new HashMap<>(){{put("name", "name"); put("type", "string");}});
                    add(new HashMap<>(){{put("name", "version"); put("type", "string");}});
                    add(new HashMap<>(){{put("name", "chainId"); put("type", "uint256");}});
                    add(new HashMap<>(){{put("name", "verifyingContract"); put("type", "address");}});
                }});
            }});
            put("primaryType", "Transition");
            put("domain", new HashMap<>(){{
                put("name", "pente");
                put("version", "0.0.1");
                put("chainId", domain.getConfig().getChainId());
                put("verifyingContract", domain.getConfig().getAddress());
            }});
            put("message", new HashMap<>(){{
                put("inputs", inputs);
                put("reads", reads);
                put("outputs", outputs);
            }});
        }};
        var encoded = domain.encodeData(FromDomain.EncodeDataRequest.newBuilder().
                setEncodingType(FromDomain.EncodeDataRequest.EncodingType.TYPED_DATA_V4).
                setBody(new ObjectMapper().writeValueAsString(typedDataRequest)).
                build()).get();
        return encoded.getData().toByteArray();
    }

}
