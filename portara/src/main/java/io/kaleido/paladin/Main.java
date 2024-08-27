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

import io.kaleido.paladin.configlight.RuntimeInfo;
import io.kaleido.paladin.configlight.YamlConfig;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;

public class Main {

    private static final Logger LOGGER = LogManager.getLogger(Main.class);

    static int run(String[] args) {

        if (args.length < 2) {
            throw new Error("usage: <config.paladin.yaml> <node|testbed>");
        }
        try {
            final String configFile = args[0];
            final String engineName = args[1];

            // We have a very limited amount of parsing of the config file that happens in the loader.
            // We just need enough to know whether to use a special temp dir for our socket file,
            // and to initialize the Java logging framework.
            RuntimeInfo runtimeInfo = new YamlConfig(configFile).getRuntimeInfo();

            return KataJNA.Load().Run(
                    runtimeInfo.socketFilename(),
                    runtimeInfo.instanceUUID(),
                    configFile,
                    engineName
            );
        } catch(Throwable e) {
            throw new RuntimeException(e.getMessage(), e);
        }
    }

    public static void main(String[] args) {
        int rc;
        try {
            rc = run(args);
        } catch(Exception e) {
            LOGGER.error("loader error: {}", e.getMessage());
            rc = 1;
        }
        if (rc != 0) {
            LOGGER.error("exiting with error: {}", rc);
        }
        System.exit(rc);
    }
}
