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

import io.grpc.LoadBalancerRegistry;
import io.grpc.internal.PickFirstLoadBalancerProvider;
import io.kaleido.paladin.configlight.RuntimeInfo;
import io.kaleido.paladin.configlight.YamlConfig;
import io.kaleido.paladin.loader.PluginLoader;
import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;

import java.util.concurrent.CompletableFuture;

public class Main {

    private static final Logger LOGGER = PaladinLogging.getLogger(Main.class);

    static CoreJNA.PaladinGo paladinGo;

    static RuntimeInfo instance;

    public static synchronized CoreJNA.PaladinGo ensureLoaded() {
        if (paladinGo == null) {
            paladinGo = CoreJNA.Load();
        }
        return paladinGo;
    }

    private static synchronized RuntimeInfo setRunning(RuntimeInfo newInstance) {
        if (instance != null && newInstance != null) {
            throw new IllegalStateException("already running %s".formatted(newInstance.instanceId()));
        }
        instance = newInstance;
        Main.class.notifyAll();
        return instance;
    }

    public static synchronized void stop() {
        LOGGER.info("Stopping paladin");
        final RuntimeInfo runningInstance = instance;
        if (runningInstance != null) {
            CompletableFuture.runAsync(() -> ensureLoaded().Stop());
            while (instance != null) {
                try {
                    Main.class.wait();
                } catch(InterruptedException e) {
                    throw new RuntimeException(e);
                }
            }
        }
    }

    static {
        // see https://github.com/LF-Decentralized-Trust-labs/paladin/issues/239
        LoadBalancerRegistry.getDefaultRegistry().register(new PickFirstLoadBalancerProvider());
    }

    public static int run(String[] args) {
        PluginLoader loader = null;

        if (args.length < 2) {
            throw new Error("usage: <config.paladin.yaml> <engine|testbed>");
        }
        try {
            final String configFile = args[0];
            final String engineName = args[1];

            // We have a very limited amount of parsing of the config file that happens in the loader.
            // We just need enough to know whether to use a special temp dir for our socket file,
            // and to initialize the Java logging framework.
            RuntimeInfo runtimeInfo = setRunning(new YamlConfig(configFile).getRuntimeInfo());

            loader = new PluginLoader(runtimeInfo.socketFilename(), runtimeInfo.instanceId());

            var rc = ensureLoaded().Run(
                    runtimeInfo.socketFilename(),
                    runtimeInfo.instanceId().toString(),
                    configFile,
                    engineName
            );
            LOGGER.error("Paladin shutting down with rc={}", rc);
            return rc;
        } catch(Throwable e) {
            LOGGER.error("Paladin shutting down due to error", e);
            throw new RuntimeException(e.getMessage(), e);
        } finally {
            if (loader != null) {
                loader.shutdown();
            }
            setRunning(null);
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
