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

import com.fasterxml.jackson.core.JsonGenerator;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.DeserializationContext;
import com.fasterxml.jackson.databind.JsonDeserializer;
import com.fasterxml.jackson.databind.JsonSerializer;
import com.fasterxml.jackson.databind.SerializerProvider;
import com.fasterxml.jackson.databind.annotation.JsonDeserialize;
import com.fasterxml.jackson.databind.annotation.JsonSerialize;

import java.io.IOException;
import java.math.BigInteger;

/** helper utility for hex bytes */
public abstract class JsonHexNum {

    public static final BigInteger MAX_UINT256 = BigInteger.valueOf(2).pow(256).subtract(BigInteger.valueOf(1));

    private final BigInteger intVal;

    protected JsonHexNum(BigInteger intVal) throws IllegalArgumentException {
        this.intVal = intVal;
    }

    @JsonDeserialize(using = JsonDeserializerUint256.class)
    @JsonSerialize(using = JsonSerializerUint256.class)
    public static class Uint256 extends JsonHexNum {
        public static final Uint256 ZERO = new Uint256(0);

        public Uint256(long intVal) {
            this(BigInteger.valueOf(intVal));
        }
        public Uint256(BigInteger intVal) {
            super(intVal);
            if (intVal.signum() < 0) {
                throw new IllegalArgumentException("negative value for uint256");
            }
            if (intVal.compareTo(MAX_UINT256) > 0) {
                throw new IllegalArgumentException("value cannot be stored in 256bits");
            }
        }

        public static Uint256 fromString(String strValue) {
            if (strValue.startsWith("0x"))  {
                return new Uint256(new BigInteger(strValue.substring(2), 16));
            }
            return new Uint256(new BigInteger(strValue, 10));
        }

        public static Uint256 fromBigIntZeroNull(BigInteger intVal) {
            if (intVal == null) return ZERO;
            return new Uint256(intVal);
        }

    }

    public static class JsonDeserializerUint256 extends JsonDeserializer<Uint256> {
        @Override
        public Uint256 deserialize(JsonParser jp, DeserializationContext ctxt) throws IOException {
            return Uint256.fromString(deserializeStr(jp));
        }
    }

    public static class JsonSerializerUint256 extends JsonSerializer<Uint256> {
        @Override
        public void serialize(Uint256 value, JsonGenerator gen, SerializerProvider serializers) throws IOException {
            gen.writeString(value.toString());
        }
    }

    public BigInteger bigInt() {
        return intVal;
    }

    public long longValue() {
        return bigInt().longValue();
    }

    public int intValue() {
        return bigInt().intValue();
    }

    public String toString() {
        return "0x" + bigInt().toString(16);
    }

    private static String deserializeStr(JsonParser jp) throws IOException {
        String strValue = jp.getValueAsString();
        if (strValue == null) {
            throw new IOException("JSON value must be a string");
        }
        return strValue;
    }

}
