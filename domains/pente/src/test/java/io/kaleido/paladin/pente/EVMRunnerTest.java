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

package io.kaleido.paladin.pente;

import io.kaleido.paladin.pente.evmrunner.EVMRunner;
import io.kaleido.paladin.pente.evmstate.PersistedAccount;
import org.hyperledger.besu.datatypes.Address;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.assertEquals;

public class EVMRunnerTest {

    @Test
    void testNonceSmartContractAddress() throws Exception {
        var addr = Address.fromHexString("0x6ac7ea33f8831ea9dcc53393aaa88b25a785dbf0");
        assertEquals("0xcd234a471b72ba2f1ccf0a70fcaba648a5eecd8d", EVMRunner.nonceSmartContractAddress(addr, 0).toHexString());
        assertEquals("0x343c43a37d37dff08ae8c4a11544c718abb4fcf8", EVMRunner.nonceSmartContractAddress(addr, 1).toHexString());
    }
}
