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

package io.kaleido.paladin;

import io.kaleido.paladin.toolkit.*;

import java.util.concurrent.CompletableFuture;

public class TestDomain extends DomainInstance {
    TestDomain(String grpcTarget, String instanceId) {
        super(grpcTarget, instanceId);
        init();
    }

    @Override
    protected CompletableFuture<ConfigureDomainResponse> configureDomain(ConfigureDomainRequest request) {
        DomainConfig domainConfig = DomainConfig.newBuilder()
                .build();
        return CompletableFuture.completedFuture(ConfigureDomainResponse.newBuilder()
                .setDomainConfig(domainConfig)
                .build()
        );
    }

    @Override
    protected CompletableFuture<InitDomainResponse> initDomain(InitDomainRequest request) {
        return CompletableFuture.completedFuture(InitDomainResponse.getDefaultInstance());
    }

    @Override
    protected CompletableFuture<InitDeployResponse> initDeploy(InitDeployRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<PrepareDeployResponse> prepareDeploy(PrepareDeployRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<InitContractResponse> initContract(InitContractRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<InitTransactionResponse> initTransaction(InitTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<AssembleTransactionResponse> assembleTransaction(AssembleTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<EndorseTransactionResponse> endorseTransaction(EndorseTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<PrepareTransactionResponse> prepareTransaction(PrepareTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<HandleEventBatchResponse> handleEventBatch(HandleEventBatchRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<SignResponse> sign(SignRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<GetVerifierResponse> getVerifier(GetVerifierRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ValidateStateHashesResponse> validateStateHashes(ValidateStateHashesRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<InitCallResponse> initCall(InitCallRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ExecCallResponse> execCall(ExecCallRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<BuildReceiptResponse> buildReceipt(BuildReceiptRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ConfigurePrivacyGroupResponse> configurePrivacyGroup(ConfigurePrivacyGroupRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<InitPrivacyGroupResponse> initPrivacyGroup(InitPrivacyGroupRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<WrapPrivacyGroupEVMTXResponse> wrapPrivacyGroupTransaction(WrapPrivacyGroupEVMTXRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }
}
