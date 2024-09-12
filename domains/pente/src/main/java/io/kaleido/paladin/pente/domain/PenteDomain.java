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

import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.ByteString;
import github.com.kaleido_io.paladin.toolkit.FromDomain;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.pente.evmstate.AccountLoader;
import io.kaleido.paladin.pente.evmstate.DynamicLoadWorldState;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import io.kaleido.paladin.toolkit.*;
import io.kaleido.paladin.toolkit.JsonHex.Address;
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
    }

    @Override
    protected CompletableFuture<ToDomain.ConfigureDomainResponse> configureDomain(ToDomain.ConfigureDomainRequest request) {
        // The in-memory config is late initialized here (and does so in its lock so access from any thread
        // we get called on for this and subsequent gRPC calls is safe).
        config.initFromConfig(request);

        var domainConfig = ToDomain.DomainConfig.newBuilder()
                .setConstructorAbiJson(config.abiEntry_privateConstructor().toString())
                .setFactoryContractAddress(config.getAddress().toString())
                .addAllAbiStateSchemasJson(config.allPenteSchemas())
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
                        setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                        setLookup(lookup).
                        build());
            }
            return CompletableFuture.completedFuture(response.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    private List<Address> getResolvedEndorsers(Bytes32 salt, String[] members, List<ToDomain.ResolvedVerifier> resolvedVerifiers) {
        // Get the resolved address for each endorser we set the lookup for
        var lookups = PenteTransaction.buildGroupScopeIdentityLookups(salt, members);
        List<Address> endorsementAddresses = new ArrayList<>(lookups.size());
        for (String lookup : lookups) {
            for (ToDomain.ResolvedVerifier verifier : resolvedVerifiers) {
                if (verifier.getLookup().equals(lookup) && verifier.getAlgorithm().equals(Algorithms.ECDSA_SECP256K1_PLAINBYTES)) {
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
            onchainConfBuilder.write(PenteConfiguration.intToBytes4(PenteConfiguration.PenteConfigID_Endorsement_V0));
            onchainConfBuilder.write(PenteConfiguration.abiEncoder_Endorsement_V0(
                    params.evmVersion(),
                    resolvedVerifiers.size(),
                    resolvedVerifiers
            ).getBytes());
            var response = ToDomain.PrepareDeployResponse.newBuilder();
            var newPrivacyGroupABIJson = config.getFactoryContractABI().getABIEntry("function", "newPrivacyGroup").toJSON(false);
            response.getTransactionBuilder().
                    setFunctionAbiJson(newPrivacyGroupABIJson).
                    setParamsJson(new ObjectMapper().writeValueAsString(new PenteConfiguration.NewPrivacyGroupFactoryParams(
                            new Bytes32(request.getTransaction().getTransactionId()),
                            new JsonHex.Bytes(onchainConfBuilder.toByteArray())
                    )));
            LOGGER.info("endorsement group verifier addresses: {}", resolvedVerifiers);
            return CompletableFuture.completedFuture(response.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.InitTransactionResponse> initTransaction(ToDomain.InitTransactionRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());
            var response = ToDomain.InitTransactionResponse.newBuilder();
            response.addRequiredVerifiers(ToDomain.ResolveVerifierRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                    setLookup(tx.getFrom()).
                    build()
            );
            return CompletableFuture.completedFuture(response.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.AssembleTransactionResponse> assembleTransaction(ToDomain.AssembleTransactionRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());

            // Execution throws an EVMExecutionException if fails
            var accountLoader = new AssemblyAccountLoader();
            var execResult = tx.executeEVM(config.getChainId(), tx.getFromVerifier(request.getResolvedVerifiersList()), accountLoader);
            var result = ToDomain.AssembleTransactionResponse.newBuilder();
            var assembledTransaction = tx.buildAssembledTransaction(execResult.evm(), accountLoader);
            result.setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.OK);
            result.setAssembledTransaction(assembledTransaction);

            // Just like a base Eth transaction, we sign the encoded transaction.
            // However, we do not package the signature back up in any RLP encoded way
            // back again into a full transaction RLP bytestring.
            result.addAttestationPlan(ToDomain.AttestationRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                    setAttestationType(ToDomain.AttestationType.SIGN).
                    setPayload(ByteString.copyFrom(execResult.txPayloadHash().getBytes())).
                    addParties(tx.getFrom()).
                    build()
            );

            // In addition to the signing address of the sender of this transaction (which can be any eth address)
            // we need to get endorsements from all endorsers in the list associated with the privacy group.
            // This includes this node, but not the same signing address, so there's no special optimization for "us"
            // to avoid the re-execution of the EVM transaction on this local node at endorsement phase.
            var params = tx.getValues();
            var endorsers = PenteTransaction.buildGroupScopeIdentityLookups(params.group().salt(), params.group().members());
            result.addAttestationPlan(ToDomain.AttestationRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                    setAttestationType(ToDomain.AttestationType.ENDORSE).
                    addAllParties(endorsers).
                    build()
            );
            return CompletableFuture.completedFuture(result.build());
        } catch(PenteTransaction.EVMExecutionException e) {
            // Note unlike a base ledger, we do not write a nonce update to the sender's account
            // (which would be a UTXO spend + mint) for a revert during assembly of a transaction,
            // as endorsing and submitting that would be lots of work.
            LOGGER.error(new FormattedMessage("EVM execution failed during assemble for TX {}", request.getTransaction().getTransactionId()), e);
            return CompletableFuture.completedFuture(ToDomain.AssembleTransactionResponse.newBuilder().
                    setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.REVERT).
                    setRevertReason(e.getMessage()).
                    build());
        } catch(Exception e) {
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

            // Do the execution of the transaction again ourselves
            var tx = new PenteTransaction(this, request.getTransaction());
            var endorsementLoader = new EndorsementAccountLoader(inputAccounts, readAccounts);
            var execResult = tx.executeEVM(config.getChainId(), tx.getFromVerifier(request.getResolvedVerifiersList()), endorsementLoader);

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

            // Recover the signer against the payload as we processed it
            ByteString signature = null;
            for (var sign : request.getSignaturesList()) {
                if (sign.getVerifier().getAlgorithm().equals(Algorithms.ECDSA_SECP256K1_PLAINBYTES) &&
                   sign.getVerifier().getVerifier().equals(execResult.senderAddress().toString())) {
                    signature = sign.getPayload();
                }
            }
            if (signature == null) {
                throw new IllegalArgumentException("missing signature for %s".formatted(execResult.senderAddress()));
            }
            var recovered = recoverSigner(FromDomain.RecoverSignerRequest.newBuilder().
                    setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                    setPayload(ByteString.copyFrom(execResult.txPayloadHash().getBytes())).
                    setSignature(signature).
                    build()).get();
            if (!recovered.getVerifier().equals(execResult.senderAddress().toString())) {
                throw new IllegalArgumentException("invalid signature for %s (recovered=%s)".formatted(execResult.senderAddress(), recovered.getVerifier()));
            }

            // Check we agree with the typed data we will sign
            var endorsementPayload = tx.eip712TypedDataEndorsementPayload(
                request.getInputsList().stream().map(ToDomain.EndorsableState::getId).toList(),
                request.getReadsList().stream().map(ToDomain.EndorsableState::getId).toList(),
                request.getOutputsList().stream().map(ToDomain.EndorsableState::getId).toList()
            );

            // Ok - we are happy to add our endorsement signature
            return CompletableFuture.completedFuture(ToDomain.EndorseTransactionResponse.newBuilder().
                    setEndorsementResult(ToDomain.EndorseTransactionResponse.Result.SIGN).
                    setPayload(ByteString.copyFrom(endorsementPayload)).
                    build());
        } catch(PenteTransaction.EVMExecutionException e) {
            LOGGER.error(new FormattedMessage("EVM execution failed during endorsement TX {}", request.getTransaction().getTransactionId()), e);
            return CompletableFuture.completedFuture(ToDomain.EndorseTransactionResponse.newBuilder().
                    setEndorsementResult(ToDomain.EndorseTransactionResponse.Result.SIGN).
                    setRevertReason(e.getMessage()).
                    build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareTransactionResponse> prepareTransaction(ToDomain.PrepareTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    /** during assembly we load available states from the Paladin state store */
    class AssemblyAccountLoader implements AccountLoader {
        private final HashMap<org.hyperledger.besu.datatypes.Address, FromDomain.StoredState> loadedAccountStates = new HashMap<>();
        public Optional<PersistedAccount> load(org.hyperledger.besu.datatypes.Address address) throws IOException {
            return withIOException(() -> {
                var queryJson = JsonQuery.newBuilder().
                        limit(1).
                        isEqual("address", address.toString()).
                        json();
                var response = findAvailableStates(FromDomain.FindAvailableStatesRequest.newBuilder().
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
    public interface SupplierEx<T> { T get() throws Exception; }
    static <ReturnType> ReturnType withIOException(SupplierEx<ReturnType> fn) throws IOException {
        try {
            return fn.get();
        } catch(IOException e) {
            throw e;
        } catch (Exception e) {
            if (e instanceof RuntimeException) {
                throw (RuntimeException)(e);
            }
            throw new RuntimeException(e);
        }
    }
}
