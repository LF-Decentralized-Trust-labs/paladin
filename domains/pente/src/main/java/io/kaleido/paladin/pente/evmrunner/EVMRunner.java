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

package io.kaleido.paladin.pente.evmrunner;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import io.kaleido.paladin.pente.evmstate.AccountLoader;
import io.kaleido.paladin.pente.evmstate.DebugEVMTracer;
import io.kaleido.paladin.pente.evmstate.DynamicLoadWorldState;
import io.kaleido.paladin.pente.evmstate.VirtualBlockchain;
import io.kaleido.paladin.toolkit.JsonHex;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.tuweni.bytes.Bytes;
import org.bouncycastle.jcajce.provider.digest.Keccak;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.evm.Code;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;
import org.hyperledger.besu.evm.processor.ContractCreationProcessor;
import org.hyperledger.besu.evm.processor.MessageCallProcessor;
import org.hyperledger.besu.evm.tracing.OperationTracer;
import org.web3j.abi.FunctionEncoder;
import org.web3j.abi.FunctionReturnDecoder;
import org.web3j.abi.TypeReference;
import org.web3j.abi.Utils;
import org.web3j.abi.datatypes.Function;
import org.web3j.abi.datatypes.Type;
import org.web3j.rlp.RlpEncoder;
import org.web3j.rlp.RlpList;
import org.web3j.rlp.RlpString;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Deque;
import java.util.LinkedList;
import java.util.List;
import java.util.stream.Collectors;

public class EVMRunner {

    private static final Logger logger = LogManager.getLogger(EVMRunner.class);

    private final EVMVersion evmVersion;

    private final VirtualBlockchain virtualBlockchain;

    private final Address coinbase;

    public static Address randomAddress() {
        return Address.wrap(Bytes.random(20));
    }

    private final DynamicLoadWorldState world;

    public EVMRunner(EVMVersion evmVersion, AccountLoader accountLoader, long blockNumber) {
        this.evmVersion = evmVersion;
        this.coinbase = randomAddress();
        this.world = new DynamicLoadWorldState(accountLoader, evmVersion.evmConfiguration());
        this.virtualBlockchain = new VirtualBlockchain(blockNumber);
    }

    @SuppressWarnings("rawtypes")
    public MessageFrame runContractDeployment(
            Address sender,
            Address smartContractAddress,
            Bytes codeBytes,
            long initialGas,
            List<JsonEVMLog> logAccumulator,
            Type ...parameters
    ) {
        // Use web3j to encode the input data
        Bytes constructorParamsBytes = null;
        if (parameters.length > 0) {
            String constructorParamsHex = FunctionEncoder.encodeConstructor(List.of(parameters));
            constructorParamsBytes = Bytes.fromHexString(constructorParamsHex);
        }
        return runContractDeploymentBytes(sender, smartContractAddress, codeBytes, constructorParamsBytes, initialGas, logAccumulator);
    }

    public static Address nonceSmartContractAddress(Address address, long nonce) {
        var rlpBytes = RlpEncoder.encode(new RlpList(
                RlpString.create(address.toArray()),
                RlpString.create(nonce)
        ));
        var hash = new Keccak.Digest256().digest(rlpBytes);
        return Address.wrap(Bytes.wrap(Arrays.copyOfRange(hash, 12, hash.length)));
    }

    public MessageFrame runContractDeploymentBytes(
            Address senderAddress,
            Address smartContractAddress,
            Bytes codeBytes,
            Bytes constructorParamsBytes,
            long initialGas,
            List<JsonEVMLog> logAccumulator
    ) {
        if (constructorParamsBytes != null) {
            codeBytes = Bytes.wrap(codeBytes, constructorParamsBytes);
        }

        Code code = this.evmVersion.evm().getCode(Hash.hash(codeBytes), codeBytes);
        var sender = this.world.getUpdater().getOrCreate(senderAddress);
        if (smartContractAddress == null) {
            smartContractAddress = nonceSmartContractAddress(senderAddress, sender.getNonce());
        }

        // Build the message frame
        logger.debug("Deploying to={} from={} nonce={}", smartContractAddress, senderAddress, sender.getNonce());
        final MessageFrame frame =
                MessageFrame.builder()
                        .type(MessageFrame.Type.CONTRACT_CREATION)
                        .worldUpdater(this.world.getUpdater())
                        .initialGas(initialGas)
                        .originator(senderAddress)
                        .sender(senderAddress)
                        .address(smartContractAddress)
                        .contract(smartContractAddress)
                        .code(code)
                        .inputData(Bytes.EMPTY)
                        .gasPrice(Wei.ZERO)
                        .value(Wei.ZERO)
                        .apparentValue(Wei.ZERO)
                        .blockValues(virtualBlockchain)
                        .completer(c -> {})
                        .miningBeneficiary(coinbase)
                        .blockHashLookup(virtualBlockchain)
                        .maxStackSize(Integer.MAX_VALUE)
                        .build();
        this.runFrame(frame, logAccumulator);
        return frame;
    }


    public String methodSignature(Function function) {
        StringBuilder result = new StringBuilder();
        result.append(function.getName());
        result.append("(");
        String params = function.getInputParameters().stream().map(Type::getTypeAsString).collect(Collectors.joining(","));
        result.append(params);
        result.append(")");
        return result.toString();
    }

    @SuppressWarnings("rawtypes")
    public MessageFrame runContractInvoke(
            Address sender,
            Address smartContractAddress,
            String methodName,
            long initialGas,
            List<JsonEVMLog> logAccumulator,
            Type ...parameters
    ) {

        // Use web3j to encode the call data
        Function function = new Function(methodName, List.of(parameters), List.of());
        String callDataHex = FunctionEncoder.encode(function);
        return runContractInvokeBytes(sender, smartContractAddress, Bytes.fromHexString(callDataHex), initialGas, logAccumulator);
    }

    public MessageFrame runContractInvokeBytes(
            Address senderAddress,
            Address smartContractAddress,
            Bytes callData,
            long initialGas,
            List<JsonEVMLog> logAccumulator
    ) {
        var sender = this.world.getUpdater().getOrCreate(senderAddress);
        logger.debug("Invoking to={} from={} nonce={}", smartContractAddress, senderAddress, sender.getNonce());

        // Build the message frame
        var contractAccount = this.world.getUpdater().get(smartContractAddress);
        if (contractAccount == null) {
            throw new IllegalArgumentException("no contract deployed at %s".formatted(smartContractAddress));
        }
        Bytes codeBytes = contractAccount.getCode();
        Code code = this.evmVersion.evm().getCode(Hash.hash(codeBytes), codeBytes);
        final MessageFrame frame =
                MessageFrame.builder()
                        .type(MessageFrame.Type.MESSAGE_CALL)
                        .worldUpdater(this.world.getUpdater())
                        .initialGas(initialGas)
                        .originator(senderAddress)
                        .sender(senderAddress)
                        .address(smartContractAddress)
                        .contract(smartContractAddress)
                        .code(code)
                        .inputData(callData)
                        .gasPrice(Wei.ZERO)
                        .value(Wei.ZERO)
                        .apparentValue(Wei.ZERO)
                        .blockValues(virtualBlockchain)
                        .completer(c -> {})
                        .miningBeneficiary(coinbase)
                        .blockHashLookup(virtualBlockchain)
                        .maxStackSize(Integer.MAX_VALUE)
                        .completer(__ -> {})
                        .build();
        this.runFrame(frame, logAccumulator);
        return frame;
    }

    public List<Type<?>> decodeReturn( MessageFrame frame, List<TypeReference<?>> returns) {
        return FunctionReturnDecoder.decode(
                frame.getOutputData().toHexString(),
                Utils.convert(returns)).stream().map(r ->
                (Type<?>)(r)
        ).collect(Collectors.toList());

    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record JsonEVMLog(
            @JsonProperty()
            JsonHex.Address address,
            @JsonProperty()
            List<JsonHex.Bytes32> topics,
            @JsonProperty()
            JsonHex.Bytes data
    ) {}

    public void runFrame(MessageFrame initialFrame, List<JsonEVMLog> logAccumulator) {
        final OperationTracer tracer = new DebugEVMTracer();
        Deque<MessageFrame> messageFrameStack = initialFrame.getMessageFrameStack();
        final PrecompileContractRegistry precompileContractRegistry = new PrecompileContractRegistry();
        final MessageCallProcessor mcp = new MessageCallProcessor(this.evmVersion.evm(), precompileContractRegistry);
        final ContractCreationProcessor ccp =
                new ContractCreationProcessor(
                        this.evmVersion.gasCalculator(),
                        this.evmVersion.evm(),
                        false,
                        List.of(),
                        0);

        while (!messageFrameStack.isEmpty()) {
            final MessageFrame messageFrame = messageFrameStack.peek();
            switch (messageFrame.getType()) {
                case CONTRACT_CREATION -> ccp.process(messageFrame, tracer);
                case MESSAGE_CALL -> mcp.process(messageFrame, tracer);
            }

            logger.debug("Frame {}/{} completed: {}", messageFrame.getType(),  Integer.toHexString(messageFrame.hashCode()), messageFrame.getState());

            if (messageFrame.getExceptionalHaltReason().isPresent()) {
                logger.debug(messageFrame.getExceptionalHaltReason().get().toString());
            }
            if (messageFrame.getRevertReason().isPresent()) {
                logger.debug(
                        new String(
                                messageFrame.getRevertReason().get().toArrayUnsafe(), StandardCharsets.UTF_8));
            }

            var frameLogs = messageFrame.getLogs();
            if (frameLogs != null) {
                logAccumulator.addAll(
                    frameLogs.stream().map(l -> new JsonEVMLog(
                        new JsonHex.Address(messageFrame.getContractAddress().toArray()),
                        l.getTopics().stream().map(t -> new JsonHex.Bytes32(t.toArray())).toList(),
                        new JsonHex.Bytes(l.getData().toArray())
                    )).toList()
                );
            }
        }
    }

    public DynamicLoadWorldState getWorld() {
        return world;
    }
}
