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

import com.sun.jna.Library;
import com.sun.jna.Native;
import github.com.kaleido_io.paladin.toolkit.Service;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;

import java.util.UUID;

public class PluginJNA extends Plugin {

    private static final Logger LOGGER = LogManager.getLogger(PluginJNA.class);

    private PluginJNA paladinGo;

    private final PluginCShared lib;

    interface PluginCShared extends Library {
        int Run(String grpcTarget, String pluginUUID);
        void Stop();
    }

    PluginJNA(String grpcTarget, PluginInfo info, String libName) throws UnsatisfiedLinkError {
        super(grpcTarget, info);
        LOGGER.info("Loading plugin via JNA: {}", libName);
        lib = Native.load(libName, PluginCShared.class);
    }


    @Override
    public void stop() {
        lib.Stop();
    }

    @Override
    public void run() {
        int rc = lib.Run(this.grpcTarget, info.instanceUUID().toString());
        if (rc != 0) {
            throw new RuntimeException("Plugin returned RC=%d".formatted(rc));
        }
    }
}
