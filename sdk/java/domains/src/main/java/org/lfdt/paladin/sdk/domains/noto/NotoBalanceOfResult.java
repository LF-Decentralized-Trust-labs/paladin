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

import org.lfdt.paladin.sdk.core.types.HexUint256;

/**
 * The result of {@link NotoInstance#balanceOf}.
 *
 * @param totalBalance the sum of the account's unspent tokens
 * @param totalStates the number of unspent states that make up the balance
 * @param overflow whether the account holds more states than were counted, so the balance is a
 *     lower bound
 */
public record NotoBalanceOfResult(
    HexUint256 totalBalance, HexUint256 totalStates, boolean overflow) {}
