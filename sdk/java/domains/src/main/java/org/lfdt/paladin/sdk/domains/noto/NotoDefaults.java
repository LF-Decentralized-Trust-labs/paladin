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

import org.lfdt.paladin.sdk.core.types.HexBytes;

/** Defaults applied to optional Noto inputs. */
final class NotoDefaults {

  /** Empty bytes, sent for optional {@code bytes} inputs the caller leaves unset. */
  static final HexBytes EMPTY_BYTES = HexBytes.wrap(new byte[0]);

  private NotoDefaults() {}

  static HexBytes orEmpty(final HexBytes bytes) {
    return bytes == null ? EMPTY_BYTES : bytes;
  }
}
