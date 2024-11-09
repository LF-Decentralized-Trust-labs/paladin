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

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.pente.domain.PenteConfiguration;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;

import java.io.IOException;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public class PenteHelper {

    final String domainName;
    final Testbed testbed;
    final PenteConfiguration.GroupTupleJSON groupInfo;
    final String address;

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record PenteCallOutputJSON(
            @JsonProperty
            String output
    ) {
    }

    public static PenteHelper newPrivacyGroup(String domainName, String from, Testbed testbed, PenteConfiguration.GroupTupleJSON groupInfo, boolean externalCallsEnabled) throws IOException {
        String address = testbed.getRpcClient().request("testbed_deploy",
                domainName, from, 
                new PenteConfiguration.PrivacyGroupConstructorParamsJSON(
                        groupInfo,
                        "shanghai",
                        PenteConfiguration.ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
                        externalCallsEnabled
                ));
        return new PenteHelper(domainName, testbed, groupInfo, address);
    }

    private PenteHelper(String domainName, Testbed testbed, PenteConfiguration.GroupTupleJSON groupInfo, String address) {
        this.domainName = domainName;
        this.testbed = testbed;
        this.groupInfo = groupInfo;
        this.address = address;
    }

    public String address() {
        return address;
    }

    private static Testbed.TransactionResult getTransactionInfo(LinkedHashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    public JsonHex.Address deploy(String sender, String bytecode, JsonABI.Parameters inputABI, Object inputValues) throws IOException {
        JsonABI.Entry deployABI = JsonABI.newFunction(
                "deploy",
                JsonABI.newParameters(
                        JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                JsonABI.newParameter("salt", "bytes32"),
                                JsonABI.newParameter("members", "string[]")
                        )),
                        JsonABI.newParameter("bytecode", "bytes"),
                        JsonABI.newTuple("inputs", "", inputABI)
                ),
                JsonABI.newParameters()
        );

        var tx = getTransactionInfo(
                testbed.getRpcClient().request("testbed_invoke", new Testbed.TransactionInput(
                        "private",
                        "",
                        sender,
                        JsonHex.addressFrom(address),
                        new HashMap<>() {{
                            put("group", groupInfo);
                            put("bytecode", bytecode);
                            put("inputs", inputValues);
                        }},
                        new JsonABI(List.of(deployABI)),
                        ""
                ), true));
        var domainData = new ObjectMapper().convertValue(tx.domainData(), PenteConfiguration.DomainData.class);
        return domainData.contractAddress();
    }

    public void invoke(String methodName, JsonABI.Parameters inputParams, String sender, JsonHex.Address privateAddress, Object inputValues) throws IOException {
        JsonABI.Entry invokeABI = JsonABI.newFunction(
                methodName,
                JsonABI.newParameters(
                        JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                JsonABI.newParameter("salt", "bytes32"),
                                JsonABI.newParameter("members", "string[]")
                        )),
                        JsonABI.newParameter("to", "address"),
                        JsonABI.newTuple("inputs", "", inputParams)
                ),
                JsonABI.newParameters()
        );

        testbed.getRpcClient().request("testbed_invoke",
                new Testbed.TransactionInput(
                        "private",
                        "",
                        sender,
                        JsonHex.addressFrom(address),
                        new HashMap<>() {{
                            put("group", groupInfo);
                            put("to", privateAddress);
                            put("inputs", inputValues);
                        }},
                        new JsonABI(List.of(invokeABI)),
                        ""
                ), true);
    }

    public PenteCallOutputJSON call(String methodName, JsonABI.Parameters inputParams, JsonABI.Parameters outputParams, String sender, JsonHex.Address privateAddress, Object inputValues) throws IOException {
        JsonABI.Entry callABI = JsonABI.newFunction(
                methodName,
                JsonABI.newParameters(
                        JsonABI.newTuple("group", "Group", JsonABI.newParameters(
                                JsonABI.newParameter("salt", "bytes32"),
                                JsonABI.newParameter("members", "string[]")
                        )),
                        JsonABI.newParameter("to", "address"),
                        JsonABI.newTuple("inputs", "", inputParams)
                ),
                outputParams
        );

        var queryResult = testbed.getRpcClient().request("testbed_call",
                new Testbed.TransactionInput(
                        "private",
                        "",
                        sender,
                        JsonHex.addressFrom(address),
                        new HashMap<>() {{
                            put("group", groupInfo);
                            put("to", privateAddress);
                            put("inputs", inputValues);
                        }},
                        new JsonABI(List.of(callABI)),
                        ""
                ), "");
        return new ObjectMapper().convertValue(queryResult, PenteCallOutputJSON.class);
    }

    public Testbed.TransactionResult prepare(String sender, JsonABI.Entry fn, Map<String, Object> inputs) throws IOException {
        return getTransactionInfo(
                testbed.getRpcClient().request("testbed_prepare", new Testbed.TransactionInput(
                        "private",
                        "",
                        sender,
                        JsonHex.addressFrom(address),
                        inputs,
                        new JsonABI(List.of(fn)),
                        "")));
    }
}
