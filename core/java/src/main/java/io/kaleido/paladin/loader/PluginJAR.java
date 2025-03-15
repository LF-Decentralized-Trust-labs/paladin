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
package io.kaleido.paladin.loader;

import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;

import java.io.File;
import java.lang.reflect.Method;
import java.net.URI;
import java.net.URL;
import java.net.URLClassLoader;

public class PluginJAR extends Plugin {

    private static final Logger LOGGER = PaladinLogging.getLogger(PluginJAR.class);

    private final String libName;
    private final String className;
    private Object pluginImpl;
    private Method stopInstanceMethod;

    PluginJAR(String grpcTarget, PluginInfo info, PluginStopped onStop, String libName, String className) {
        super(grpcTarget, info, onStop);
        this.libName = libName;
        this.className = className;
    }

    @Override
    public synchronized void loadAndStart() throws Exception {
        LOGGER.info("loading JAR plugin {} libName={}", className, libName);
        ClassLoader classLoader = this.getClass().getClassLoader();
        if (libName != null && !libName.isBlank()) {
            URI fileURI = new File(libName).toURI();
            classLoader = new URLClassLoader(new URL[]{fileURI.toURL()}, classLoader);
        }
        Class<?> clazz = classLoader.loadClass(className);
        pluginImpl = clazz.getDeclaredConstructor().newInstance();
        Method startInstanceMethod = clazz.getMethod("startInstance", String.class, String.class);
        stopInstanceMethod = clazz.getMethod("stopInstance", String.class);
        LOGGER.info("loaded JAR plugin {} pluginId={}", pluginImpl.getClass().getName(), info.instanceId());
        startInstanceMethod.invoke(pluginImpl, grpcTarget, info.instanceId());
        LOGGER.info("started JAR plugin {} pluginId={}", pluginImpl.getClass().getName(), info.instanceId());
    }

    @Override
    public synchronized void stop() throws Exception {
        if (stopInstanceMethod != null) {
            stopInstanceMethod.invoke(pluginImpl, info.instanceId());
        }
        onStop.pluginStopped(info.instanceId(), this, null);
    }

}
