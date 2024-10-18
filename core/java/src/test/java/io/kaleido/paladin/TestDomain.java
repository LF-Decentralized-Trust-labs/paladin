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

import io.kaleido.paladin.toolkit.ToDomain;
import io.kaleido.paladin.toolkit.DomainBase;
import io.kaleido.paladin.toolkit.DomainInstance;

import java.util.concurrent.CompletableFuture;

public class TestDomain extends DomainInstance {
    TestDomain(String grpcTarget, String instanceId) {
        super(grpcTarget, instanceId);
        init();
    }

    @Override
    protected CompletableFuture<ToDomain.ConfigureDomainResponse> configureDomain(ToDomain.ConfigureDomainRequest request) {
        ToDomain.DomainConfig domainConfig = ToDomain.DomainConfig.newBuilder()
                .setBaseLedgerSubmitConfig(ToDomain.BaseLedgerSubmitConfig.newBuilder()
                        .setSubmitMode(ToDomain.BaseLedgerSubmitConfig.Mode.ONE_TIME_USE_KEYS)
                        .build())
                .build();
        return CompletableFuture.completedFuture(ToDomain.ConfigureDomainResponse.newBuilder()
                .setDomainConfig(domainConfig)
                .build()
        );
    }

    @Override
    protected CompletableFuture<ToDomain.InitDomainResponse> initDomain(ToDomain.InitDomainRequest request) {
        return CompletableFuture.completedFuture(ToDomain.InitDomainResponse.getDefaultInstance());
    }

    @Override
    protected CompletableFuture<ToDomain.InitDeployResponse> initDeploy(ToDomain.InitDeployRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareDeployResponse> prepareDeploy(ToDomain.PrepareDeployRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.InitTransactionResponse> initTransaction(ToDomain.InitTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.AssembleTransactionResponse> assembleTransaction(ToDomain.AssembleTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.EndorseTransactionResponse> endorseTransaction(ToDomain.EndorseTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareTransactionResponse> prepareTransaction(ToDomain.PrepareTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.HandleEventBatchResponse> handleEventBatch(ToDomain.HandleEventBatchRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.SignResponse> sign(ToDomain.SignRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.GetVerifierResponse> getVerifier(ToDomain.GetVerifierRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.ValidateStateHashesResponse> validateStateHashes(ToDomain.ValidateStateHashesRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }
}
