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

package io.kaleido.paladin.pente.domain.helpers;

import com.fasterxml.jackson.databind.JsonNode;
import io.kaleido.paladin.testbed.Testbed;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.pente.domain.PenteConfiguration.GroupTupleJSON;
import io.kaleido.paladin.toolkit.*;

import java.io.IOException;
import java.util.HashMap;
import java.util.List;

public class NotoHelper {
    final String domainName;
    final Testbed testbed;
    final JsonABI abi;
    final String address;

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record ConstructorParams(
            @JsonProperty
            String notary,
            @JsonProperty
            String notaryMode,
            @JsonProperty
            OptionsParams options
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record OptionsParams(
            @JsonProperty
            HookParams hooks
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record HookParams(
            @JsonProperty
            String publicAddress,
            @JsonProperty
            JsonHex.Address privateAddress,
            @JsonProperty
            GroupTupleJSON privateGroup
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record NotoCoin(
            @JsonProperty
            NotoCoinData data
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record NotoCoinData(
            @JsonProperty
            String owner,
            @JsonProperty
            String amount
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record ApproveExtraParams(
            @JsonProperty
            JsonHex.Bytes data
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record PublicTransaction(
            @JsonProperty
            JsonABI.Entry functionABI,
            @JsonProperty
            JsonNode paramsJSON,
            @JsonProperty
            JsonHex.Bytes encodedCall
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record NotoTransferMetadata(
            @JsonProperty
            ApproveExtraParams approvalParams,
            @JsonProperty
            PublicTransaction transferWithApproval
    ) {
    }

    public static NotoHelper deploy(String domainName, String from, Testbed testbed, ConstructorParams params) throws IOException {
        String address = testbed.getRpcClient().request("testbed_deploy", domainName, from, params);
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                NotoHelper.class.getClassLoader(),
                "contracts/domains/interfaces/INotoPrivate.sol/INotoPrivate.json",
                "abi"
        );
        return new NotoHelper(domainName, testbed, abi, address);
    }

    private NotoHelper(String domainName, Testbed testbed, JsonABI abi, String address) {
        this.domainName = domainName;
        this.testbed = testbed;
        this.abi = abi;
        this.address = address;
    }

    public String address() {
        return address;
    }

    public List<NotoCoin> queryStates(JsonHex.Bytes32 schemaID, JsonQuery.Query query) throws IOException {
        List<HashMap<String, Object>> states = testbed.getRpcClient().request("pstate_queryContractStates",
                domainName, address, schemaID, query, "available");
        var mapper = new ObjectMapper();
        return states.stream().map(state -> mapper.convertValue(state, NotoCoin.class)).toList();
    }

    public void mint(String sender, String to, int amount) throws IOException {
        testbed.getRpcClient().request("testbed_invoke", new Testbed.TransactionInput(
                "private",
                "",
                sender,
                JsonHex.addressFrom(address),
                new HashMap<>() {{
                    put("to", to);
                    put("amount", amount);
                    put("data", "0x");
                }},
                abi,
                "mint"
        ), true);
    }

    public void transfer(String sender, String to, int amount) throws IOException {
        testbed.getRpcClient().request("testbed_invoke", new Testbed.TransactionInput(
                "private",
                "",
                sender,
                JsonHex.addressFrom(address),
                new HashMap<>() {{
                    put("to", to);
                    put("amount", amount);
                    put("data", "0x");
                }},
                abi,
                "transfer"
        ), true);
    }

    public Testbed.TransactionResult prepareTransfer(String sender, String to, int amount) throws IOException {
        return TestbedHelper.getTransactionResult(
                testbed.getRpcClient().request("testbed_prepare", new Testbed.TransactionInput(
                        "private",
                        "",
                        sender,
                        JsonHex.addressFrom(address),
                        new HashMap<>() {{
                            put("to", to);
                            put("amount", amount);
                            put("data", "0x");
                        }},
                        abi,
                        "transfer"
                )));
    }

    public void approveTransfer(String sender, List<Testbed.StateEncoded> inputs, List<Testbed.StateEncoded> outputs, JsonHex.Bytes data, String delegate) throws IOException {
        testbed.getRpcClient().request("testbed_invoke", new Testbed.TransactionInput(
                "private",
                "",
                sender,
                JsonHex.addressFrom(address),
                new HashMap<>() {{
                    put("inputs", inputs);
                    put("outputs", outputs);
                    put("data", data);
                    put("delegate", delegate);
                }},
                abi,
                "approveTransfer"
        ), true);
    }
}
