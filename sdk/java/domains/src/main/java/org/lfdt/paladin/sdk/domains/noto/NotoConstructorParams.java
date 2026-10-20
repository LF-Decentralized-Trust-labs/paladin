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

import java.util.Objects;

/**
 * Inputs to {@link NotoFactory#newNoto}: the configuration of a new Noto token. Start from {@link
 * #basic(String)}, {@link #basic(String, NotoBasicOptions)} or {@link #hooks(String,
 * NotoHooksOptions)}, then name the token with {@link #withName(String)} and {@link
 * #withSymbol(String)}.
 *
 * @param name the token name; defaults to empty
 * @param symbol the token symbol; defaults to empty
 * @param notary lookup string of the notary identity
 * @param notaryMode how the notary validates transactions
 * @param options the notary configuration matching {@code notaryMode}
 */
public record NotoConstructorParams(
    String name, String symbol, String notary, NotaryMode notaryMode, NotoOptions options) {

  /**
   * Creates the parameters, defaulting an unset name and symbol to empty.
   *
   * @param name the token name; defaults to empty
   * @param symbol the token symbol; defaults to empty
   * @param notary lookup string of the notary identity; must not be {@code null}
   * @param notaryMode how the notary validates transactions; must not be {@code null}
   * @param options the notary configuration matching {@code notaryMode}; must not be {@code null}
   */
  public NotoConstructorParams {
    name = name == null ? "" : name;
    symbol = symbol == null ? "" : symbol;
    Objects.requireNonNull(notary, "notary");
    Objects.requireNonNull(notaryMode, "notaryMode");
    Objects.requireNonNull(options, "options");
  }

  /**
   * A token whose notary applies the default basic rules.
   *
   * @param notary lookup string of the notary identity
   * @return the parameters, with {@link NotoBasicOptions#defaults()}
   */
  public static NotoConstructorParams basic(final String notary) {
    return basic(notary, NotoBasicOptions.defaults());
  }

  /**
   * A token whose notary applies the given basic rules.
   *
   * @param notary lookup string of the notary identity
   * @param options the rules the notary applies
   * @return the parameters
   */
  public static NotoConstructorParams basic(final String notary, final NotoBasicOptions options) {
    return new NotoConstructorParams(
        null,
        null,
        notary,
        NotaryMode.BASIC,
        new NotoOptions(Objects.requireNonNull(options, "options"), null));
  }

  /**
   * A token whose notary delegates validation to a hooks contract.
   *
   * @param notary lookup string of the notary identity
   * @param hooks the hooks contract
   * @return the parameters
   */
  public static NotoConstructorParams hooks(final String notary, final NotoHooksOptions hooks) {
    return new NotoConstructorParams(
        null,
        null,
        notary,
        NotaryMode.HOOKS,
        new NotoOptions(null, Objects.requireNonNull(hooks, "hooks")));
  }

  /**
   * Copies these parameters with a new token name.
   *
   * @param name the token name
   * @return the updated parameters
   */
  public NotoConstructorParams withName(final String name) {
    return new NotoConstructorParams(name, symbol, notary, notaryMode, options);
  }

  /**
   * Copies these parameters with a new token symbol.
   *
   * @param symbol the token symbol
   * @return the updated parameters
   */
  public NotoConstructorParams withSymbol(final String symbol) {
    return new NotoConstructorParams(name, symbol, notary, notaryMode, options);
  }
}
