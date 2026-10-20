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

import org.lfdt.paladin.sdk.core.types.Bytes32;
import org.lfdt.paladin.sdk.core.types.HexBytes;
import org.lfdt.paladin.sdk.core.types.HexUint256;

/**
 * Inputs to {@link NotoInstance#prepareBurnUnlock}: prepares a burn that spends a lock.
 *
 * @param lockId the id of the lock
 * @param from lookup string of the identity whose tokens are spent
 * @param amount the number of tokens
 * @param unlockData opaque data recorded when the lock is later spent; defaults to empty
 * @param data opaque data recorded with the transaction; defaults to empty
 */
public record NotoPrepareBurnUnlockParams(
    Bytes32 lockId, String from, HexUint256 amount, HexBytes unlockData, HexBytes data) {

  /**
   * Creates the parameters; defaults unset byte fields to empty.
   *
   * @param lockId the id of the lock
   * @param from lookup string of the identity whose tokens are spent
   * @param amount the number of tokens
   * @param unlockData opaque data recorded when the lock is later spent; defaults to empty
   * @param data opaque data recorded with the transaction; defaults to empty
   */
  public NotoPrepareBurnUnlockParams {
    unlockData = NotoDefaults.orEmpty(unlockData);
    data = NotoDefaults.orEmpty(data);
  }
}
