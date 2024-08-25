/*
 *  © Copyright Kaleido, Inc. 2024. The materials in this file constitute the "Pre-Existing IP,"
 *  "Background IP," "Background Technology" or the like of Kaleido, Inc. and are provided to you
 *  under a limited, perpetual license only, subject to the terms of the applicable license
 *  agreement between you and Kaleido, Inc.  All other rights reserved.
 */

package io.kaleido.paladin.configlight;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.core.config.Configurator;

import java.io.File;
import java.io.IOException;


public record RuntimeInfo(String instanceUUID, String socketFilename) {}
