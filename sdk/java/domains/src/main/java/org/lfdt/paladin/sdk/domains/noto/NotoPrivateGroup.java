/*
 * Copyright contributors to Paladin, an LFDT project
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
package org.lfdt.paladin.sdk.domains.noto;

import java.util.List;
import org.lfdt.paladin.sdk.core.types.Bytes32;

/**
 * The privacy group hosting a Noto hooks contract.
 *
 * @param salt the salt that, with the members, identifies the group
 * @param members lookup strings of the group members
 */
public record NotoPrivateGroup(Bytes32 salt, List<String> members) {

  /**
   * Creates the group details, copying the member list.
   *
   * @param salt the salt that, with the members, identifies the group
   * @param members lookup strings of the group members
   */
  public NotoPrivateGroup {
    members = List.copyOf(members);
  }
}
