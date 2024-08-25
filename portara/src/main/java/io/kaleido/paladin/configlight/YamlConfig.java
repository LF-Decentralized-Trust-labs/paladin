/*
 *  © Copyright Kaleido, Inc. 2024. The materials in this file constitute the "Pre-Existing IP,"
 *  "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 *  under a limited, perpetual license only, subject to the terms of the applicable license
 *  agreement between you and Kaleido, Inc.  All other rights reserved.
 */

package io.kaleido.paladin.configlight;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;

import java.io.File;
import java.io.IOException;
import java.text.MessageFormat;
import java.util.UUID;

import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.core.config.Configurator;

public class YamlConfig {

    private static final Logger LOGGER = LogManager.getLogger(YamlConfig.class);

    private final YamlRootConfig loadedConfig;

    public YamlConfig(String configFile) throws IOException {
        final ObjectMapper mapper = new ObjectMapper(new YAMLFactory());
        this.loadedConfig = mapper.readValue(new File(configFile), YamlRootConfig.class);
        setupLogging();
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
        // Socket files have to be quite small
        File socketFile = new File(tempDir, String.format("p.%s.sock", uStr.substring(uStr.length()-6)));
        LOGGER.info("instance={} uds={}", uStr, socketFile.getAbsolutePath());
        return new RuntimeInfo(uuid.toString(), socketFile.getAbsolutePath());
    }

    void setupLogging() {
        YamlLogConfig logConfig = loadedConfig.logConfig();
        if (logConfig == null) {
            logConfig = new YamlLogConfig("info");
        }

        Level level;
        switch (logConfig.level()) {
            case "error":
                level = Level.ERROR;
                break;
            case "warn":
            case "warning":
                level = Level.WARN;
                break;
            case "debug":
                level = Level.DEBUG;
                break;
            case "trace":
                level = Level.TRACE;
                break;
            case "info":
            default:
                level = Level.INFO;
                break;
        }
        Configurator.setAllLevels("io.kaleido.paladin", level);
    }

}
