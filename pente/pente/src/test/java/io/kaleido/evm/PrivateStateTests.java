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

package io.kaleido.evm;

import io.kaleido.pente.evmstate.DebugEVMTracer;
import io.kaleido.pente.evmstate.InMemoryWorldState;
import io.kaleido.pente.evmstate.InMemoryWorldStateUpdater;
import io.kaleido.pente.evmstate.VirtualBlockchain;
import org.apache.commons.io.IOUtils;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.evm.Code;
import org.hyperledger.besu.evm.EVM;
import org.hyperledger.besu.evm.MainnetEVMs;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.gascalculator.ShanghaiGasCalculator;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.precompile.PrecompileContractRegistry;
import org.hyperledger.besu.evm.processor.ContractCreationProcessor;
import org.hyperledger.besu.evm.processor.MessageCallProcessor;
import org.hyperledger.besu.evm.tracing.OperationTracer;
import org.hyperledger.besu.evm.worldstate.WorldUpdater;
import org.junit.jupiter.api.Test;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.web3j.abi.FunctionEncoder;
import org.web3j.abi.datatypes.generated.Uint256;

import java.io.IOException;
import java.io.InputStream;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.util.Deque;
import java.util.List;
import java.util.Random;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

public class PrivateStateTests {

    private final Logger logger = LoggerFactory.getLogger(PrivateStateTests.class);

    Address randomAddress() {
        return Address.wrap(Bytes.random(20));
    }

    @Test
    void runAnEVM() throws IOException {

        // Generate a shiny new EVM
        EvmConfiguration evmConfiguration = EvmConfiguration.DEFAULT;
        long chainId = new Random().nextLong();
        EVM evm = MainnetEVMs.shanghai(
                BigInteger.valueOf(chainId),
                evmConfiguration
        );
        ShanghaiGasCalculator gasCalculator = new ShanghaiGasCalculator();

        // Load some bytecode for our first contract deploy
        String hexByteCode;
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream("solidity/SimpleStorage.bin")) {
            assertNotNull(is);
            hexByteCode = IOUtils.toString(is, StandardCharsets.UTF_8);
        }

        WorldUpdater worldUpdater = new InMemoryWorldStateUpdater(
                new InMemoryWorldState(),
                evmConfiguration);
        VirtualBlockchain virtualBlockchain = new VirtualBlockchain(0 );

        // Use web3j to encode some input data
        String constructorParamsHex = FunctionEncoder.encodeConstructor(List.of(new Uint256(12345)));
        Bytes constructorParamsBytes = Bytes.fromHexString(constructorParamsHex);

        // Process the code
        Bytes codeBytes = Bytes.fromHexString(hexByteCode);

        // Add constructor params to end of code
        codeBytes = Bytes.wrap(codeBytes, constructorParamsBytes);
        Code code = evm.getCode(Hash.hash(codeBytes), codeBytes);

        // Setup the addresses
        Address sender = randomAddress();
        Address receiver = randomAddress();
        worldUpdater.getOrCreate(sender).setBalance(Wei.of(BigInteger.TWO.pow(20)));
        worldUpdater.getOrCreate(receiver).setCode(codeBytes);

        // Construct a contract deployment message
        final MessageFrame frame =
                MessageFrame.builder()
                        .type(MessageFrame.Type.CONTRACT_CREATION)
                        .worldUpdater(worldUpdater)
                        .initialGas(100000)
                        .address(receiver)
                        .originator(sender)
                        .sender(sender)
                        .gasPrice(Wei.ZERO)
                        .inputData(Bytes.EMPTY)
                        .value(Wei.ZERO)
                        .apparentValue(Wei.ZERO)
                        .contract(Address.ZERO)
                        .code(code)
                        .blockValues(virtualBlockchain)
                        .completer(c -> {})
                        .miningBeneficiary(randomAddress())
                        .blockHashLookup(virtualBlockchain)
                        .maxStackSize(Integer.MAX_VALUE)
                        .build();

        final OperationTracer tracer = new DebugEVMTracer();
        Deque<MessageFrame> messageFrameStack = frame.getMessageFrameStack();
        final PrecompileContractRegistry precompileContractRegistry = new PrecompileContractRegistry();
        final MessageCallProcessor mcp = new MessageCallProcessor(evm, precompileContractRegistry);
        final ContractCreationProcessor ccp =
                new ContractCreationProcessor(evm.getGasCalculator(), evm, false, List.of(), 0);

        logger.debug("Running contract deployment");
        while (!messageFrameStack.isEmpty()) {
            final MessageFrame messageFrame = messageFrameStack.peek();
            switch (messageFrame.getType()) {
                case CONTRACT_CREATION -> ccp.process(messageFrame, tracer);
                case MESSAGE_CALL -> mcp.process(messageFrame, tracer);
            }

            assertEquals(MessageFrame.State.COMPLETED_SUCCESS, messageFrame.getState());
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
