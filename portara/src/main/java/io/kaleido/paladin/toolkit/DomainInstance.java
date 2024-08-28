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

package io.kaleido.paladin.toolkit;

import github.com.kaleido_io.paladin.toolkit.FromDomain;
import github.com.kaleido_io.paladin.toolkit.Service;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.grpc.stub.StreamObserver;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.util.concurrent.CompletableFuture;

public abstract class DomainInstance extends PluginInstance<Service.DomainMessage> {

    private static final Logger LOGGER = LogManager.getLogger(DomainInstance.class);

    protected abstract ToDomain.ConfigureDomainResponse configureDomain(ToDomain.ConfigureDomainRequest request);
    protected abstract ToDomain.InitDomainResponse initDomain(ToDomain.InitDomainRequest request);
    protected abstract ToDomain.InitDeployResponse initDeploy(ToDomain.InitDeployRequest request);
    protected abstract ToDomain.PrepareDeployResponse prepareDeploy(ToDomain.PrepareDeployRequest request);
    protected abstract ToDomain.InitTransactionResponse initTransaction(ToDomain.InitTransactionRequest request);
    protected abstract ToDomain.AssembleTransactionResponse assembleTransaction(ToDomain.AssembleTransactionRequest request);
    protected abstract ToDomain.EndorseTransactionResponse endorseTransaction(ToDomain.EndorseTransactionRequest request);
    protected abstract ToDomain.PrepareTransactionResponse prepareTransaction(ToDomain.PrepareTransactionRequest request);

    protected DomainInstance(String grpcTarget, String instanceId) {
        super(grpcTarget, instanceId);
    }

    protected CompletableFuture<FromDomain.FindAvailableStatesResponse> findAvailableStates(FromDomain.FindAvailableStatesRequest request) {
        Service.DomainMessage message = Service.DomainMessage.newBuilder().setHeader(newRequestHeader()).build();
        return requestReply(message).thenApply(Service.DomainMessage::getFindAvailableStatesRes);
    }

    @Override
    final StreamObserver<Service.DomainMessage> connect(StreamObserver<Service.DomainMessage> observer) {
        return stub.connectDomain(observer);
    }

    @Override
    final Service.Header getHeader(Service.DomainMessage domainMessage) {
        return domainMessage.getHeader();
    }

    @Override
    final Service.DomainMessage buildMessage(Service.Header header) {
        return Service.DomainMessage.newBuilder().setHeader(header).build();
    }

    @Override
    final void handleRequest(Service.DomainMessage request) {
        Service.DomainMessage.Builder response = Service.DomainMessage.newBuilder();
        try {
            switch (request.getRequestToDomainCase()) {
                case CONFIGURE_DOMAIN -> response.setConfigureDomainRes(configureDomain(request.getConfigureDomain()));
                case INIT_DOMAIN -> response.setInitDomainRes(initDomain(request.getInitDomain()));
                case INIT_DEPLOY -> response.setInitDeployRes(initDeploy(request.getInitDeploy()));
                case PREPARE_DEPLOY -> response.setPrepareDeployRes(prepareDeploy(request.getPrepareDeploy()));
                case INIT_TRANSACTION -> response.setInitTransactionRes(initTransaction(request.getInitTransaction()));
                case ASSEMBLE_TRANSACTION ->
                        response.setAssembleTransactionRes(assembleTransaction(request.getAssembleTransaction()));
                case ENDORSE_TRANSACTION ->
                        response.setEndorseTransactionRes(endorseTransaction(request.getEndorseTransaction()));
                case PREPARE_TRANSACTION ->
                        response.setPrepareTransactionRes(prepareTransaction(request.getPrepareTransaction()));
                default -> throw new IllegalArgumentException("unknown request: %s".formatted(request.getRequestToDomainCase()));
            }
            response.setHeader(getReplyHeader(request));
        } catch(Exception e) {
            LOGGER.error(new FormattedMessage("unable to process {} {}", request.getRequestToDomainCase(), request.getHeader().getMessageId()), e);
            response.setHeader(getErrorReplyHeader(request, e));
        }
        send(response.build());
    }

}
