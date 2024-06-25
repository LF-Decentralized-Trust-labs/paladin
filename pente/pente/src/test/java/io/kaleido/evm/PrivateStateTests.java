/*
 *  © Copyright Kaleido, Inc. 2024. The materials in this file constitute the "Pre-Existing IP,"
 *  "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 *  under a limited, perpetual license only, subject to the terms of the applicable license
 *  agreement between you and Kaleido, Inc.  All other rights reserved.
 */

package io.kaleido.evm;

import org.hyperledger.besu.evm.MainnetEVMs;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.hyperledger.besu.evm.EVM;

import java.math.BigInteger;
import java.util.Random;

public class PrivateStateTests {

    @Test
    void runAnEVM() {

        long chainId = new Random().nextLong();
        EVM evm = MainnetEVMs.shanghai(
                BigInteger.valueOf(chainId),
                EvmConfiguration.DEFAULT
        );



    }
}
