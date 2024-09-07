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

package io.kaleido.pente.domain;

import com.fasterxml.jackson.databind.ObjectMapper;
import github.com.kaleido_io.paladin.toolkit.ToDomain;
import io.kaleido.paladin.toolkit.Algorithms;
import io.kaleido.paladin.toolkit.DomainInstance;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.JsonHex.Address;
import io.kaleido.paladin.toolkit.JsonHex.Bytes32;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.web3j.abi.datatypes.DynamicStruct;

import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
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
        config.initFromJSON(request.getConfigJson());

        ToDomain.DomainConfig domainConfig = ToDomain.DomainConfig.newBuilder()
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

    private List<String> buildGroupScopeIdentityLookups(Bytes32 salt, String [] members) throws IllegalArgumentException {
        // Salt must be a 32byte hex string
        if (salt == null || members == null) throw new IllegalArgumentException("salt and members are required for group");
        String saltHex = salt.toHex();

        // To deploy a new Privacy Group we need to collect unique endorsement addresses that
        // mask the identities of all the participants.
        // We use the salt of the privacy group to do this. This salt is basically a shared
        // secret between all parties that is used to mask their identities.
        List<String> lookups = new ArrayList<>(members.length);
        for (String member : members) {
            String[] locatorSplit = member.split("@");
            switch (locatorSplit.length) {
                case 1 -> {
                    lookups.add(locatorSplit[0] + "." + saltHex);
                }
                case 2 -> {
                    lookups.add(locatorSplit[0] + "." + saltHex + "@" + locatorSplit[1]);
                }
                default -> throw new IllegalArgumentException("invalid identity locator '%s'".formatted(member));
            }
        }
        return lookups;
    }

    @Override
    protected CompletableFuture<ToDomain.InitDeployResponse> initDeploy(ToDomain.InitDeployRequest request) {
        try {
            PenteConfiguration.PrivacyGroupConstructorParamsJSON params =
                    new ObjectMapper().readValue(request.getTransaction().getConstructorParamsJson(),
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

            ToDomain.InitDeployResponse.Builder response = ToDomain.InitDeployResponse.newBuilder();
            for (String lookup : buildGroupScopeIdentityLookups(params.group().salt(), params.group().members())) {
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
        List<String> lookups = buildGroupScopeIdentityLookups(salt, members);
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
            PenteConfiguration.PrivacyGroupConstructorParamsJSON params =
                    new ObjectMapper().readValue(request.getTransaction().getConstructorParamsJson(),
                            PenteConfiguration.PrivacyGroupConstructorParamsJSON.class);

            List<Address> resolvedVerifiers = getResolvedEndorsers(params.group().salt(), params.group().members(), request.getResolvedVerifiersList());
            ByteArrayOutputStream onchainConfBuilder = new ByteArrayOutputStream();
            onchainConfBuilder.write(PenteConfiguration.PenteConfigID_Endorsement_V0);
            onchainConfBuilder.write(PenteConfiguration.abiEncoder_Endorsement_V0(new PenteConfiguration.Endorsement_V0(
                    resolvedVerifiers.size(),
                    resolvedVerifiers
            )).getBytes());
            ToDomain.PrepareDeployResponse.Builder response = ToDomain.PrepareDeployResponse.newBuilder();
            response.getTransactionBuilder().
                    setFunctionName("newPrivacyGroup").
                    setParamsJson(new ObjectMapper().writeValueAsString(new PenteConfiguration.NewPrivacyGroupFactoryParams(
                            new Bytes32(request.getTransaction().getTransactionId()),
                            new JsonHex.Bytes(onchainConfBuilder.toByteArray())
                    )));
            return CompletableFuture.completedFuture(response.build());
        } catch(Exception e) {
            return CompletableFuture.failedFuture(e);
        }
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
}
