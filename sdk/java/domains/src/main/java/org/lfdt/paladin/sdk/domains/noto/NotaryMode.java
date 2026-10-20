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

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/** How the notary of a Noto token validates transactions. */
public enum NotaryMode {
  /** The notary applies the built-in rules configured by {@link NotoBasicOptions}. */
  BASIC("basic"),
  /** The notary delegates validation to a hooks contract configured by {@link NotoHooksOptions}. */
  HOOKS("hooks");

  private final String jsonValue;

  NotaryMode(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this notary mode.
   *
   * @return the lower-case JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a notary mode from its JSON token, case-insensitively.
   *
   * @param modeString the JSON token to resolve
   * @return the matching notary mode
   * @throws IllegalArgumentException if {@code modeString} is null or not a known notary mode
   */
  @JsonCreator
  public static NotaryMode fromJson(final String modeString) {
    for (NotaryMode mode : values()) {
      if (mode.jsonValue.equalsIgnoreCase(modeString)) {
        return mode;
      }
    }
    throw new IllegalArgumentException("unknown notary mode: \"" + modeString + "\"");
  }
}
