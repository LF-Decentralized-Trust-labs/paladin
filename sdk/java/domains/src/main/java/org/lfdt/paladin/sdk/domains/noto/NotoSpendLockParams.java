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

/**
 * Inputs to {@link NotoInstance#spendLock}: spends a lock on the base ledger.
 *
 * @param lockId the id of the lock
 * @param spendArgs the encoded arguments authorizing the spend
 * @param data opaque data recorded with the transaction; defaults to empty
 */
public record NotoSpendLockParams(Bytes32 lockId, HexBytes spendArgs, HexBytes data) {

  /**
   * Creates the parameters; defaults unset byte fields to empty.
   *
   * @param lockId the id of the lock
   * @param spendArgs the encoded arguments authorizing the spend
   * @param data opaque data recorded with the transaction; defaults to empty
   */
  public NotoSpendLockParams {
    spendArgs = NotoDefaults.orEmpty(spendArgs);
    data = NotoDefaults.orEmpty(data);
  }
}
