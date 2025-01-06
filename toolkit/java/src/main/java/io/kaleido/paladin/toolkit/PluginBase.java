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

import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;

import java.util.HashMap;
import java.util.Map;

abstract class PluginBase<MSG> {

    private static final Logger LOGGER = PaladinLogging.getLogger(PluginBase.class);

    abstract PluginInstance<MSG> newPluginInstance(String grpcTarget, String instanceId);

    private final Map<String, PluginInstance<MSG>> instances = new HashMap<>();

    public synchronized void startInstance(String grpcTarget, String instanceId) {
        LOGGER.info("starting plugin instance {}", instanceId);
        instances.put(instanceId, newPluginInstance(grpcTarget, instanceId));
    }

    public synchronized void stopInstance(String instanceId) {
        PluginInstance<MSG> instance = instances.remove(instanceId);
        if (instance != null) {
            instance.shutdown();
        }
    }

}
