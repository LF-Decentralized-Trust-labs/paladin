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

package io.kaleido.evm;

import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.DelegatingBytes;
import org.hyperledger.besu.datatypes.Address;

import java.util.Collection;
import java.util.Optional;
import java.util.stream.Collectors;
import java.util.stream.Stream;

public class TestUtils {

    static String sortedAddressList(Collection<Address> addresses) {
        return addresses.stream().map(DelegatingBytes::toString).sorted().collect(Collectors.joining(","));
    }

    static boolean listContains(Collection<?> entries, String stringMatch) {
        return entries.stream().map(Object::toString).anyMatch(e -> e.equals(stringMatch));
    }

    static <T> Optional<T> firstNonMatch(Collection<T> entries, T ...excludes) {
        return entries.stream().filter(e -> Stream.of(excludes).noneMatch(e1 -> e1.toString().equals(e.toString()))).findFirst();
    }

}
