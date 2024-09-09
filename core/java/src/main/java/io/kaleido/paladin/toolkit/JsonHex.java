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

package io.kaleido.paladin.toolkit;

import com.fasterxml.jackson.core.JsonGenerator;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.DeserializationContext;
import com.fasterxml.jackson.databind.JsonDeserializer;
import com.fasterxml.jackson.databind.JsonSerializer;
import com.fasterxml.jackson.databind.SerializerProvider;
import com.fasterxml.jackson.databind.annotation.JsonDeserialize;
import com.fasterxml.jackson.databind.annotation.JsonSerialize;
import org.bouncycastle.jcajce.provider.digest.Keccak;

import java.io.IOException;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.Random;

/** helper utility for hex bytes */
public abstract class JsonHex {

    private final int requiredByteLength;

    private final byte[] bytes;

    protected JsonHex(String str, int requiredByteLength) throws IllegalArgumentException {
        this(HexFormat.of().parseHex(trimOxPrefix(str)), requiredByteLength);
    }
    protected JsonHex(byte[] bytes, int requiredByteLength) throws IllegalArgumentException {
        this.bytes = bytes;
        this.requiredByteLength = requiredByteLength;
        if (requiredByteLength >= 0 && bytes.length != requiredByteLength) {
            throw new IllegalArgumentException("required length %d (length %d)".formatted(requiredByteLength, bytes.length));
        }
    }

    @JsonDeserialize(using = JsonDeserializerDynamic.class)
    @JsonSerialize(using = JsonSerializerDynamic.class)
    public static class Bytes extends JsonHex {
        public Bytes(String str) {
            super(str, -1);
        }
        public Bytes(byte[] bytes) {
            super(bytes, -1);
        }
    }

    public static class JsonDeserializerDynamic extends JsonDeserializer<Bytes> {
        @Override
        public Bytes deserialize(JsonParser jp, DeserializationContext ctxt) throws IOException {
            return new Bytes(deserializeStr(jp));
        }
    }

    public static class JsonSerializerDynamic extends JsonSerializer<Bytes> {
        @Override
        public void serialize(Bytes value, JsonGenerator gen, SerializerProvider serializers) throws IOException {
            gen.writeString(value.to0xHex());
        }
    }

    @JsonDeserialize(using = JsonDeserializerBytes32.class)
    @JsonSerialize(using = JsonSerializerBytes32.class)
    public static class Bytes32 extends JsonHex {
        public Bytes32(String str) {
            super(str, 32);
        }
        public Bytes32(byte[] bytes) {
            super(bytes, 32);
        }
    }

    public static class JsonDeserializerBytes32 extends JsonDeserializer<Bytes32> {
        @Override
        public Bytes32 deserialize(JsonParser jp, DeserializationContext ctxt) throws IOException {
            return new Bytes32(deserializeStr(jp));
        }
    }

    public static class JsonSerializerBytes32 extends JsonSerializer<Bytes32> {
        @Override
        public void serialize(Bytes32 value, JsonGenerator gen, SerializerProvider serializers) throws IOException {
            gen.writeString(value.to0xHex());
        }
    }

    @JsonDeserialize(using = JsonDeserializerAddress.class)
    @JsonSerialize(using = JsonSerializerAddress.class)
    public static class Address extends JsonHex {
        public Address(String str) {super(str, 20);}
        public Address(byte[] bytes) {
            super(bytes, 20);
        }
        public String checksummed() {

            // EIP-55: Mixed-case checksum address encoding
            // https://eips.ethereum.org/EIPS/eip-55

            Keccak.DigestKeccak kecc = new Keccak.Digest256();
            byte[] hexBytes = toHex().getBytes(StandardCharsets.UTF_8);
            kecc.update(hexBytes, 0, hexBytes.length);
            String hexHash = new Bytes32(kecc.digest()).toHex();

            String hexAddr = this.toHex();
            StringBuilder out = new StringBuilder(42);
            out.append("0x");
            for (int i = 0; i < 40; i++) {
                int hexHashDigit = Integer.parseInt(hexHash.substring(i, i+1), 16);
                if (hexHashDigit >= 8) {
                    out.append(Character.toUpperCase(hexAddr.charAt(i)));
                } else {
                    out.append(Character.toLowerCase(hexAddr.charAt(i)));
                }
            }
            return out.toString();
        }
    }

    public static class JsonDeserializerAddress extends JsonDeserializer<Address> {
        @Override
        public Address deserialize(JsonParser jp, DeserializationContext ctxt) throws IOException {
            return new Address(deserializeStr(jp));
        }
    }

    public static class JsonSerializerAddress extends JsonSerializer<Address> {
        @Override
        public void serialize(Address value, JsonGenerator gen, SerializerProvider serializers) throws IOException {
            gen.writeString(value.to0xHex());
        }
    }

    /** only used in from(str, requiredByteLength) - do not use as a JSON type */
    public static class FixedLenBytes extends JsonHex {
        private FixedLenBytes(String str, int requiredByteLength) {super(str, requiredByteLength);}
        private FixedLenBytes(byte[] bytes, int requiredByteLength) {super(bytes, requiredByteLength);}
    }


    private static String deserializeStr(JsonParser jp) throws IOException {
        String strValue = jp.getValueAsString();
        if (strValue == null) {
            throw new IOException("JSON value must be a string");
        }
        return strValue;
    }

    public static String trimOxPrefix(String s) {
        return trimPrefix(s, "0x");
    }

    public static String trimPrefix(String s, String prefix) {
        if (s == null || s.isEmpty()) {
            return "";
        }
        if (s.length() < prefix.length()) {
            return s;
        }
        for (int i = 0; i < prefix.length(); i++) {
            if (s.charAt(i) != prefix.charAt(i)) {
                return s;
            }
        }
        return s.substring(prefix.length());
    }

    @Override
    public boolean equals(final Object obj) {
        return (obj instanceof JsonHex) && Arrays.equals(this.bytes, ((JsonHex)obj).bytes);
    }

    public static Bytes from(String str) throws IllegalArgumentException {
        return new Bytes(str);
    }

    public static FixedLenBytes from(String str, int requiredByteLength) throws IllegalArgumentException {
        return new FixedLenBytes(str, requiredByteLength);
    }

    public static Bytes wrap(byte[] bytes) throws IllegalArgumentException {
        return new Bytes(bytes);
    }

    public static Bytes from(byte[] buffer, int offset, int len) throws IllegalArgumentException {
        return new Bytes(Arrays.copyOfRange(buffer, offset, len));
    }

    public static FixedLenBytes wrap(byte[] bytes, int requiredByteLength) throws IllegalArgumentException {
        return new FixedLenBytes(bytes, requiredByteLength);
    }

    public static Address addressFrom(String str) throws IllegalArgumentException {
        return new JsonHex.Address(str);
    }

    public static Bytes32 randomBytes32() throws IllegalArgumentException {
        Random rand = new Random();
        byte[] bytes = new byte[32];
        rand.nextBytes(bytes);
        return new Bytes32(bytes);
    }

    public static Bytes randomBytes(int len) throws IllegalArgumentException {
        Random rand = new Random();
        byte[] bytes = new byte[len];
        rand.nextBytes(bytes);
        return new Bytes(bytes);
    }

    public String toString() {
        return to0xHex();
    }

    public String toHex() {
        return HexFormat.of().formatHex(bytes);
    }

    public String to0xHex() {
        return "0x" + toHex();
    }

    public int getRequiredByteLength() {
        return requiredByteLength;
    }

    public byte[] getBytes() {
        return bytes;
    }


}
