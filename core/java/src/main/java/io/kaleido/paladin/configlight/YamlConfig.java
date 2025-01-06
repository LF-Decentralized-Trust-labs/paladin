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

package io.kaleido.paladin.configlight;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.Logger;

import java.io.File;
import java.io.IOException;
import java.util.UUID;

public class YamlConfig {

    private static final Logger LOGGER = PaladinLogging.getLogger(YamlConfig.class);

    private final YamlRootConfig loadedConfig;

    public YamlConfig(String configFile) throws IOException {
        final ObjectMapper mapper = new ObjectMapper(new YAMLFactory());
        this.loadedConfig = mapper.readValue(new File(configFile), YamlRootConfig.class);
        setupLogging();
        setupLoaderDebug();
        LOGGER.debug("Loaded config {}", configFile);
    }

    public RuntimeInfo getRuntimeInfo() throws IOException {
        String tempDirPath = loadedConfig.tempDir();
        if (tempDirPath == null || tempDirPath.isEmpty()) {
            tempDirPath = System.getProperty("java.io.tmpdir");
        }
        UUID uuid = UUID.randomUUID();
        File tempDir = new File(tempDirPath);
        if (!tempDir.isDirectory()) {
            throw new IOException(String.format("%s is not a directory", tempDir.getAbsolutePath()));
        }
        String uStr = uuid.toString();
        // Allocate a socket file with our pid used to make it unique
        File socketFile = new File(tempDir, String.format("p.%d.sock", ProcessHandle.current().pid()));
        String grpcTarget = "unix:" + socketFile.getAbsolutePath();
        LOGGER.info("instance={} grpcTarget={}", uStr, grpcTarget);
        return new RuntimeInfo(uuid, grpcTarget);
    }

    void setupLogging() {
        YamlLogConfig logConfig = loadedConfig.log();
        if (logConfig == null) {
            logConfig = new YamlLogConfig("info");
        }

        Level level = switch (logConfig.level()) {
            case "error" -> Level.ERROR;
            case "warn", "warning" -> Level.WARN;
            case "debug" -> Level.DEBUG;
            case "trace" -> Level.TRACE;
            default -> Level.INFO;
        };
        PaladinLogging.setLevel(level);

    }

    void setupLoaderDebug() {
        YamlLoaderConfig loaderConfig = loadedConfig.loader();
        if (loaderConfig == null) {
            loaderConfig = new YamlLoaderConfig(false);
        }
        if (loaderConfig.debug()) {
            System.setProperty("jna.debug_load", "true");
        }
    }

}
