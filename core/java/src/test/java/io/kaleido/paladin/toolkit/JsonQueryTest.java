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

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.skyscreamer.jsonassert.JSONAssert;
import org.skyscreamer.jsonassert.JSONCompareMode;

import java.util.Arrays;

import static io.kaleido.paladin.toolkit.JsonQuery.Modifier.CASE_INSENSITIVE;
import static io.kaleido.paladin.toolkit.JsonQuery.Modifier.NOT;

public class JsonQueryTest {

    @Test
    public void testQueryFlat() throws Exception {

        var query = JsonQuery.newBuilder().
                limit(1).
                sort("field1").sort("field2").
                isEqual("field1", "value1").
                isNotEqual("field2", "value2").
                isLike("field3", "%some value%").
                isLessThan("field4", 12345).
                isLessThanEqual("field5", 23456).
                isGreaterThan("field6", 34567).
                isGreaterThanEqual("field7", 45678).
                isIn("field8", Arrays.asList("a","b","c")).
                isNotIn("field9", Arrays.asList("x","y","z")).
                isNull("field10", NOT).
                isNull("field11").
                isEqual("field12", "value12", NOT, CASE_INSENSITIVE).
                json();

        JSONAssert.assertEquals("""
            {
              "limit": 1,
              "sort": ["field1","field2"],
              "eq": [
                { "field": "field1", "value": "value1" },
                { "field": "field12", "value": "value12", "not": true, "caseInsensitive": true }
              ],
              "neq": [
                { "field": "field2", "value": "value2" }
              ],
              "like": [
                { "field": "field3", "value": "%some value%" }
              ],
              "lt": [
                { "field": "field4", "value": 12345 }
              ],
              "lte": [
                { "field": "field5", "value": 23456 }
              ],
              "gt": [
                { "field": "field6", "value": 34567 }
              ],
              "gte": [
                { "field": "field7", "value": 45678 }
              ],
              "in": [
                { "field": "field8", "values": ["a","b","c"] }
              ],
              "nin": [
                { "field": "field9", "values": ["x","y","z"] }
              ],
              "null": [
                { "field": "field10", "not": true },
                { "field": "field11" }
              ]
            }
        """, query, JSONCompareMode.STRICT);
    }

    @Test
    public void testQueryOr() throws Exception {

        var query = JsonQuery.newBuilder().
            or(nested -> nested
                    .isEqual("field1", "value1")
                    .isNotEqual("field2", "value2")
            ).
            or(nested -> nested
                    .isNotEqual("field1", "value1")
                    .isEqual("field3", "value3")
            );
        // Note "build()" is not necessary to get JSON out

        JSONAssert.assertEquals("""
            {
              "or": [
                {
                  "eq": [
                    { "field": "field1", "value": "value1" }
                  ],
                  "neq": [
                    { "field": "field2", "value": "value2" }
                  ]
                },
                {
                  "eq": [
                    { "field": "field3", "value": "value3" }
                  ],
                  "neq": [
                    { "field": "field1", "value": "value1" }
                  ]
                }
              ]
            }
        """, new ObjectMapper().writeValueAsString(query), JSONCompareMode.STRICT);
    }

}
