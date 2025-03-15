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
 
 public abstract class DomainInstance extends PluginInstance<DomainMessage> {
 
     private static final Logger LOGGER = PaladinLogging.getLogger(DomainInstance.class);
 
     protected abstract CompletableFuture<ConfigureDomainResponse> configureDomain(ConfigureDomainRequest request);
     protected abstract CompletableFuture<InitDomainResponse> initDomain(InitDomainRequest request);
     protected abstract CompletableFuture<InitDeployResponse> initDeploy(InitDeployRequest request);
     protected abstract CompletableFuture<PrepareDeployResponse> prepareDeploy(PrepareDeployRequest request);
     protected abstract CompletableFuture<InitContractResponse> initContract(InitContractRequest request);
     protected abstract CompletableFuture<InitTransactionResponse> initTransaction(InitTransactionRequest request);
     protected abstract CompletableFuture<AssembleTransactionResponse> assembleTransaction(AssembleTransactionRequest request);
     protected abstract CompletableFuture<EndorseTransactionResponse> endorseTransaction(EndorseTransactionRequest request);
     protected abstract CompletableFuture<PrepareTransactionResponse> prepareTransaction(PrepareTransactionRequest request);
     protected abstract CompletableFuture<HandleEventBatchResponse> handleEventBatch(HandleEventBatchRequest request);
     protected abstract CompletableFuture<SignResponse> sign(SignRequest request);
     protected abstract CompletableFuture<GetVerifierResponse> getVerifier(GetVerifierRequest request);
     protected abstract CompletableFuture<ValidateStateHashesResponse> validateStateHashes(ValidateStateHashesRequest request);
     protected abstract CompletableFuture<InitCallResponse> initCall(InitCallRequest request);
     protected abstract CompletableFuture<ExecCallResponse> execCall(ExecCallRequest request);
     protected abstract CompletableFuture<BuildReceiptResponse> buildReceipt(BuildReceiptRequest request);
     protected abstract CompletableFuture<ConfigurePrivacyGroupResponse> configurePrivacyGroup(ConfigurePrivacyGroupRequest request);
     protected abstract CompletableFuture<InitPrivacyGroupResponse> initPrivacyGroup(InitPrivacyGroupRequest request);
     protected abstract CompletableFuture<WrapPrivacyGroupEVMTXResponse> wrapPrivacyGroupTransaction(WrapPrivacyGroupEVMTXRequest request);

     protected DomainInstance(String grpcTarget, String instanceId) {
         super(grpcTarget, instanceId);
     }
 
     public CompletableFuture<FindAvailableStatesResponse> findAvailableStates(FindAvailableStatesRequest request) {
         DomainMessage message = DomainMessage.newBuilder().
                 setHeader(newRequestHeader()).
                 setFindAvailableStates(request).
                 build();
         return requestReply(message).thenApply(DomainMessage::getFindAvailableStatesRes);
     }
 
     public CompletableFuture<EncodeDataResponse> encodeData(EncodeDataRequest request) {
         DomainMessage message = DomainMessage.newBuilder().
                 setHeader(newRequestHeader()).
                 setEncodeData(request).
                 build();
         return requestReply(message).thenApply(DomainMessage::getEncodeDataRes);
     }
 
     public CompletableFuture<DecodeDataResponse> decodeData(DecodeDataRequest request) {
         DomainMessage message = DomainMessage.newBuilder().
                 setHeader(newRequestHeader()).
                 setDecodeData(request).
                 build();
         return requestReply(message).thenApply(DomainMessage::getDecodeDataRes);
     }
 
     public CompletableFuture<RecoverSignerResponse> recoverSigner(RecoverSignerRequest request) {
         DomainMessage message = DomainMessage.newBuilder().
                 setHeader(newRequestHeader()).
                 setRecoverSigner(request).
                 build();
         return requestReply(message).thenApply(DomainMessage::getRecoverSignerRes);
     }
 
     @Override
     final StreamObserver<DomainMessage> connect(StreamObserver<DomainMessage> observer) {
         LOGGER.info("connecting domain gRPC to Paladin");
         return stub.connectDomain(observer);
     }
 
     @Override
     final Header getHeader(DomainMessage domainMessage) {
         return domainMessage.getHeader();
     }
 
     @Override
     final DomainMessage buildMessage(Header header) {
         return DomainMessage.newBuilder().setHeader(header).build();
     }
 
     @Override
     final CompletableFuture<DomainMessage> handleRequest(DomainMessage request) {
         LOGGER.info("JAVA_PLUGIN_REQUEST - pluginId={} type={} msgId={}", pluginId, request.getRequestToDomainCase().toString(), request.getHeader().getMessageId());
         DomainMessage.Builder response = DomainMessage.newBuilder();
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
                 case CONFIGURE_PRIVACY_GROUP -> configurePrivacyGroup(request.getConfigurePrivacyGroup()).thenApply(response::setConfigurePrivacyGroupRes);
                 case INIT_PRIVACY_GROUP -> initPrivacyGroup(request.getInitPrivacyGroup()).thenApply(response::setInitPrivacyGroupRes);
                 case WRAP_PRIVACY_GROUP_EVMTX -> wrapPrivacyGroupTransaction(request.getWrapPrivacyGroupEvmtx()).thenApply(response::setWrapPrivacyGroupEvmtxRes);
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
 