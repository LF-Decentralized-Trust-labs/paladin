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
import github.com.kaleido_io.paladin.toolkit.FromDomain;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.pente.evmstate.AccountLoader;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import io.kaleido.paladin.toolkit.Algorithms;
import io.kaleido.paladin.toolkit.DomainInstance;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import io.kaleido.paladin.toolkit.JsonQuery;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.tuweni.bytes.Bytes;
import org.hyperledger.besu.evm.frame.MessageFrame;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
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
                .setFactoryContractAbiJson(config.getFactoryContractABI().toString())
                .setPrivateContractAbiJson(config.getPrivacyGroupABI().toString())
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
            response.getTransactionBuilder().
                    setFunctionName("newPrivacyGroup").
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
            if (tx.requiresABIEncoding()) response.addAbiEncodingRequests(tx.getABIEncodingRequest());
            return CompletableFuture.completedFuture(response.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.AssembleTransactionResponse> assembleTransaction(ToDomain.AssembleTransactionRequest request) {
        try {
            var tx = new PenteTransaction(this, request.getTransaction());
            var accoutLoader = new PenteAccountLoader();
            var evm = tx.getEVM(config.getChainId(), tx.getBaseBlock(), accoutLoader);
            var values = tx.getValues();
            var besuSender = org.hyperledger.besu.datatypes.Address.wrap(Bytes.wrap(tx.getFromVerifier(request.getResolvedVerifiersList()).getBytes()));
            MessageFrame execResult;
            if (values.to() == null) {
                execResult = evm.runContractDeploymentBytes(
                    besuSender,
                    null,
                    Bytes.wrap(tx.getValues().bytecode().getBytes()),
                    Bytes.wrap(tx.getABIEncodedData(request.getAbiEncodedDataList(), PenteTransaction.ABIEncodingRequestType.CONSTRUCTOR_PARAMS))
                );
            } else {
                execResult = evm.runContractInvokeBytes(
                    besuSender,
                    org.hyperledger.besu.datatypes.Address.fromHexString(values.to()),
                    Bytes.wrap(tx.getABIEncodedData(request.getAbiEncodedDataList(), PenteTransaction.ABIEncodingRequestType.FUNCTION_PARAMS))
                );
            }
            var result = ToDomain.AssembleTransactionResponse.newBuilder();
            if (execResult.getState() != MessageFrame.State.COMPLETED_SUCCESS) {
                result.setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.REVERT);
            } else {
                result.setAssemblyResult(ToDomain.AssembleTransactionResponse.Result.OK);
                result.setAssembledTransaction(tx.buildAssembledTransaction(evm, accoutLoader));
                result.addAttestationPlan(ToDomain.AttestationRequest.newBuilder().
                        setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                        setAttestationType(ToDomain.AttestationType.SIGN).
                        addParties(tx.getFrom()).
                        build()
                );
                result.addAttestationPlan(ToDomain.AttestationRequest.newBuilder().
                        setAlgorithm(Algorithms.ECDSA_SECP256K1_PLAINBYTES).
                        setAttestationType(ToDomain.AttestationType.ENDORSE).
                        build()
                );
            }
            return CompletableFuture.completedFuture(result.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    @Override
    protected CompletableFuture<ToDomain.EndorseTransactionResponse> endorseTransaction(ToDomain.EndorseTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    @Override
    protected CompletableFuture<ToDomain.PrepareTransactionResponse> prepareTransaction(ToDomain.PrepareTransactionRequest request) {
        return CompletableFuture.failedFuture(new UnsupportedOperationException());
    }

    class PenteAccountLoader implements AccountLoader {
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
                    return Optional.empty();
                }
                var state = response.getStates(0);
                loadedAccountStates.put(address, state);
                return Optional.of(PersistedAccount.deserialize(state.getDataJsonBytes().toByteArray()));
            });
        }
        public Map<org.hyperledger.besu.datatypes.Address, FromDomain.StoredState> getLoadedAccountStates() {
            return loadedAccountStates;
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
