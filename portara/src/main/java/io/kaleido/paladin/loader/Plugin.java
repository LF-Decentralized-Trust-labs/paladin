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

abstract class Plugin  {

   protected final String grpcTarget;
   protected final PluginInfo info;

   interface PluginStopped {
      void pluginStopped(String instanceId, Plugin plugin, Throwable t);
   }

   final PluginStopped onStop;

   Plugin(String grpcTarget, PluginInfo info, PluginStopped onStop) {
      this.grpcTarget = grpcTarget;
      this.info = info;
      this.onStop = onStop;
   }

   abstract void loadAndStart() throws Exception;

   abstract void stop() throws Exception;

   public PluginInfo getInfo() {
      return info;
   }

}
