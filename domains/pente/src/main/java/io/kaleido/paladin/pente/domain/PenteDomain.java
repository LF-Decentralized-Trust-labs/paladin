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

package io.kaleido.paladin.pente.domain;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.ByteString;
import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.toolkit.FromDomain;
import io.kaleido.paladin.toolkit.ToDomain;
import io.kaleido.paladin.pente.evmstate.AccountLoader;
import io.kaleido.paladin.pente.evmstate.DynamicLoadWorldState;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import io.kaleido.paladin.toolkit.*;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.*;
import java.util.concurrent.CompletableFuture;

public class PenteDomain extends DomainInstance {
    private static final Logger LOGGER = LogManager.getLogger(PenteDomain.class);

    private final PenteConfiguration config = new PenteConfiguration();

    PenteDomain(String grpcTarget, String instanceId) {
        super(grpcTarget, instanceId);
        init();
    }

    @Override
    protected CompletableFuture<ToDomain.ConfigureDomainResponse> configureDomain(ToDomain.ConfigureDomainRequest request) {
        // The in-memory config is late initialized here (and does so in its lock so access from any thread
        // we get called on for this and subsequent gRPC calls is safe).
        config.initFromConfig(request);

        var domainConfig = ToDomain.DomainConfig.newBuilder()
                .addAllAbiStateSchemasJson(config.allPenteSchemas())
                .setAbiEventsJson(config.getEventsABI().toString())
                .build();
        return CompletableFuture.completedFuture(ToDomain.ConfigureDomainResponse.newBuilder()
                .setDomainConfig(domainConfig)
                .build()
        );
    }

    @Override
    protected CompletableFuture<ToDomain.InitDomainResponse> initDomain(ToDomain.InitDomainRequest request) {
        // Store our state schema
        config.schemasInitialized(request.getAbiStateSchemasList());
        return CompletableFuture.completedFuture(ToDomain.InitDomainResponse.getDefaultInstance());
    }

    @Override
    protected CompletableFuture<ToDomain.InitDeployResponse> initDeploy(ToDomain.InitDeployRequest request) {
        try {
            var params = new ObjectMapper().readValue(request.getTransaction().getConstructorParamsJson(),
                    PenteConfiguration.PrivacyGroupConstructorParamsJSON.class);

            // Only support one string right now for endorsement type.
            // The intention is that more validation options (BLS and/or ZKP based) can be added later.
            //
            // Note the threshold is 100% right now, as there are architectural considerations for
            // supporting gap-fil of missed transactions for members that miss transitions because
            // they were not required to endorse every transaction.
            if (params.endorsementType() == null || !params.endorsementType().equalsIgnoreCase(
                    PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES)) {
                throw new Exception("endorsementTypes supported: %s".formatted(
                        Collections.singletonList(PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES)
                ));
            }

            if (params.group() == null || params.group().members() == null || params.group().members().length < 1) {
                throw new Exception("privacy group must have at least one member");
            }

            var response = ToDomain.InitDeployResponse.newBuilder();
            var lookups = PenteTransaction.buildGroupScopeIdentityLookups(params.group().salt(), params.group().members());
            LOGGER.info("endorsement group identity lookups: {}", lookups);
            for (String lookup : lookups) {
                response.addRequiredVerifiers(ToDomain.ResolveVerifierRequest.newBuilder().
                        setAlgorithm(Algorithms.ECDSA_SECP256K1).
                        setVerifierType(Verifiers.ETH_ADDRESS).
                        setLookup(lookup).
                        build());
            }
            return CompletableFuture.completedFuture(response.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    private List<Address> getResolvedEndorsers(Bytes32 salt, String[] members, List<ToDomain.ResolvedVerifier> resolvedVerifiers) {
        // Get the resolved address for each endorser we set the lookup for
        var lookups = PenteTransaction.buildGroupScopeIdentityLookups(salt, members);
        List<Address> endorsementAddresses = new ArrayList<>(lookups.size());
        for (String lookup : lookups) {
            for (ToDomain.ResolvedVerifier verifier : resolvedVerifiers) {
                if (verifier.getLookup().equals(lookup) &&
                        verifier.getAlgorithm().equals(Algorithms.ECDSA_SECP256K1) &&
                        verifier.getVerifierType().equals(Verifiers.ETH_ADDRESS)) {
                    // Check it's not in the list already
                    Address addr = JsonHex.addressFrom(verifier.getVerifier());
                    for (Address endorser : endorsementAddresses) {
                        if (endorser.equals(addr)) {
                            throw new IllegalArgumentException("Duplicate resolved endorser %s (lookup='%s')".formatted(addr, lookup));
                        }
                    }
                    endorsementAddresses.add(addr);
                }
            }
        }
        return endorsementAddresses;
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareDeployResponse> prepareDeploy(ToDomain.PrepareDeployRequest request) {
        try {
            var params = new ObjectMapper().readValue(request.getTransaction().getConstructorParamsJson(),
                    PenteConfiguration.PrivacyGroupConstructorParamsJSON.class);

            var resolvedVerifiers = getResolvedEndorsers(params.group().salt(), params.group().members(), request.getResolvedVerifiersList());
            var onchainConfBuilder = new ByteArrayOutputStream();
            onchainConfBuilder.write(PenteConfiguration.intToBytes4(PenteConfiguration.PenteConfigID_V0));
            onchainConfBuilder.write(PenteConfiguration.abiEncoder_Config_V0(
                    params.evmVersion(),
                    resolvedVerifiers.size(),
                    resolvedVerifiers,
                    params.externalCallsEnabled()
            ).getBytes());
            var response = ToDomain.PrepareDeployResponse.newBuilder().
                    setSigner("%s.deploy.%s".formatted(config.getDomainName(), UUID.randomUUID().toString()));
            var newPrivacyGroupABIJson = config.getFactoryContractABI().getABIEntry("function", "newPrivacyGroup").toJSON(false);
            response.getTransactionBuilder().
                    setFunctionAbiJson(newPrivacyGroupABIJson).
                    setParamsJson(new ObjectMapper().writeValueAsString(new PenteConfiguration.NewPrivacyGroupFactoryParams(
                            new Bytes32(request.getTransaction().getTransactionId()),
                            new JsonHex.Bytes(onchainConfBuilder.toByteArray())
                    )));
            LOGGER.info("endorsement group verifier addresses: {}", resolvedVerifiers);
            return CompletableFuture.completedFuture(response.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.InitContractResponse> initContract(ToDomain.InitContractRequest request) {
        try {
            var onChainConfig = PenteConfiguration.decodeConfig(request.getContractConfig().toByteArray());

            var contractConfigObj = new PenteConfiguration.ContractConfig(onChainConfig.evmVersion());
            var contractConfig = ToDomain.ContractConfig.newBuilder().
                    setContractConfigJson(new ObjectMapper().writeValueAsString(contractConfigObj)).
                    setCoordinatorSelection(ToDomain.ContractConfig.CoordinatorSelection.COORDINATOR_ENDORSER).
                    setSubmitterSelection(ToDomain.ContractConfig.SubmitterSelection.SUBMITTER_COORDINATOR).
                    build();
            return CompletableFuture.completedFuture(ToDomain.InitContractResponse.newBuilder().
                    setValid(true).
                    setContractConfig(contractConfig).
                    build());

        } catch (Exception e) {
            LOGGER.error(new FormattedMessage("Invalid configuration for domain {}", request.getContractAddress()), e);
            // We do not stall the indexer for this
            return CompletableFuture.completedFuture(ToDomain.InitContractResponse.newBuilder().
                    setValid(false).
                    build());
        }
    }

    @Override
    protected CompletableFuture<ToDomain.InitTransactionResponse> initTransaction(ToDomain.InitTransactionRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());
            var response = ToDomain.InitTransactionResponse.newBuilder();
            response.addRequiredVerifiers(ToDomain.ResolveVerifierRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1).
                    setVerifierType(Verifiers.ETH_ADDRESS).
                    setLookup(tx.getFrom()).
                    build()
            );
            return CompletableFuture.completedFuture(response.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    private List<PenteConfiguration.TransactionExternalCall> parseExternalCalls(List<EVMRunner.JsonEVMLog> logs) throws Exception {
        var externalCalls = new ArrayList<PenteConfiguration.TransactionExternalCall>();
        for (var log : logs) {
            if (log.topics().getFirst().equals(config.getExternalCallTopic())) {
                var decodedEvent = decodeData(FromDomain.DecodeDataRequest.newBuilder().
                        setEncodingType(FromDomain.EncodingType.EVENT_DATA).
                        setDefinition(config.getExternalCallEventABI().toJSON(false)).
                        addAllTopics(log.topics().stream().map(t -> ByteString.copyFrom(t.getBytes())).toList()).
                        setData(ByteString.copyFrom(log.data().getBytes())).
                        build()).get();
                var externalCall = new ObjectMapper().readValue(
                        decodedEvent.getBody(),
                        PenteConfiguration.TransactionExternalCall.class);
                externalCalls.add(externalCall);
            }
        }
        return externalCalls;
    }

    private String buildDomainData(PenteEVMTransaction.EVMExecutionResult execResult) throws Exception {
        return new ObjectMapper().writeValueAsString(
                new PenteConfiguration.DomainData(
                        new Address(execResult.contractAddress().toArray()),
                        parseExternalCalls(execResult.logs())));
    }

    @Override
    protected CompletableFuture<ToDomain.AssembleTransactionResponse> assembleTransaction(ToDomain.AssembleTransactionRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());

            // Execution throws an EVMExecutionException if fails
            var accountLoader = new AssemblyAccountLoader(request.getStateQueryContext());
            var ethTxn = new PenteEVMTransaction(this, tx, tx.getFromVerifier(request.getResolvedVerifiersList()));
            var execResult = ethTxn.invokeEVM(accountLoader);
            var result = ToDomain.AssembleTransactionResponse.newBuilder();

            // Just like a base Eth transaction, we need a signed and encoded transaction for endorser verification.
            // We package this up as an "info" state on the transaction, that will be distributed to all parties
            // just like all the other states. However, it never exists in the UTXO map on-chain, and is never
            // available for selection as an input to a transaction. It exists only to be emitted as part of the
            // event from the transaction where it is used.
            var encodedTxn = tx.getSignedRawTransaction(ethTxn);
            var assembledTransaction = tx.buildAssembledTransaction(execResult.evm(), accountLoader, ethTxn, encodedTxn, buildDomainData(execResult));

            // We now have the assembly result
            result.setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.OK);
            result.setAssembledTransaction(assembledTransaction);

            // In addition to the signing address of the sender of this transaction (which can be any eth address)
            // we need to get endorsements from all endorsers in the list associated with the privacy group.
            // This includes this node, but not the same signing address, so there's no special optimization for "us"
            // to avoid the re-execution of the EVM transaction on this local node at endorsement phase.
            var params = tx.getValues();
            var endorsers = PenteTransaction.buildGroupScopeIdentityLookups(params.group().salt(), params.group().members());
            result.addAttestationPlan(ToDomain.AttestationRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1).
                    setVerifierType(Verifiers.ETH_ADDRESS).
                    setAttestationType(ToDomain.AttestationType.ENDORSE).
                    setPayloadType(SignPayloads.OPAQUE_TO_RSV).
                    addAllParties(endorsers).
                    build()
            );
            return CompletableFuture.completedFuture(result.build());
        } catch (PenteEVMTransaction.EVMExecutionException e) {
            // Note unlike a base ledger, we do not write a nonce update to the sender's account
            // (which would be a UTXO spend + mint) for a revert during assembly of a transaction,
            // as endorsing and submitting that would be lots of work.
            LOGGER.error(new FormattedMessage("EVM execution failed during assemble for TX {}", request.getTransaction().getTransactionId()), e);
            return CompletableFuture.completedFuture(ToDomain.AssembleTransactionResponse.newBuilder().
                    setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.REVERT).
                    setRevertReason(e.getMessage()).
                    build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.EndorseTransactionResponse> endorseTransaction(ToDomain.EndorseTransactionRequest request) {
        try {
            // Parse all the inputs/reads supplied into inputs
            var inputAccounts = new ArrayList<PersistedAccount>(request.getInputsCount());
            for (var input : request.getInputsList()) {
                inputAccounts.add(PersistedAccount.deserialize(input.getStateDataJson().getBytes(StandardCharsets.UTF_8)));
            }
            var readAccounts = new ArrayList<PersistedAccount>(request.getReadsCount());
            for (var read : request.getReadsList()) {
                readAccounts.add(PersistedAccount.deserialize(read.getStateDataJson().getBytes(StandardCharsets.UTF_8)));
            }
            if (request.getInfoCount() != 1)
                throw new IllegalArgumentException("Expected exactly one info state containing the transaction input");

            // Recover the input from the signed rawTransaction that is in the "info" state recorded alongside the transaction
            var tx = new PenteTransaction(this, request.getTransaction());
            var evmTxn = PenteEVMTransaction.buildFromInput(this, request.getInfo(0).getStateDataJson().getBytes(StandardCharsets.UTF_8));

            // Do the execution of the transaction again ourselves
            var endorsementLoader = new EndorsementAccountLoader(inputAccounts, readAccounts);
            var execResult = evmTxn.invokeEVM(endorsementLoader);

            // For the inputs, the endorsementLoader checks we loaded everything from the right set
            var inputsMatch = endorsementLoader.checkEmpty();

            // Build the expected outputs
            var expectedOutputs = new ArrayList<PersistedAccount>(request.getOutputsCount());
            for (var output : request.getOutputsList()) {
                expectedOutputs.add(PersistedAccount.deserialize(output.getStateDataJson().getBytes(StandardCharsets.UTF_8)));
            }
            // Go round the actual outputs and confirm they match
            var newWorld = execResult.evm().getWorld();
            var committedUpdates = newWorld.getCommittedAccountUpdates();
            var outputsMatch = (committedUpdates.size() == expectedOutputs.size());
            for (var update : committedUpdates.entrySet()) {
                boolean matchFound = false;
                if (update.getValue() != DynamicLoadWorldState.LastOpType.DELETED) {
                    var resultingState = newWorld.get(update.getKey());
                    for (var expectedOutput : expectedOutputs) {
                        if (expectedOutput.getAddress().equals(update.getKey())) {
                            if (expectedOutput.equals(resultingState)) {
                                matchFound = true;
                            } else {
                                LOGGER.error("Address {} expected={} actual={}", update.getKey(), expectedOutput, resultingState);
                            }
                            break;
                        }
                    }
                }
                if (!matchFound) {
                    LOGGER.error("Address update result unmatched {}", update.getKey());
                }
                outputsMatch = outputsMatch && matchFound;
            }

            if (!inputsMatch || !outputsMatch) {
                LOGGER.error("Endorsement failed inputsMatch={} outputsMatch={}. EXPECTED inputs={} reads={} outputs={}",
                        inputsMatch, outputsMatch,
                        inputAccounts, readAccounts, expectedOutputs);
                throw new IllegalStateException("Execution state mismatch detected in endorsement");
            }

            // Check we agree with the typed data we will sign
            var endorsementPayload = tx.eip712TypedDataEndorsementPayload(
                    request.getInputsList().stream().map(ToDomain.EndorsableState::getId).toList(),
                    request.getReadsList().stream().map(ToDomain.EndorsableState::getId).toList(),
                    request.getOutputsList().stream().map(ToDomain.EndorsableState::getId).toList(),
                    request.getInfoList().stream().map(ToDomain.EndorsableState::getId).toList(),
                    parseExternalCalls(execResult.logs())
            );

            // Ok - we are happy to add our endorsement signature
            return CompletableFuture.completedFuture(ToDomain.EndorseTransactionResponse.newBuilder().
                    setEndorsementResult(ToDomain.EndorseTransactionResponse.Result.SIGN).
                    setPayload(ByteString.copyFrom(endorsementPayload)).
                    build());
        } catch (PenteEVMTransaction.EVMExecutionException e) {
            LOGGER.error(new FormattedMessage("EVM execution failed during endorsement TX {}", request.getTransaction().getTransactionId()), e);
            return CompletableFuture.completedFuture(ToDomain.EndorseTransactionResponse.newBuilder().
                    setEndorsementResult(ToDomain.EndorseTransactionResponse.Result.SIGN).
                    setRevertReason(e.getMessage()).
                    build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareTransactionResponse> prepareTransaction(ToDomain.PrepareTransactionRequest request) {
        try {
            var signatures = request.getAttestationResultList().stream().
                    filter(r -> r.getAttestationType() == ToDomain.AttestationType.ENDORSE).
                    toList();
            List<PenteConfiguration.TransactionExternalCall> externalCalls;
            if (request.getDomainData().isEmpty()) {
                externalCalls = Collections.emptyList();
            } else {
                var domainData = new ObjectMapper().readValue(request.getDomainData(), PenteConfiguration.DomainData.class);
                externalCalls = domainData.externalCalls();
            }

            var params = new HashMap<String, Object>() {{
                put("txId", request.getTransaction().getTransactionId());
                put("states", new HashMap<String, Object>() {{
                    put("inputs", request.getInputStatesList().stream().map(ToDomain.EndorsableState::getId).toList());
                    put("reads", request.getReadStatesList().stream().map(ToDomain.EndorsableState::getId).toList());
                    put("outputs", request.getOutputStatesList().stream().map(ToDomain.EndorsableState::getId).toList());
                    put("info", request.getInfoStatesList().stream().map(ToDomain.EndorsableState::getId).toList());
                }});
                put("externalCalls", externalCalls);
                put("signatures", signatures.stream().map(r -> JsonHex.wrap(r.getPayload().toByteArray())).toList());
            }};

            var transitionABI = config.getPrivacyGroupABI().getABIEntry("function", "transition").toJSON(false);
            var transitionTX = ToDomain.PreparedTransaction.newBuilder().
                    setFunctionAbiJson(transitionABI).
                    setParamsJson(new ObjectMapper().writeValueAsString(params));
            var result = ToDomain.PrepareTransactionResponse.newBuilder().
                    setTransaction(transitionTX);

            if (request.getTransaction().getIntent() == ToDomain.TransactionSpecification.Intent.PREPARE_TRANSACTION) {
                // TODO: can the transitionHash be reused from a prior step instead of being computed again?
                var tx = new PenteTransaction(this, request.getTransaction());
                var transitionHash = tx.eip712TypedDataEndorsementPayload(
                        request.getInputStatesList().stream().map(ToDomain.EndorsableState::getId).toList(),
                        request.getReadStatesList().stream().map(ToDomain.EndorsableState::getId).toList(),
                        request.getOutputStatesList().stream().map(ToDomain.EndorsableState::getId).toList(),
                        request.getInfoStatesList().stream().map(ToDomain.EndorsableState::getId).toList(),
                        externalCalls);

                var transitionWithApprovalABI = config.getPrivacyGroupABI().getABIEntry("function", "transitionWithApproval");
                var transitionWithApprovalParams = new HashMap<String, Object>() {{
                    put("txId", request.getTransaction().getTransactionId());
                    put("states", params.get("states"));
                    put("externalCalls", externalCalls);
                }};
                var transitionWithApprovalParamsJSON = new ObjectMapper().writeValueAsString(transitionWithApprovalParams);

                var encodeRequest = FromDomain.EncodeDataRequest.newBuilder().
                        setEncodingType(FromDomain.EncodingType.FUNCTION_CALL_DATA).
                        setDefinition(transitionWithApprovalABI.toJSON(false)).
                        setBody(transitionWithApprovalParamsJSON).
                        build();
                var encodeResponse = encodeData(encodeRequest).get();

                var metadata = new PenteConfiguration.PenteTransitionMetadata(
                        new PenteConfiguration.PenteApprovalParams(
                                new JsonHex.Bytes32(transitionHash),
                                signatures.stream().map(r -> JsonHex.wrap(r.getPayload().toByteArray())).toList()),
                        new PenteConfiguration.PentePublicTransaction(
                                transitionWithApprovalABI,
                                transitionWithApprovalParamsJSON,
                                JsonHex.wrap(encodeResponse.getData().toByteArray())));

                result.setMetadata(new ObjectMapper().writeValueAsString(metadata));
            }
            return CompletableFuture.completedFuture(result.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.HandleEventBatchResponse> handleEventBatch(ToDomain.HandleEventBatchRequest request) {
        try {
            var mapper = new ObjectMapper();
            var result = ToDomain.HandleEventBatchResponse.newBuilder();
            for (var event : request.getEventsList()) {
                if (PenteConfiguration.transferSignature.equals(event.getSoliditySignature())) {
                    var transfer = mapper.readValue(event.getDataJson(), PenteTransitionJSON.class);
                    var inputs = Arrays.stream(transfer.inputs).map(id -> ToDomain.StateUpdate.newBuilder()
                            .setId(id.to0xHex())
                            .setTransactionId(transfer.txId.to0xHex())
                            .build()).toList();
                    var reads = Arrays.stream(transfer.reads).map(id -> ToDomain.StateUpdate.newBuilder()
                            .setId(id.to0xHex())
                            .setTransactionId(transfer.txId.to0xHex())
                            .build()).toList();
                    var outputs = Arrays.stream(transfer.outputs).map(id -> ToDomain.StateUpdate.newBuilder()
                            .setId(id.to0xHex())
                            .setTransactionId(transfer.txId.to0xHex())
                            .build()).toList();
                    var info = Arrays.stream(transfer.info).map(id -> ToDomain.StateUpdate.newBuilder()
                            .setId(id.to0xHex())
                            .setTransactionId(transfer.txId.to0xHex())
                            .build()).toList();
                    result.addTransactionsComplete(ToDomain.CompletedTransaction.newBuilder()
                                    .setTransactionId(transfer.txId.to0xHex())
                                    .setLocation(event.getLocation())
                                    .build())
                            .addAllSpentStates(inputs)
                            .addAllReadStates(reads)
                            .addAllConfirmedStates(outputs)
                            .addAllInfoStates(info);
                } else if (PenteConfiguration.approvalSignature.equals(event.getSoliditySignature())) {
                    var approval = mapper.readValue(event.getDataJson(), PenteApprovedJSON.class);
                    result.addTransactionsComplete(ToDomain.CompletedTransaction.newBuilder()
                                    .setTransactionId(approval.txId.to0xHex())
                                    .setLocation(event.getLocation())
                                    .build());
                    
                } else {
                    throw new Exception("Unknown signature: " + event.getSoliditySignature());
                }
            }
            return CompletableFuture.completedFuture(result.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.SignResponse> sign(ToDomain.SignRequest request) {
        // Pente currently only uses SECP256K1 cryptography, which is fully supported by the built-in
        // Paladin signing module without any extension requirements.
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.GetVerifierResponse> getVerifier(ToDomain.GetVerifierRequest request) {
        // Pente currently only uses SECP256K1 cryptography, which is fully supported by the built-in
        // Paladin signing module without any extension requirements.
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.ValidateStateHashesResponse> validateStateHashes(ToDomain.ValidateStateHashesRequest request) {
        // Pente uses the standard state hash generation of Paladin, so this function is not called per the spec
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.InitCallResponse> initCall(ToDomain.InitCallRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());
            var response = ToDomain.InitCallResponse.newBuilder();
            response.addRequiredVerifiers(ToDomain.ResolveVerifierRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1).
                    setVerifierType(Verifiers.ETH_ADDRESS).
                    setLookup(tx.getFrom()).
                    build()
            );
            return CompletableFuture.completedFuture(response.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.ExecCallResponse> execCall(ToDomain.ExecCallRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());
            var accountLoader = new AssemblyAccountLoader(request.getStateQueryContext());
            var ethTxn = new PenteEVMTransaction(this, tx, tx.getFromVerifier(request.getResolvedVerifiersList()));
            var result = ethTxn.invokeEVM(accountLoader);

            var response = ToDomain.ExecCallResponse.newBuilder();
            response.setResultJson(tx.decodeOutput(result.outputData()));
            return CompletableFuture.completedFuture(response.build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.BuildReceiptResponse> buildReceipt(ToDomain.BuildReceiptRequest request) {
        try {
            if (!request.getComplete()) {
                throw new IllegalStateException("all states must be available to build an EVM receipt");
            }

            // We execute the transaction just like we would during endorsement, but here we return the information
            // for users to consume. Such as the contractAddress, the logs, or even the outputData.

            var inputAccounts = new ArrayList<PersistedAccount>(request.getInputStatesCount());
            for (var input : request.getInputStatesList()) {
                inputAccounts.add(PersistedAccount.deserialize(input.getStateDataJson().getBytes(StandardCharsets.UTF_8)));
            }
            var readAccounts = new ArrayList<PersistedAccount>(request.getReadStatesCount());
            for (var read : request.getReadStatesList()) {
                readAccounts.add(PersistedAccount.deserialize(read.getStateDataJson().getBytes(StandardCharsets.UTF_8)));
            }
            if (request.getInfoStatesCount() != 1)
                throw new IllegalArgumentException("Expected exactly one info state containing the transaction input");

            // Recover the input from the signed rawTransaction that is in the "info" state recorded alongside the transaction
            var evmTxn = PenteEVMTransaction.buildFromInput(this, request.getInfoStates(0).getStateDataJson().getBytes(StandardCharsets.UTF_8));

            // Do the execution of the transaction again ourselves
            var endorsementLoader = new EndorsementAccountLoader(inputAccounts, readAccounts);
            var execResult = evmTxn.invokeEVM(endorsementLoader);

            // Build the full receipt from the result
            var jsonReceipt = evmTxn.buildJSONReceipt(execResult);

            return CompletableFuture.completedFuture(ToDomain.BuildReceiptResponse.newBuilder().
                    setReceiptJson(new ObjectMapper().writerWithDefaultPrettyPrinter().writeValueAsString(jsonReceipt)).
                    build());
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record PenteTransitionJSON(
            @JsonProperty
            Bytes32 txId,
            @JsonProperty
            Bytes32[] inputs,
            @JsonProperty
            Bytes32[] reads,
            @JsonProperty
            Bytes32[] outputs,
            @JsonProperty
            Bytes32[] info,
            @JsonProperty
            Bytes data
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record PenteApprovedJSON(
            @JsonProperty
            Bytes32 txId,
            @JsonProperty
            Address delegate,
            @JsonProperty
            Bytes32 transitionHash
    ) {
    }

    /** during assembly, we load available states from the Paladin state store */
    class AssemblyAccountLoader implements AccountLoader {
        private final HashMap<org.hyperledger.besu.datatypes.Address, FromDomain.StoredState> loadedAccountStates = new HashMap<>();
        private final String stateQueryContext;

        AssemblyAccountLoader(String stateQueryContext) {
            this.stateQueryContext = stateQueryContext;
        }

        public Optional<PersistedAccount> load(org.hyperledger.besu.datatypes.Address address) throws IOException {
            return withIOException(() -> {
                var queryJson = JsonQuery.newBuilder().
                        limit(1).
                        isEqual("address", address.toString()).
                        json();
                var response = findAvailableStates(FromDomain.FindAvailableStatesRequest.newBuilder().
                        setStateQueryContext(stateQueryContext).
                        setSchemaId(config.schemaId_AccountStateLatest()).
                        setQueryJson(queryJson).
                        build()).get();
                if (response.getStatesCount() != 1) {
                    loadedAccountStates.put(address, null);
                    return Optional.empty();
                }
                var state = response.getStates(0);
                loadedAccountStates.put(address, state);
                return Optional.of(PersistedAccount.deserialize(state.getDataJsonBytes().toByteArray()));
            });
        }
        final Map<org.hyperledger.besu.datatypes.Address, FromDomain.StoredState> getLoadedAccountStates() {
            return loadedAccountStates;
        }
    }

    /** During endorsement, only the accounts in the "inputs" and "reads" list are available to execute. */
    static class EndorsementAccountLoader implements AccountLoader {
        private final Map<org.hyperledger.besu.datatypes.Address, PersistedAccount> inputAccounts = new HashMap<>();
        private final Map<org.hyperledger.besu.datatypes.Address, PersistedAccount> readAccounts = new HashMap<>();
        EndorsementAccountLoader(List<PersistedAccount> inputAccounts, List<PersistedAccount> readAccounts) {
            for (var account : inputAccounts) {
                this.inputAccounts.put(account.getAddress(), account);
            }
            for (var account : readAccounts) {
                this.readAccounts.put(account.getAddress(), account);
            }
        }

        public Optional<PersistedAccount> load(org.hyperledger.besu.datatypes.Address address) {
            var account = inputAccounts.remove(address);
            if (account != null) {
                return Optional.of(account);
            }
            account = readAccounts.remove(address);
            if (account != null) {
                return Optional.of(account);
            }
            return Optional.empty();
        }

        boolean checkEmpty() {
            return readAccounts.isEmpty() && inputAccounts.isEmpty();
        }
    }

    PenteConfiguration getConfig() {
        return config;
    }

    @FunctionalInterface
    public interface SupplierEx<T> {
        T get() throws Exception;
    }

    static <ReturnType> ReturnType withIOException(SupplierEx<ReturnType> fn) throws IOException {
        try {
            return fn.get();
        } catch (IOException e) {
            throw e;
        } catch (Exception e) {
            if (e instanceof RuntimeException) {
                throw (RuntimeException) (e);
            }
            throw new RuntimeException(e);
        }
    }
}
