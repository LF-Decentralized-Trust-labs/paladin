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

package org.lfdt.labs.paladin.toolkit;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.math.BigInteger;

import static org.junit.jupiter.api.Assertions.*;

public class JsonHexTest {

    @Test
    public void testDynamic() throws Exception {
        assertEquals("feedbeef", JsonHex.from("0xfEEdbEEf").toHex());
        assertEquals("0xfeedbeef", JsonHex.wrap(JsonHex.from("fEEdbEEf").getBytes()).to0xHex());
        assertEquals("0x", JsonHex.from("").toString());
        assertEquals("0x", JsonHex.from((String)(null)).toString());

        assertThrows(IllegalArgumentException.class, () -> {
            JsonHex.from("wrong");
        });
        assertThrows(IllegalArgumentException.class, () -> {
            JsonHex.from("0");
        });
    }

    @Test
    public void testFixed() throws Exception {
        assertEquals("0xfeedbeef", JsonHex.from("0xfEEdbEEf", 4).toString());
        assertEquals("0xfeedbeef", JsonHex.wrap(JsonHex.from("0xfEEdbEEf").getBytes(), 4).toString());
        assertThrows(IllegalArgumentException.class, () -> {
            JsonHex.from("0xfeedbeef", 10);
        });
        assertEquals("0x", JsonHex.from("", 0).toString());
        assertThrows(IllegalArgumentException.class, () -> {
            JsonHex.from("0x", 1);
        });
    }

    private record TestRecord(
            @JsonProperty()
            JsonHex.Bytes bytes,
            @JsonProperty()
            JsonHex.Bytes32 bytes32,
            @JsonProperty()
            JsonHex.Address address
    ) {};

    @Test
    public void testJsonParsing() throws Exception {
        TestRecord tr = new ObjectMapper().readValue("""
                {
                    "bytes": "AAbbCCdd",
                    "bytes32": "0x4783d50032169c868672a02ff005a7f222e9b0f9da1ac5f10814c5b03894cbff",
                    "address": "0x67e0aEcDbdA15B040978299B1dCFdff77c0C1dE8"
                }""", TestRecord.class);
        assertEquals("0xaabbccdd", tr.bytes().toString());
        assertEquals("0x4783d50032169c868672a02ff005a7f222e9b0f9da1ac5f10814c5b03894cbff", tr.bytes32().toString());
        assertEquals("0x67e0aecdbda15b040978299b1dcfdff77c0c1de8", tr.address().toString());
        assertEquals("0x67e0aEcDbdA15B040978299B1dCFdff77c0C1dE8", tr.address().checksummed());
        assertEquals(20, tr.address().getRequiredByteLength());

        // Check equals() works
        assertEquals(tr.bytes32(), JsonHex.from("4783d50032169c868672a02ff005a7f222e9b0f9da1ac5f10814c5b03894cbff"));

        assertThrows(IOException.class, () -> {
            new ObjectMapper().readValue("""
                {"bytes":{}}
                """,  TestRecord.class);
        });
        tr = new ObjectMapper().readValue("{}",  TestRecord.class);
        assertNull(tr.bytes());
        assertNull(tr.bytes32());
        assertNull(tr.address());

        tr = new TestRecord(
                JsonHex.from("AAbbCCdd"),
                new JsonHex.Bytes32("0x4783d50032169c868672a02ff005a7f222e9b0f9da1ac5f10814c5b03894cbff"),
                JsonHex.addressFrom("0x67e0aEcDbdA15B040978299B1dCFdff77c0C1dE8")
        );
        String jsonRecord = new ObjectMapper().writeValueAsString(tr);
        assertEquals("""
                {"bytes":"0xaabbccdd","bytes32":"0x4783d50032169c868672a02ff005a7f222e9b0f9da1ac5f10814c5b03894cbff","address":"0x67e0aecdbda15b040978299b1dcfdff77c0c1de8"}
                """.trim(), jsonRecord);
    }

    @Test
    public void testRandom() {
        JsonHex.Bytes32 b32 = JsonHex.randomBytes32();
        assertEquals(64, b32.toHex().length());
        JsonHex.Bytes b16 = JsonHex.randomBytes(16);
        assertEquals(32, b16.toHex().length());
    }

    private record TestRecordInt(
            @JsonProperty()
            JsonHexNum.Uint256 value
    ) {};


    @Test
    public void testUint256() throws Exception {

        var biggestUint256 = new JsonHexNum.Uint256(BigInteger.valueOf(2).pow(256).subtract(BigInteger.valueOf(1)));
        assertEquals("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", biggestUint256.toString());

        TestRecordInt tr = new ObjectMapper().readValue("""
                {"value":"0x11223344"}
                """, TestRecordInt.class);
        assertEquals(287454020L, tr.value.longValue());
        assertEquals(287454020, tr.value.intValue());
        assertEquals("0x11223344", tr.value.toString());
        assertEquals("{\"value\":\"0x11223344\"}", new ObjectMapper().writeValueAsString(tr));

        tr = new ObjectMapper().readValue("""
                {"value":"1122334455"}
                """, TestRecordInt.class);
        assertEquals(1122334455L, tr.value.longValue());

        // check we work with very large JSON strings, larger than long coudl store
        tr = new ObjectMapper().readValue("""
                {"value":11223344556677889900112233445566}
                """, TestRecordInt.class);
        assertEquals("0x8da89506792d2b6fb84a3538be", tr.value.toString());

        tr = new ObjectMapper().readValue("""
                {"value":"0x0"}
                """, TestRecordInt.class);
        assertEquals(0L, tr.value.bigInt().longValue());
        assertEquals("0x0", tr.value.toString());
        assertEquals("0x0", JsonHexNum.Uint256.fromString("0x00000000").toString());

        assertThrows(IllegalArgumentException.class, () -> {
            JsonHexNum.Uint256.fromString("0x010000000000000000000000000000000000000000000000000000000000000000").toString();
        });

        assertEquals(0L, JsonHexNum.Uint256.fromBigIntZeroNull(null).longValue());

    }
}
