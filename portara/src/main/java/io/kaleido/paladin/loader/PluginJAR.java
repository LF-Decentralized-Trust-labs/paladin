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

import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;

import java.io.File;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.net.MalformedURLException;
import java.net.URI;
import java.net.URL;
import java.net.URLClassLoader;

public class PluginJAR extends Plugin {

    private static final Logger LOGGER = LogManager.getLogger(PluginJAR.class);

    private final String libName;
    private final String className;
    private Object pluginImpl;
    private Method stopInstanceMethod;

    PluginJAR(String grpcTarget, PluginInfo info, PluginStopped onStop, String libName, String className) throws MalformedURLException, NoSuchMethodException, ClassNotFoundException, InvocationTargetException, InstantiationException, IllegalAccessException {
        super(grpcTarget, info, onStop);
        this.libName = libName;
        this.className = className;
    }


    @Override
    public synchronized void stop() throws Exception {
        stopInstanceMethod.invoke(pluginImpl, info.instanceId());
    }

    @Override
    public synchronized void loadAndStart() throws Exception {
        ClassLoader classLoader = this.getClass().getClassLoader();
        if (libName != null && !libName.isBlank()) {
            URI fileURI = new File(libName).toURI();
            classLoader = new URLClassLoader(new URL[]{fileURI.toURL()}, classLoader);
        }
        Class<?> clazz = Class.forName(className);
        pluginImpl = clazz.getDeclaredConstructor().newInstance();
        Method startInstanceMethod = clazz.getDeclaredMethod("startInstance", String.class, String.class);
        stopInstanceMethod = clazz.getDeclaredMethod("stopInstance", String.class);
        startInstanceMethod.invoke(pluginImpl, grpcTarget, info.instanceId());
    }
}
