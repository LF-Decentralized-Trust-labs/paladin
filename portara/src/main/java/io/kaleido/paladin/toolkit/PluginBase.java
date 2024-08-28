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

package io.kaleido.paladin.toolkit;

import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;

import java.util.HashMap;
import java.util.Map;

public abstract class PluginBase<MSG extends CommonMessage> {

    private static final Logger LOGGER = LogManager.getLogger(PluginBase.class);

    protected abstract PluginInstance<MSG> newPluginInstance(String grpcTarget, String instanceUUID);

    private final Map<String, PluginInstance<MSG>> instances = new HashMap<>();

    public synchronized void startInstance(String grpcTarget, String instanceUUID) {
        instances.put(instanceUUID, newPluginInstance(grpcTarget, instanceUUID));
    }

    public synchronized void stopInstance(String instanceUUID) {
        PluginInstance<MSG> instance = instances.remove(instanceUUID);
        if (instance != null) {
            instance.shutdown();
        }
    }

}
