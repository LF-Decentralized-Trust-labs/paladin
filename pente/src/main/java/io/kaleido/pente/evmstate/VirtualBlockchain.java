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

package io.kaleido.pente.evmstate;

import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.Bytes32;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.datatypes.Wei;
import org.hyperledger.besu.evm.frame.BlockValues;
import org.hyperledger.besu.evm.operation.BlockHashOperation;

import java.util.Optional;

public class VirtualBlockchain implements BlockValues, BlockHashOperation.BlockHashLookup {

    private long blockNumber;
    private long blockTimestamp;
    private long gasLimit;

    public VirtualBlockchain(long blockNumber) {
        this.blockNumber = blockNumber;
        this.blockTimestamp = System.currentTimeMillis() / 1000;
        this.gasLimit = Long.MAX_VALUE; // TODO: consider right answer here
    }

    @Override
    public Bytes getDifficultyBytes() {
        return null;
    }

    @Override
    public Bytes32 getMixHashOrPrevRandao() {
        return null;
    }

    @Override
    public Optional<Wei> getBaseFee() {
        return null;
    }

    @Override
    public long getNumber() {
        return this.blockNumber;
    }

    @Override
    public long getTimestamp() {
        return this.gasLimit;
    }

    @Override
    public long getGasLimit() {
        return this.blockTimestamp;
    }

    @Override
    public Hash apply(Long aLong) {
        return null; // TODO: lookup blocks
    }
}
