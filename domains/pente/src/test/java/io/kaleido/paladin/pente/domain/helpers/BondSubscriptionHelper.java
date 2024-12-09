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

import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.ResourceLoader;

import java.io.IOException;
import java.util.HashMap;

public class BondSubscriptionHelper {
    final PenteHelper pente;
    final JsonABI abi;
    final JsonHex.Address address;

    public static BondSubscriptionHelper deploy(PenteHelper pente, String sender, Object inputs) throws IOException {
        String bytecode = ResourceLoader.jsonResourceEntryText(
                BondSubscriptionHelper.class.getClassLoader(),
                "contracts/private/BondSubscription.sol/BondSubscription.json",
                "bytecode"
        );
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                BondTrackerHelper.class.getClassLoader(),
                "contracts/private/BondSubscription.sol/BondSubscription.json",
                "abi"
        );
        var constructor = abi.getABIEntry("constructor", null);
        var address = pente.deploy(sender, bytecode, constructor.inputs(), inputs);
        return new BondSubscriptionHelper(pente, abi, address);
    }

    private BondSubscriptionHelper(PenteHelper pente, JsonABI abi, JsonHex.Address address) {
        this.pente = pente;
        this.abi = abi;
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public void prepareBond(String sender, JsonHex.Address to, JsonHex.Bytes encodedCall) throws IOException {
        var method = abi.getABIEntry("function", "prepareBond");
        pente.invoke(
                method.name(),
                method.inputs(),
                sender,
                address,
                new HashMap<>() {{
                    put("to", to);
                    put("encodedCall", encodedCall);
                }}
        );
    }

    public void preparePayment(String sender, JsonHex.Address to, JsonHex.Bytes encodedCall) throws IOException {
        var method = abi.getABIEntry("function", "preparePayment");
        pente.invoke(
                method.name(),
                method.inputs(),
                sender,
                address,
                new HashMap<>() {{
                    put("to", to);
                    put("encodedCall", encodedCall);
                }}
        );
    }

    public Testbed.TransactionResult distribute(String sender) throws IOException {
        var method = abi.getABIEntry("function", "distribute");
        return pente.invoke(
                method.name(),
                method.inputs(),
                sender,
                address,
                new HashMap<>());
    }
}
