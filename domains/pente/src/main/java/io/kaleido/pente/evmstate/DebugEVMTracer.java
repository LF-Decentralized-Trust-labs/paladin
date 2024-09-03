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
import org.hyperledger.besu.datatypes.Transaction;
import org.hyperledger.besu.evm.frame.ExceptionalHaltReason;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.log.Log;
import org.hyperledger.besu.evm.operation.Operation;
import org.hyperledger.besu.evm.tracing.OperationTracer;
import org.hyperledger.besu.evm.worldstate.WorldView;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.List;
import java.util.Optional;

public class DebugEVMTracer implements OperationTracer {

    private final Logger logger = LoggerFactory.getLogger(DebugEVMTracer.class);

    @Override
    public void tracePreExecution(MessageFrame frame) {
        logger.trace("tracePreExecution: op={}", frame.getCurrentOperation().getName());
    }

    @Override
    public void tracePostExecution(MessageFrame frame, Operation.OperationResult operationResult) {
        logger.trace("tracePostExecution: op={} cost={}", frame.getCurrentOperation().getName(), operationResult.getGasCost());
    }

    @Override
    public void tracePrecompileCall(MessageFrame frame, long gasRequirement, Bytes output) {
        logger.trace("tracePrecompileCall: frame={} gasRequirement={} output={}", frame, gasRequirement, output);
    }

    @Override
    public void traceAccountCreationResult(MessageFrame frame, Optional<ExceptionalHaltReason> haltReason) {
        logger.trace("traceAccountCreationResult: frame={} haltReason={}", frame, haltReason);
    }

    @Override
    public void tracePrepareTransaction(WorldView worldView, Transaction transaction) {
        logger.trace("tracePrepareTransaction: worldView={} transaction={}", worldView, transaction);
    }

    @Override
    public void traceStartTransaction(WorldView worldView, Transaction transaction) {
        logger.trace("traceStartTransaction: worldView={} transaction={}", worldView, transaction);
    }

    @Override
    public void traceEndTransaction(WorldView worldView, Transaction transaction, boolean status, Bytes output, List<Log> logs, long gasUsed, long timeNs) {
        logger.trace("traceEndTransaction: worldView={} transaction={} status={} output={} logs={} gasUsed={} timens={}", worldView, transaction, status, output, logs, gasUsed, timeNs);
    }

    @Override
    public void traceContextEnter(MessageFrame frame) {
        logger.trace("traceContextEnter: frame={}", frame);
    }

    @Override
    public void traceContextReEnter(MessageFrame frame) {
        logger.trace("traceContextReEnter: frame={}", frame);
    }

    @Override
    public void traceContextExit(MessageFrame frame) {
        logger.trace("traceContextExit: frame={}", frame);
    }

    @Override
    public boolean isExtendedTracing() {
        return true;
    }
}
