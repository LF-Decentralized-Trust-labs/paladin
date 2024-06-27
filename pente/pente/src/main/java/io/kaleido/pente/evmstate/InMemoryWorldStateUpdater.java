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

import kotlin.NotImplementedError;
import org.hyperledger.besu.datatypes.Address;
import org.hyperledger.besu.evm.account.Account;
import org.hyperledger.besu.evm.frame.MessageFrame;
import org.hyperledger.besu.evm.internal.EvmConfiguration;
import org.hyperledger.besu.evm.worldstate.AbstractWorldUpdater;
import org.hyperledger.besu.evm.worldstate.WorldState;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collection;
import java.util.HashSet;
import java.util.Set;

public class InMemoryWorldStateUpdater extends AbstractWorldUpdater<InMemoryWorldState,Account> {

    private final Logger logger = LoggerFactory.getLogger(InMemoryWorldStateUpdater.class);

    private final InMemoryWorldState world;

    public InMemoryWorldStateUpdater(InMemoryWorldState world, EvmConfiguration evmConfiguration) {
        super(world, evmConfiguration);
        this.world = world;
    }

    @Override
    protected Account getForMutation(Address address) {
        return world.get(address);
    }

    @Override
    public Collection<? extends Account> getTouchedAccounts() {
        return getUpdatedAccounts();
    }

    @Override
    public Collection<Address> getDeletedAccountAddresses() {
        return deletedAccounts;
    }

    @Override
    public void revert() {
        logger.debug("reverted");
        super.reset();
    }

    @Override
    public void commit() {
        // TODO: lots to consider here... but for the moment we pretend we did a thing
        for (Account account : getTouchedAccounts()) {
            logger.debug("updated account: {}", account);
        }
    }
}
