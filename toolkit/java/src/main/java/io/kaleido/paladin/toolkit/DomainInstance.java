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

import io.grpc.stub.StreamObserver;
import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.util.concurrent.CompletableFuture;

public abstract class DomainInstance extends PluginInstance<Service.DomainMessage> {

    private static final Logger LOGGER = PaladinLogging.getLogger(DomainInstance.class);

    protected abstract CompletableFuture<ToDomain.ConfigureDomainResponse> configureDomain(ToDomain.ConfigureDomainRequest request);
    protected abstract CompletableFuture<ToDomain.InitDomainResponse> initDomain(ToDomain.InitDomainRequest request);
    protected abstract CompletableFuture<ToDomain.InitDeployResponse> initDeploy(ToDomain.InitDeployRequest request);
    protected abstract CompletableFuture<ToDomain.PrepareDeployResponse> prepareDeploy(ToDomain.PrepareDeployRequest request);
    protected abstract CompletableFuture<ToDomain.InitContractResponse> initContract(ToDomain.InitContractRequest request);
    protected abstract CompletableFuture<ToDomain.InitTransactionResponse> initTransaction(ToDomain.InitTransactionRequest request);
    protected abstract CompletableFuture<ToDomain.AssembleTransactionResponse> assembleTransaction(ToDomain.AssembleTransactionRequest request);
    protected abstract CompletableFuture<ToDomain.EndorseTransactionResponse> endorseTransaction(ToDomain.EndorseTransactionRequest request);
    protected abstract CompletableFuture<ToDomain.PrepareTransactionResponse> prepareTransaction(ToDomain.PrepareTransactionRequest request);
    protected abstract CompletableFuture<ToDomain.HandleEventBatchResponse> handleEventBatch(ToDomain.HandleEventBatchRequest request);
    protected abstract CompletableFuture<ToDomain.SignResponse> sign(ToDomain.SignRequest request);
    protected abstract CompletableFuture<ToDomain.GetVerifierResponse> getVerifier(ToDomain.GetVerifierRequest request);
    protected abstract CompletableFuture<ToDomain.ValidateStateHashesResponse> validateStateHashes(ToDomain.ValidateStateHashesRequest request);
    protected abstract CompletableFuture<ToDomain.InitCallResponse> initCall(ToDomain.InitCallRequest request);
    protected abstract CompletableFuture<ToDomain.ExecCallResponse> execCall(ToDomain.ExecCallRequest request);
    protected abstract CompletableFuture<ToDomain.BuildReceiptResponse> buildReceipt(ToDomain.BuildReceiptRequest request);

    protected DomainInstance(String grpcTarget, String instanceId) {
        super(grpcTarget, instanceId);
    }

    public CompletableFuture<FromDomain.FindAvailableStatesResponse> findAvailableStates(FromDomain.FindAvailableStatesRequest request) {
        Service.DomainMessage message = Service.DomainMessage.newBuilder().
                setHeader(newRequestHeader()).
                setFindAvailableStates(request).
                build();
        return requestReply(message).thenApply(Service.DomainMessage::getFindAvailableStatesRes);
    }

    public CompletableFuture<FromDomain.EncodeDataResponse> encodeData(FromDomain.EncodeDataRequest request) {
        Service.DomainMessage message = Service.DomainMessage.newBuilder().
                setHeader(newRequestHeader()).
                setEncodeData(request).
                build();
        return requestReply(message).thenApply(Service.DomainMessage::getEncodeDataRes);
    }

    public CompletableFuture<FromDomain.DecodeDataResponse> decodeData(FromDomain.DecodeDataRequest request) {
        Service.DomainMessage message = Service.DomainMessage.newBuilder().
                setHeader(newRequestHeader()).
                setDecodeData(request).
                build();
        return requestReply(message).thenApply(Service.DomainMessage::getDecodeDataRes);
    }

    public CompletableFuture<FromDomain.RecoverSignerResponse> recoverSigner(FromDomain.RecoverSignerRequest request) {
        Service.DomainMessage message = Service.DomainMessage.newBuilder().
                setHeader(newRequestHeader()).
                setRecoverSigner(request).
                build();
        return requestReply(message).thenApply(Service.DomainMessage::getRecoverSignerRes);
    }

    @Override
    final StreamObserver<Service.DomainMessage> connect(StreamObserver<Service.DomainMessage> observer) {
        LOGGER.info("connecting domain gRPC to Paladin");
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
    final CompletableFuture<Service.DomainMessage> handleRequest(Service.DomainMessage request) {
        LOGGER.info("JAVA_PLUGIN_REQUEST - pluginId={} type={} msgId={}", pluginId, request.getRequestToDomainCase().toString(), request.getHeader().getMessageId());
        Service.DomainMessage.Builder response = Service.DomainMessage.newBuilder();
        try {
            CompletableFuture<?> resultApplied = switch (request.getRequestToDomainCase()) {
                case CONFIGURE_DOMAIN -> configureDomain(request.getConfigureDomain()).thenApply(response::setConfigureDomainRes);
                case INIT_DOMAIN -> initDomain(request.getInitDomain()).thenApply(response::setInitDomainRes);
                case INIT_DEPLOY -> initDeploy(request.getInitDeploy()).thenApply(response::setInitDeployRes);
                case PREPARE_DEPLOY -> prepareDeploy(request.getPrepareDeploy()).thenApply(response::setPrepareDeployRes);
                case INIT_CONTRACT -> initContract(request.getInitContract()).thenApply(response::setInitContractRes);
                case INIT_TRANSACTION -> initTransaction(request.getInitTransaction()).thenApply(response::setInitTransactionRes);
                case ASSEMBLE_TRANSACTION -> assembleTransaction(request.getAssembleTransaction()).thenApply(response::setAssembleTransactionRes);
                case ENDORSE_TRANSACTION -> endorseTransaction(request.getEndorseTransaction()).thenApply(response::setEndorseTransactionRes);
                case PREPARE_TRANSACTION -> prepareTransaction(request.getPrepareTransaction()).thenApply(response::setPrepareTransactionRes);
                case HANDLE_EVENT_BATCH -> handleEventBatch(request.getHandleEventBatch()).thenApply(response::setHandleEventBatchRes);
                case SIGN -> sign(request.getSign()).thenApply(response::setSignRes);
                case GET_VERIFIER -> getVerifier(request.getGetVerifier()).thenApply(response::setGetVerifierRes);
                case VALIDATE_STATE_HASHES -> validateStateHashes(request.getValidateStateHashes()).thenApply(response::setValidateStateHashesRes);
                case INIT_CALL -> initCall(request.getInitCall()).thenApply(response::setInitCallRes);
                case EXEC_CALL -> execCall(request.getExecCall()).thenApply(response::setExecCallRes);
                case BUILD_RECEIPT -> buildReceipt(request.getBuildReceipt()).thenApply(response::setBuildReceiptRes);
                default -> throw new IllegalArgumentException("unknown request: %s".formatted(request.getRequestToDomainCase()));
            };
            return resultApplied.thenApply((ra) -> {
                response.setHeader(getReplyHeader(request));
                var builtResponse = response.build();
                LOGGER.info("JAVA_PLUGIN_RESPONSE - pluginId={} type={} msgId={}", pluginId, request.getResponseFromDomainCase().toString(), request.getHeader().getMessageId());
                return builtResponse;
            });
        } catch(Exception e) {
            LOGGER.error(new FormattedMessage("JAVA_PLUGIN_ERROR - pluginId={} type={} msgId={}", pluginId, request.getRequestToDomainCase(), request.getHeader().getMessageId()), e);
            return CompletableFuture.failedFuture(e);
        }
    }

}
