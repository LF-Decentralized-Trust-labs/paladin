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

package io.kaleido.pente.evmstate;

import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.datatypes.Hash;
import org.hyperledger.besu.evm.account.Account;
import org.hyperledger.besu.evm.worldstate.WorldState;
import org.apache.tuweni.bytes.Bytes32;

import java.util.stream.Stream;

public class InMemoryWorldState implements WorldState {

    @Override
    public Hash rootHash() {
        return null;
    }

    @Override
    public Hash frontierRootHash() {
        return null;
    }

    @Override
    public Stream<StreamableAccount> streamAccounts(Bytes32 bytes32, int i) {
        return null;
    }

    @Override
    public Account get(Address address) {
        return null;
    }
}
