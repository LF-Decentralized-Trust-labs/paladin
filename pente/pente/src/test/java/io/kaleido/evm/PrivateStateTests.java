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

import io.kaleido.pente.evmrunner.EVMRunner;
import io.kaleido.pente.evmrunner.EVMVersion;
import org.apache.commons.io.IOUtils;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.junit.jupiter.api.Test;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.web3j.abi.FunctionReturnDecoder;
import org.web3j.abi.TypeReference;
import org.web3j.abi.Utils;
import org.web3j.abi.datatypes.Function;
import org.web3j.abi.datatypes.Type;
import org.web3j.abi.datatypes.generated.Uint256;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
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
        EVMRunner evmRunner = new EVMRunner(EVMVersion.Shanghai(new Random().nextLong(), EvmConfiguration.DEFAULT), 0);

        // Load some bytecode for our first contract deploy
        String hexByteCode;
        try (InputStream is = Thread.currentThread().getContextClassLoader().getResourceAsStream("solidity/SimpleStorage.bin")) {
            assertNotNull(is);
            hexByteCode = IOUtils.toString(is, StandardCharsets.UTF_8);
        }
        final Address smartContractAddress = EVMRunner.randomAddress();
        final Address sender = EVMRunner.randomAddress();
        MessageFrame deployFrame = evmRunner.runContractDeployment(
                sender,
                smartContractAddress,
                Bytes.fromHexString(hexByteCode),
                new Uint256(12345)
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, deployFrame.getState());
        MessageFrame setFrame = evmRunner.runContractInvoke(
                sender,
                smartContractAddress,
                "set",
                new Uint256(23456)
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, setFrame.getState());
        MessageFrame getFrame = evmRunner.runContractInvoke(
                sender,
                smartContractAddress,
                "get"
        );
        assertEquals(MessageFrame.State.COMPLETED_SUCCESS, getFrame.getState());
        List<Type<?>> returns = evmRunner.decodeReturn(getFrame, List.of(new TypeReference<Uint256>() {}));
        assertEquals(23456, ((Uint256)(returns.getFirst())).getValue().intValue());
    }
}
