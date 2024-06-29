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

package io.kaleido.pente.evmrunner;

import io.kaleido.pente.evmstate.DebugEVMTracer;
import io.kaleido.pente.evmstate.InMemoryWorldState;
import io.kaleido.pente.evmstate.InMemoryWorldStateUpdater;
import io.kaleido.pente.evmstate.VirtualBlockchain;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.evm.Code;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;
import org.hyperledger.besu.evm.processor.ContractCreationProcessor;
import org.hyperledger.besu.evm.processor.MessageCallProcessor;
import org.hyperledger.besu.evm.tracing.OperationTracer;
import org.hyperledger.besu.evm.worldstate.WorldUpdater;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.web3j.abi.FunctionEncoder;
import org.web3j.abi.FunctionReturnDecoder;
import org.web3j.abi.TypeReference;
import org.web3j.abi.Utils;
import org.web3j.abi.datatypes.Function;
import org.web3j.abi.datatypes.Type;
import org.web3j.abi.datatypes.generated.Uint256;

import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.util.Deque;
import java.util.List;
import java.util.stream.Collectors;
import java.util.stream.Stream;

public class EVMRunner {

    private final Logger logger = LoggerFactory.getLogger(EVMRunner.class);

    private final EVMVersion evmVersion;

    private final WorldUpdater worldUpdater;

    private final VirtualBlockchain virtualBlockchain;

    private final Address coinbase;

    public static Address randomAddress() {
        return Address.wrap(Bytes.random(20));
    }

    public EVMRunner(EVMVersion evmVersion, long blockNumber) {
        this.evmVersion = evmVersion;
        this.coinbase = randomAddress();

       this.worldUpdater = new InMemoryWorldStateUpdater(
                new InMemoryWorldState(),
                evmVersion.evmConfiguration());
       this.virtualBlockchain = new VirtualBlockchain(blockNumber);
    }

    @SuppressWarnings("rawtypes")
    public MessageFrame runContractDeployment(
            Address sender,
            Address smartContractAddress,
            Bytes codeBytes,
            Type ...parameters
    ) {

        // Use web3j to encode the input data
        if (parameters.length > 0) {
            String constructorParamsHex = FunctionEncoder.encodeConstructor(List.of(parameters));
            Bytes constructorParamsBytes = Bytes.fromHexString(constructorParamsHex);
            codeBytes = Bytes.wrap(codeBytes, constructorParamsBytes);
        }
        Code code = this.evmVersion.evm().getCode(Hash.hash(codeBytes), codeBytes);
        this.worldUpdater.getOrCreate(sender);

        // Build the message frame
        final MessageFrame frame =
                MessageFrame.builder()
                        .type(MessageFrame.Type.CONTRACT_CREATION)
                        .worldUpdater(worldUpdater)
                        .initialGas(100000)
                        .originator(sender)
                        .sender(sender)
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
        logger.debug("Running contract deployment from {} to contract address {}", sender, smartContractAddress);
        this.runFrame(frame);
        return frame;
    }


    @SuppressWarnings("rawtypes")
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
            Type ...parameters
    ) {

        // Use web3j to encode the call data
        Function function = new Function(methodName, List.of(parameters), List.of());
        String callDataHex = FunctionEncoder.encode(function);
        this.worldUpdater.getOrCreate(sender);

        // Build the message frame
        Bytes codeBytes = this.worldUpdater.get(smartContractAddress).getCode();
        Code code = this.evmVersion.evm().getCode(Hash.hash(codeBytes), codeBytes);
        final MessageFrame frame =
                MessageFrame.builder()
                        .type(MessageFrame.Type.MESSAGE_CALL)
                        .worldUpdater(worldUpdater)
                        .initialGas(100000)
                        .originator(sender)
                        .sender(sender)
                        .address(smartContractAddress)
                        .contract(smartContractAddress)
                        .code(code)
                        .inputData(Bytes.fromHexString(callDataHex))
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
        logger.debug("Invoking {} from {} to contract address {}", methodSignature(function), sender, smartContractAddress);
        this.runFrame(frame);
        return frame;
    }

    @SuppressWarnings("rawtypes")
    public List<Type<?>> decodeReturn( MessageFrame frame, List<TypeReference<?>> returns) {
        return FunctionReturnDecoder.decode(
                frame.getOutputData().toHexString(),
                Utils.convert(returns)).stream().map(r ->
                (Type<?>)(r)
        ).collect(Collectors.toList());

    }

    public void runFrame(MessageFrame initialFrame) {
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

            if (messageFrame.getExceptionalHaltReason().isPresent()) {
                logger.debug(messageFrame.getExceptionalHaltReason().get().toString());
            }
            if (messageFrame.getRevertReason().isPresent()) {
                logger.debug(
                        new String(
                                messageFrame.getRevertReason().get().toArrayUnsafe(), StandardCharsets.UTF_8));
            }
        }
    }

}
