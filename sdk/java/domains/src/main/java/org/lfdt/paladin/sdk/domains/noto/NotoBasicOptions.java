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

/**
 * Rules applied by a notary in {@link NotaryMode#BASIC} mode. A {@code null} flag is left for the
 * domain to default, which enables it.
 *
 * @param restrictMint whether only the notary may mint
 * @param allowBurn whether token holders may burn their tokens
 * @param allowLock whether token holders may lock their tokens
 */
public record NotoBasicOptions(Boolean restrictMint, Boolean allowBurn, Boolean allowLock) {

  /**
   * The default rules: minting restricted to the notary, burning and locking allowed.
   *
   * @return options with every flag enabled
   */
  public static NotoBasicOptions defaults() {
    return new NotoBasicOptions(true, true, true);
  }
}
