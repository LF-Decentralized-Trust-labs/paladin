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

import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.ResourceLoader;

import java.io.IOException;
import java.util.HashMap;

public class InvestorListHelper {
    final PenteHelper pente;
    final JsonABI abi;
    final JsonHex.Address address;

    public static InvestorListHelper deploy(PenteHelper pente, String sender, Object inputs) throws IOException {
        String bytecode = ResourceLoader.jsonResourceEntryText(
                CSDBondTrackerHelper.class.getClassLoader(),
                "contracts/private/InvestorList.sol/InvestorList.json",
                "bytecode"
        );
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                CSDBondTrackerHelper.class.getClassLoader(),
                "contracts/private/InvestorList.sol/InvestorList.json",
                "abi"
        );
        var constructor = abi.getABIEntry("constructor", null);
        var address = pente.deploy(sender, bytecode, constructor.inputs(), inputs);
        return new InvestorListHelper(pente, address);
    }

    InvestorListHelper(PenteHelper pente, JsonHex.Address address) throws IOException {
        this.pente = pente;
        this.abi = JsonABI.fromJSONResourceEntry(
                CSDBondTrackerHelper.class.getClassLoader(),
                "contracts/private/InvestorList.sol/InvestorList.json",
                "abi"
        );
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public void addInvestor(String sender, String addr) throws IOException {
        var method = abi.getABIEntry("function", "addInvestor");
        pente.invoke(
                method.name(),
                method.inputs(),
                sender,
                address,
                new HashMap<>() {{
                    put("addr", addr);
                }}
        );
    }
}
