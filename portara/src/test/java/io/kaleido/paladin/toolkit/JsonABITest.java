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

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.assertFalse;

import java.util.regex.Pattern;

public class JsonABITest {

    @Test
    public void testSimpleStorageABI() throws Exception {

        String resourcePath = "contracts/testcontracts/SimpleStorageWrapped.sol/SimpleStorageWrapped.json";
        JsonABI simpleABI = JsonABI.fromJSONResourceEntry(getClass().getClassLoader(), resourcePath, "abi");

        // Check default ABI formatting is non-pretty
        String defaultJson = simpleABI.toString();
        assertTrue(defaultJson.matches("\\[.*\"name\":\"set\".*]"), defaultJson);

        // Check default ABI pretty formatting
        String prettyPrinted = simpleABI.toJSON(true);
        assertFalse(prettyPrinted.matches("\\[.*\"name\":\"set\".*]"), defaultJson);
        assertTrue(Pattern.compile("\\[.*\"type\" : \"function\".*]", Pattern.DOTALL).matcher(prettyPrinted).matches(), prettyPrinted);

        // Check default entry formatting is non-pretty
        JsonABI.Entry setFunction = simpleABI.getABIEntry("function", "set");
        defaultJson = setFunction.toString();
        assertTrue(defaultJson.matches("\\{.*\"name\":\"set\".*}"), defaultJson);

        // Check pretty entry formatting
        prettyPrinted = simpleABI.getABIEntry("function", "get").toJSON(true);
        assertFalse(prettyPrinted.matches("\\{.*\"type\":\"function\".*}"), prettyPrinted);
        assertTrue(Pattern.compile("\\{.*\"type\" : \"function\".*}", Pattern.DOTALL).matcher(prettyPrinted).matches(), prettyPrinted);

        // Check default param formatting is non-pretty
        defaultJson = setFunction.inputs().getFirst().toString();
        assertTrue(defaultJson.matches("\\{.*\"name\":\"_x\".*}"), defaultJson);

        // Check default param pretty formatting
        prettyPrinted = setFunction.inputs().getFirst().toJSON(true);
        assertFalse(prettyPrinted.matches("\\{.*\"name\":\"_x\".*}"), prettyPrinted);
        assertTrue(Pattern.compile("\\{.*\"name\" : \"_x\".*}", Pattern.DOTALL).matcher(prettyPrinted).matches(), prettyPrinted);

    }

    @Test
    public void testContractFactory() throws Exception {
        JsonABI.Entry constructor = JsonABI.newConstructor(
                JsonABI.newParameters(
                        JsonABI.newParameter("param1", "uint256")
                )
        );
        assertEquals("""
            {"type":"constuctor","inputs":[{"name":"param1","type":"uint256"}]}
        """.trim(), constructor.toString());
    }

    @Test
    public void testFunctionFactory() throws Exception {
        JsonABI.Entry fn = JsonABI.newFunction(
                "doStuff",
                JsonABI.newParameters(
                        JsonABI.newTupleArray("param1", JsonABI.newParameters(
                                JsonABI.newParameter("field1", "uint256"),
                                JsonABI.newParameter("field2", "string")
                        ))
                ),
                JsonABI.newParameters(
                        JsonABI.newParameter("out1", "uint256"),
                        JsonABI.newParameter("out2", "string")
                )
        );
        assertEquals("""
                {"type":"function","name":"doStuff","inputs":[{"name":"param1","type":"tuple[]","components":[{"name":"field1","type":"uint256"},{"name":"field2","type":"string"}]}],"outputs":[{"name":"out1","type":"uint256"},{"name":"out2","type":"string"}]}
            """.trim(), fn.toString());
    }

    @Test
    public void testEventactory() throws Exception {
        JsonABI.Entry fn = JsonABI.newEvent(
                "StuffHappened",
                JsonABI.newParameters(
                        JsonABI.newIndexedParameter("field1", "uint256"),
                        JsonABI.newParameter("field2", "string")
                )
        );
        assertEquals("""
                {"type":"event","name":"StuffHappened","inputs":[{"name":"field1","type":"uint256","indexed":true},{"name":"field2","type":"string"}]}
            """.trim(), fn.toString());
    }
}
