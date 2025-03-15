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
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.ByteString;
import com.google.protobuf.util.JsonFormat;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Test;
import org.skyscreamer.jsonassert.JSONAssert;
import org.skyscreamer.jsonassert.JSONCompareMode;

import java.io.IOException;
import java.io.StringReader;
import java.util.*;
import java.util.concurrent.ExecutionException;

import static org.junit.jupiter.api.Assertions.*;

public class PentePrivacyGroupTest {

    @Test
    void tetConfigurePrivacyGroupDefaults() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = ConfigurePrivacyGroupRequest.newBuilder();

        // Run it
        var res = pente.configurePrivacyGroup(reqBuilder.build()).get();

        // Check the resulting state
        var expected = new ObjectMapper().readValue("""
                {
                    "evmVersion":"shanghai",
                    "endorsementType":"group_scoped_identities",
                    "externalCallsEnabled":"false"
                 }
                """, new TypeReference<Map<String, String>>() {});
        assertEquals(expected, res.getConfigurationMap());

    }

    @Test
    void testConfigurePrivacyGroupCustom() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = ConfigurePrivacyGroupRequest.newBuilder()
                .putInputConfiguration("evmVersion", "london")
                .putInputConfiguration("endorsementType", "some_other_option")
                .putInputConfiguration("externalCallsEnabled", "true");

        // Run it
        var res = pente.configurePrivacyGroup(reqBuilder.build()).get();

        // Check the resulting state
        var expected = new ObjectMapper().readValue("""
                {
                    "evmVersion":"london",
                    "endorsementType":"some_other_option",
                    "externalCallsEnabled":"true"
                 }
                """, new TypeReference<Map<String, String>>() {});
        assertEquals(expected, res.getConfigurationMap());

    }

    @Test
    void testConfigurePrivacyGroupBadOption() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = ConfigurePrivacyGroupRequest.newBuilder()
                .putInputConfiguration("evmversion", "mis-spelled");

        assertThrows(ExecutionException.class, () -> pente.configurePrivacyGroup(reqBuilder.build()).get());
    }

    @Test
    void testInitPrivacyGroupDefaults() throws Exception {

        var pente = new PenteDomain("", "");

        var salt = JsonHex.randomBytes32();
        var reqBuilder = InitPrivacyGroupRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId(salt.toString())
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build());

        // Run it
        var res = pente.initPrivacyGroup(reqBuilder.build()).get();

        // Check the resulting state
        var expected = new ObjectMapper().readValue("""
                {
                    "name":"bob",
                    "salt":"REPLACE",
                    "pente": {
                        "members":["me@node1","you@node2"],
                        "evmVersion":"shanghai",
                        "endorsementType":"group_scoped_identities",
                        "externalCallsEnabled":false
                    }
                 }
                """, new TypeReference<Map<Object, Object>>() {});

        // Check the resulting transaction ABI
        expected = new ObjectMapper().readValue("""
                {
                    "type": "constructor",
                    "inputs": [
                        {
                            "name": "group",
                            "type": "tuple",
                            "internalType": "struct Group",
                            "components": [
                                { "name": "salt", "type": "bytes32" },
                                { "name": "members", "type": "string[]" }
                            ]
                        },
                        {"name":"evmVersion", "type":"string"},
                        {"name":"endorsementType", "type":"string"},
                        {"name":"externalCallsEnabled", "type":"bool"}
                    ]
                }
                """, new TypeReference<>() {});
        var received = new ObjectMapper().readValue(res.getTransaction().getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void testInitPrivacyGroupCustomOptions() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = InitPrivacyGroupRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x33219f6966a20f20bdbf06d3039349a4959c68208fbcda636568e766070a7d4d")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .putConfiguration("evmVersion", "london")
                        .putConfiguration("endorsementType", "some_future_option")
                        .putConfiguration("externalCallsEnabled", "true")
                        .build());

        // Run it
        var res = pente.initPrivacyGroup(reqBuilder.build()).get();

        // Check the resulting transaction parameters
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                       "salt": "0x33219f6966a20f20bdbf06d3039349a4959c68208fbcda636568e766070a7d4d",
                       "members": ["me@node1","you@node2"]
                    },
                    "evmVersion": "london",
                    "endorsementType": "some_future_option",
                    "externalCallsEnabled": true
                }
                """, new TypeReference<>() {});
        var received = new ObjectMapper().readValue(res.getTransaction().getParamsJson(), new TypeReference<>() {});
        assertEquals(expected, received);


        // Check the resulting transaction ABI
        expected = new ObjectMapper().readValue("""
                {
                    "type": "constructor",
                    "inputs": [
                        {
                            "name": "group",
                            "type": "tuple",
                            "internalType": "struct Group",
                            "components": [
                                { "name": "salt", "type": "bytes32" },
                                { "name": "members", "type": "string[]" }
                            ]
                        },
                        {"name":"evmVersion", "type":"string"},
                        {"name":"endorsementType", "type":"string"},
                        {"name":"externalCallsEnabled", "type":"bool"}
                    ]
                }
                """, new TypeReference<>() {});
        received = new ObjectMapper().readValue(res.getTransaction().getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);
    }

    @Test
    void wrapPrivacyGroupTransactionDeployWithFuncABI() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                    .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                    .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                    .setFrom("submitter.address")
                    .setFunctionAbiJson("""
                        {"type": "constructor", "inputs": [ { "name": "input1", "type": "string" } ] }
                     """)
                    .setInputJson("""
                        {"input1":"value1"}
                    """)
                    .build()
                );

        // Run it
        var res = pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get();
        var resTx = res.getTransaction();

        assertEquals(PreparedTransaction.TransactionType.PRIVATE, resTx.getType());

        // Check the resulting transaction
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                        "salt": "0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2",
                        "members": [ "me@node1", "you@node2" ]
                    },
                    "inputs": { "input1": "value1" },
                    "bytecode": "0xfeedbeef"
                 }
                """, new TypeReference<Map<Object, Object>>() {});
        var received = new ObjectMapper().readValue(resTx.getParamsJson(), new TypeReference<Map<Object, Object>>() {});
        assertEquals(expected, received);

        // Check the resulting ABI for the TX
        expected = new ObjectMapper().readValue("""
                {
                    "type": "function",
                    "name": "deploy",
                    "inputs": [
                       {
                           "name": "group",
                           "type": "tuple",
                           "internalType": "struct Group",
                           "components": [
                               { "name": "salt", "type": "bytes32" },
                               { "name": "members", "type": "string[]" }
                           ]
                       },
                       { "name": "bytecode", "type": "bytes" },
                       {
                            "name": "inputs",
                            "type": "tuple",
                            "internalType": "struct Inputs",
                            "components": [
                                { "name": "input1", "type": "string" }
                            ]
                       }
                    ],
                    "outputs": []
                }
                """, new TypeReference<>() {});
        received = new ObjectMapper().readValue(resTx.getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void wrapPrivacyGroupInvokeWithFuncABI() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                        .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                        .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                        .setFrom("submitter.address")
                        .setTo("0x449984cefadce394740a410c7d832a5e2207c27a")
                        .setFunctionAbiJson("""
                        {"type": "function", "name": "doAThing", "inputs": [ { "name": "thing", "type": "string" } ], "outputs":  [ { "name": "done", "type": "bool" } ] }
                     """)
                        .setInputJson("""
                        {"thing":"one"}
                    """)
                        .build()
                );

        // Run it
        var res = pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get();
        var resTx = res.getTransaction();

        assertEquals(PreparedTransaction.TransactionType.PRIVATE, resTx.getType());

        // Check the resulting transaction
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                        "salt": "0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2",
                        "members": [ "me@node1", "you@node2" ]
                    },
                    "to": "0x449984cefadce394740a410c7d832a5e2207c27a",
                    "inputs": { "thing": "one" }
                 }
                """, new TypeReference<Map<Object, Object>>() {});
        var received = new ObjectMapper().readValue(resTx.getParamsJson(), new TypeReference<Map<Object, Object>>() {});
        assertEquals(expected, received);

        // Check the resulting ABI for the TX
        expected = new ObjectMapper().readValue("""
                {
                    "type": "function",
                    "name": "doAThing",
                    "inputs": [
                       {
                           "name": "group",
                           "type": "tuple",
                           "internalType": "struct Group",
                           "components": [
                               { "name": "salt", "type": "bytes32" },
                               { "name": "members", "type": "string[]" }
                           ]
                       },
                       { "name": "to", "type": "address" },
                       {
                            "name": "inputs",
                            "type": "tuple",
                            "internalType": "struct Inputs",
                            "components": [
                                { "name": "thing", "type": "string" }
                            ]
                       }
                    ],
                    "outputs": [{ "name": "done", "type": "bool" }]
                }
                """, new TypeReference<Map<Object, Object>>() {});
        received = new ObjectMapper().readValue(resTx.getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void wrapPrivacyGroupInvokeWithNoInputs() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                        .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                        .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                        .setFrom("submitter.address")
                        .setTo("0x449984cefadce394740a410c7d832a5e2207c27a")
                        .setFunctionAbiJson("""
                        {"type": "function", "name": "doAThing" }
                     """)
                        .setInputJson("null")
                        .build()
                );

        // Run it
        var res = pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get();
        var resTx = res.getTransaction();

        assertEquals(PreparedTransaction.TransactionType.PRIVATE, resTx.getType());

        // Check the resulting transaction
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                        "salt": "0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2",
                        "members": [ "me@node1", "you@node2" ]
                    },
                    "to": "0x449984cefadce394740a410c7d832a5e2207c27a",
                    "inputs": { }
                 }
                """, new TypeReference<Map<Object, Object>>() {});
        var received = new ObjectMapper().readValue(resTx.getParamsJson(), new TypeReference<Map<Object, Object>>() {});
        assertEquals(expected, received);

        // Check the resulting ABI for the TX
        expected = new ObjectMapper().readValue("""
                {
                    "type": "function",
                    "name": "doAThing",
                    "inputs": [
                       {
                           "name": "group",
                           "type": "tuple",
                           "internalType": "struct Group",
                           "components": [
                               { "name": "salt", "type": "bytes32" },
                               { "name": "members", "type": "string[]" }
                           ]
                       },
                       { "name": "to", "type": "address" },
                       {
                            "name": "inputs",
                            "type": "tuple",
                            "internalType": "struct Inputs"
                       }
                    ],
                    "outputs": []
                }
                """, new TypeReference<Map<Object, Object>>() {});
        received = new ObjectMapper().readValue(resTx.getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void wrapPrivacyGroupInvokeSimpleTransfer() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                        .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                        .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                        .setFrom("submitter.address")
                        .setTo("0x449984cefadce394740a410c7d832a5e2207c27a")
                        .setGas("0x112233")
                        .setValue("0x99999999999999999999")
                        .build()
                );

        // Run it
        var res = pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get();
        var resTx = res.getTransaction();

        assertEquals(PreparedTransaction.TransactionType.PRIVATE, resTx.getType());

        // Check the resulting transaction
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                        "salt": "0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2",
                        "members": [ "me@node1", "you@node2" ]
                    },
                    "to": "0x449984cefadce394740a410c7d832a5e2207c27a",
                    "gas": "0x112233",
                    "value": "0x99999999999999999999"
                 }
                """, new TypeReference<Map<Object, Object>>() {});
        var received = new ObjectMapper().readValue(resTx.getParamsJson(), new TypeReference<Map<Object, Object>>() {});
        assertEquals(expected, received);

        // Check the resulting ABI for the TX
        expected = new ObjectMapper().readValue("""
                {
                    "type": "function",
                    "name": "invoke",
                    "inputs": [
                       {
                           "name": "group",
                           "type": "tuple",
                           "internalType": "struct Group",
                           "components": [
                               { "name": "salt", "type": "bytes32" },
                               { "name": "members", "type": "string[]" }
                           ]
                       },
                       { "name": "to", "type": "address" },
                       { "name": "gas", "type": "uint256"},
                       { "name": "value", "type":"uint256"}
                    ],
                    "outputs": []
                }
                """, new TypeReference<Map<Object, Object>>() {});
        received = new ObjectMapper().readValue(resTx.getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void wrapPrivacyGroupInvokePreEncodedFuncCall() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                        .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                        .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                        .setFrom("submitter.address")
                        .setTo("0x449984cefadce394740a410c7d832a5e2207c27a")
                        .setInputJson("\"0xfeedbeef\"")
                        .build()
                );

        // Run it
        var res = pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get();
        var resTx = res.getTransaction();

        assertEquals(PreparedTransaction.TransactionType.PRIVATE, resTx.getType());

        // Check the resulting transaction
        var expected = new ObjectMapper().readValue("""
                {
                    "group": {
                        "salt": "0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2",
                        "members": [ "me@node1", "you@node2" ]
                    },
                    "to": "0x449984cefadce394740a410c7d832a5e2207c27a",
                    "data": "0xfeedbeef"
                 }
                """, new TypeReference<Map<Object, Object>>() {});
        var received = new ObjectMapper().readValue(resTx.getParamsJson(), new TypeReference<Map<Object, Object>>() {});
        assertEquals(expected, received);

        // Check the resulting ABI for the TX
        expected = new ObjectMapper().readValue("""
                {
                    "type": "function",
                    "name": "invoke",
                    "inputs": [
                       {
                           "name": "group",
                           "type": "tuple",
                           "internalType": "struct Group",
                           "components": [
                               { "name": "salt", "type": "bytes32" },
                               { "name": "members", "type": "string[]" }
                           ]
                       },
                       { "name": "to", "type": "address" },
                       { "name": "data", "type":"bytes"}
                    ],
                    "outputs": []
                }
                """, new TypeReference<Map<Object, Object>>() {});
        received = new ObjectMapper().readValue(resTx.getFunctionAbiJson(), new TypeReference<>() {});
        assertEquals(expected, received);

    }

    @Test
    void wrapPrivacyGroupInvokePreEncodedObjectNoABI() throws Exception {

        var pente = new PenteDomain("", "");

        var reqBuilder = WrapPrivacyGroupEVMTXRequest.newBuilder()
                .setPrivacyGroup(PrivacyGroup.newBuilder()
                        .setId("0x4b9aa1d78daa2853de4ab875393ce0008085882181e18b22ba73f0fa916c32d2")
                        .addAllMembers(Arrays.asList("me@node1","you@node2"))
                        .build())
                .setTransaction(PrivacyGroupEVMTX.newBuilder()
                        .setContractInfo(ContractInfo.newBuilder().build() /* not currently used */)
                        .setBytecode(ByteString.copyFrom(JsonHex.from("0xfeedbeef").getBytes()))
                        .setFrom("submitter.address")
                        .setTo("0x449984cefadce394740a410c7d832a5e2207c27a")
                        .setInputJson("{}")
                        .build()
                );

        // Run it
        assertThrows(ExecutionException.class, () -> pente.wrapPrivacyGroupTransaction(reqBuilder.build()).get());

    }
}