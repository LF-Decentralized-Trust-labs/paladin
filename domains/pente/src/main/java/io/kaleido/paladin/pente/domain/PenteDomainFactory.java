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

 package io.kaleido.paladin.pente.domain;

 import io.kaleido.paladin.logging.PaladinLogging;
 import io.kaleido.paladin.toolkit.DomainBase;
 import io.kaleido.paladin.toolkit.DomainInstance;
 import org.apache.logging.log4j.Logger;
 
 public class PenteDomainFactory extends DomainBase {
     private static final Logger LOGGER = PaladinLogging.getLogger(PenteDomainFactory.class);
 
     @Override
     protected DomainInstance newDomainInstance(String grpcTarget, String instanceId) {
         LOGGER.info("starting paladin domain id={}", instanceId);
         return new PenteDomain(grpcTarget, instanceId);
     }
 }
 