/*
 *  © Copyright Kaleido, Inc. 2024. The materials in this file constitute the "Pre-Existing IP,"
 *  "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 *  under a limited, perpetual license only, subject to the terms of the applicable license
 *  agreement between you and Kaleido, Inc.  All other rights reserved.
 */

package io.kaleido.paladin.logging;

import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.core.LoggerContext;
import org.apache.logging.log4j.core.appender.ConsoleAppender;
import org.apache.logging.log4j.core.config.Configuration;
import org.apache.logging.log4j.core.config.Configurator;
import org.apache.logging.log4j.core.config.LoggerConfig;
import org.apache.logging.log4j.core.config.builder.api.ConfigurationBuilder;
import org.apache.logging.log4j.core.config.builder.api.ConfigurationBuilderFactory;
import org.apache.logging.log4j.core.config.builder.impl.BuiltConfiguration;

public class PaladinLogging {

    private static final LoggerContext paladinContext;

    private static final Logger LOGGER;

    static {
        // TODO: Support more than just stdout logging for the Java part
        ConfigurationBuilder<BuiltConfiguration> logConfigBuilder = ConfigurationBuilderFactory
                .newConfigurationBuilder();
        var l4jConfig = logConfigBuilder
                .add(logConfigBuilder
                        .newAppender("Stdout", "CONSOLE")
                        .addAttribute("target", ConsoleAppender.Target.SYSTEM_OUT)
                        .add(logConfigBuilder
                                .newLayout("PatternLayout")
                                .addAttribute("pattern", "[%d{ISO8601}{GMT+0}] %-5p JAVA: %m (thread=%t,class=%marker)%n")
                        )
                )
                .add(logConfigBuilder
                        .newRootLogger(Level.ALL)
                        .add(logConfigBuilder.newAppenderRef("Stdout"))
                )
                .build(false);
        LoggerConfig loggerConfig = l4jConfig.getLoggerConfig(LogManager.ROOT_LOGGER_NAME);
        loggerConfig.setLevel(Level.INFO);
        Configurator.reconfigure(l4jConfig);
        paladinContext = Configurator.initialize(PaladinLogging.class.getClassLoader(), l4jConfig);
        LOGGER = paladinContext.getLogger(PaladinLogging.class);
    }

    public static Logger getLogger(Class<?> clazz) {
        return paladinContext.getLogger(clazz);
    }

    public static Logger getLogger(String name) {
        return paladinContext.getLogger(name);
    }

    public static void setLevel(Level level) {
        Configuration config = paladinContext.getConfiguration();
        LoggerConfig loggerConfig = config.getLoggerConfig(LogManager.ROOT_LOGGER_NAME);
        loggerConfig.setLevel(level);
        paladinContext.updateLoggers();
        LOGGER.info("Paladin Java log level set to {}", level);
    }
}
