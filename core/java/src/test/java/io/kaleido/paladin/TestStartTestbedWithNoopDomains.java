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

package io.kaleido.paladin;

import io.kaleido.paladin.toolkit.Testbed;
import org.junit.jupiter.api.Test;

import java.util.HashMap;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class TestStartTestbedWithNoopDomains {

    @Test
    void runTestbed() throws Exception {
        System.out.println(System.getProperty("os.name"));
        System.out.println(System.getProperty("os.arch"));
        System.out.println(System.getProperty("java.library.path"));

        Testbed testBed = new Testbed(
                new Testbed.Setup("../go/db/migrations/sqlite", 5000),
                new Testbed.ConfigDomain(
                        "domain1",
                        new Testbed.ConfigPlugin("c-shared", "starter", ""),
                        new HashMap<>()),
                new Testbed.ConfigDomain(
                        "domain2",
                        new Testbed.ConfigPlugin("jar", "", TestDomainFactory.class.getName()),
                        new HashMap<>())
        );
        testBed.close();
    }
}
