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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
import io.kaleido.paladin.toolkit.JsonABI;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.web3j.abi.datatypes.Address;

import java.io.IOException;
import java.io.StringReader;

/**
 * Provides thread safe access to the configuration of the domain from the functions that are called
 * on any thread.
 **/
public class PenteConfiguration {
    private static final Logger LOGGER = LogManager.getLogger(PenteConfiguration.class);

    private final JsonABI factoryContractABI;

    private final JsonABI privacyGroupABI;

    private boolean initialized;

    private Address address;

    private PenteConfiguration() {
        try {
            factoryContractABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/testcontracts/PenteFactory.sol/PenteFactory.json",
                    "abi");
            privacyGroupABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(),
                    "contracts/testcontracts/PentePrivacyGroup.sol/PentePrivacyGroup.json",
                    "abi");
        } catch (Exception t) {
            LOGGER.error("failed to initialize configuration", t);
            throw new RuntimeException(t);
        }
    }

    JsonABI.Entry privateConstructor() {
        return JsonABI.newConstructor(JsonABI.newParameters(
            JsonABI.newParameter("lookup", "string"),
            JsonABI.newTupleArray("members", JsonABI.newParameters(
                    JsonABI.newParameter("lookup", "string")
            ))
        ));
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    private record PenteYAMLConfig(
            @JsonProperty
            String address
    ) {}

    synchronized JsonABI getFactoryContractABI() {
        return factoryContractABI;
    }

    synchronized JsonABI getPrivacyGroupABI() {
        return privacyGroupABI;
    }

    synchronized void checkInitialized() {
        if (!initialized) {
            throw new IllegalStateException("config not initialized");
        }
    }

    synchronized Address getAddress() {
        checkInitialized();
        return address;
    }

    synchronized void initFromYAML(String yamlConfig) {
        try {
            final ObjectMapper mapper = new ObjectMapper(new YAMLFactory());
            PenteYAMLConfig config = mapper.readValue(new StringReader(yamlConfig), PenteYAMLConfig.class);
            this.address = new Address(config.address());
            this.initialized = true;
        } catch(IOException e) {
            throw new IllegalArgumentException(e);
        }
    }

}
