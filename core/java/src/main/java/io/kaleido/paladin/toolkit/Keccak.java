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

import io.kaleido.paladin.toolkit.JsonHex.Bytes32;

import java.nio.charset.StandardCharsets;

public class Keccak {

    public static Bytes32 Hash(byte[] data) {
        var hash = new org.bouncycastle.jcajce.provider.digest.Keccak.Digest256();
        return new Bytes32(hash.digest(data));
    }

    /** hash the UTF-8 bytes of a string */
    public static Bytes32 Hash(String data) {
        var hash = new org.bouncycastle.jcajce.provider.digest.Keccak.Digest256();
        return new Bytes32(hash.digest(data.getBytes(StandardCharsets.UTF_8)));
    }
}
