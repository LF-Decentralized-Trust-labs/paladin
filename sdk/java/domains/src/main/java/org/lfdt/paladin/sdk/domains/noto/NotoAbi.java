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
import org.lfdt.paladin.sdk.core.abi.AbiEntry;
import org.lfdt.paladin.sdk.core.abi.AbiParameter;
import org.lfdt.paladin.sdk.core.abi.StateMutability;

/**
 * The ABIs used to transact with Noto: the constructor understood by the Noto factory, the private
 * interface implemented by the domain, and the public functions of the base-ledger contract.
 */
public final class NotoAbi {

  private static final AbiParameter RECIPIENTS =
      AbiParameter.builder("recipients", "tuple[]")
          .internalType("struct INotoPrivate.UnlockRecipient[]")
          .component(param("to", "string"))
          .component(param("amount", "uint256"))
          .build();

  /** The constructor accepted by the Noto factory for a {@link NotaryMode#BASIC} token. */
  public static final AbiEntry CONSTRUCTOR_BASIC =
      constructor(
          AbiParameter.builder("basic", "tuple")
              .component(param("restrictMint", "bool"))
              .component(param("allowBurn", "bool"))
              .component(param("allowLock", "bool"))
              .build());

  /** The constructor accepted by the Noto factory for a {@link NotaryMode#HOOKS} token. */
  public static final AbiEntry CONSTRUCTOR_HOOKS =
      constructor(
          AbiParameter.builder("hooks", "tuple")
              .component(
                  AbiParameter.builder("privateGroup", "tuple")
                      .component(param("salt", "bytes32"))
                      .component(param("members", "string[]"))
                      .build())
              .component(param("publicAddress", "address"))
              .component(param("privateAddress", "address"))
              .build());

  /** The private Noto interface, invoked through the domain. */
  public static final List<AbiEntry> PRIVATE =
      List.of(
          function("mint", param("to", "string"), param("amount", "uint256"), bytes("data")),
          function("burn", param("amount", "uint256"), bytes("data")),
          function("burnFrom", param("from", "string"), param("amount", "uint256"), bytes("data")),
          function("transfer", param("to", "string"), param("amount", "uint256"), bytes("data")),
          function(
              "transferFrom",
              param("from", "string"),
              param("to", "string"),
              param("amount", "uint256"),
              bytes("data")),
          function("lock", param("amount", "uint256"), bytes("data")),
          function("createLock", param("amount", "uint256"), bytes("data")),
          function(
              "unlock",
              param("lockId", "bytes32"),
              param("from", "string"),
              RECIPIENTS,
              bytes("data")),
          function(
              "createTransferLock",
              param("from", "string"),
              RECIPIENTS,
              bytes("unlockData"),
              bytes("data")),
          function("createMintLock", RECIPIENTS, bytes("unlockData"), bytes("data")),
          function(
              "createBurnLock",
              param("from", "string"),
              param("amount", "uint256"),
              bytes("unlockData"),
              bytes("data")),
          function(
              "prepareUnlock",
              param("lockId", "bytes32"),
              param("from", "string"),
              RECIPIENTS,
              bytes("unlockData"),
              bytes("data")),
          function(
              "prepareMintUnlock",
              param("lockId", "bytes32"),
              RECIPIENTS,
              bytes("unlockData"),
              bytes("data")),
          function(
              "prepareBurnUnlock",
              param("lockId", "bytes32"),
              param("from", "string"),
              param("amount", "uint256"),
              bytes("unlockData"),
              bytes("data")),
          function(
              "delegateLock",
              param("lockId", "bytes32"),
              param("delegate", "address"),
              bytes("data")),
          view("name", List.of(), List.of(param("", "string"))),
          view("symbol", List.of(), List.of(param("", "string"))),
          view("decimals", List.of(), List.of(param("", "uint8"))),
          view(
              "balanceOf",
              List.of(param("account", "string")),
              List.of(
                  param("totalStates", "uint256"),
                  param("totalBalance", "uint256"),
                  param("overflow", "bool"))));

  /** The public functions of the Noto contract on the base ledger. */
  public static final List<AbiEntry> PUBLIC =
      List.of(
          function("spendLock", param("lockId", "bytes32"), bytes("spendArgs"), bytes("data")),
          function("cancelLock", param("lockId", "bytes32"), bytes("cancelArgs"), bytes("data")));

  private NotoAbi() {}

  /**
   * The constructor accepted by the Noto factory for the given notary mode.
   *
   * @param notaryMode the notary mode of the token being deployed
   * @return {@link #CONSTRUCTOR_HOOKS} for hooks mode, otherwise {@link #CONSTRUCTOR_BASIC}
   */
  public static AbiEntry constructor(final NotaryMode notaryMode) {
    return notaryMode == NotaryMode.HOOKS ? CONSTRUCTOR_HOOKS : CONSTRUCTOR_BASIC;
  }

  private static AbiEntry constructor(final AbiParameter options) {
    return AbiEntry.constructor()
        .input(param("name", "string"))
        .input(param("symbol", "string"))
        .input(param("notary", "string"))
        .input(param("notaryMode", "string"))
        .input(AbiParameter.builder("options", "tuple").component(options).build())
        .build();
  }

  private static AbiEntry function(final String name, final AbiParameter... inputs) {
    return AbiEntry.function(name)
        .stateMutability(StateMutability.NONPAYABLE)
        .inputs(List.of(inputs))
        .build();
  }

  private static AbiEntry view(
      final String name, final List<AbiParameter> inputs, final List<AbiParameter> outputs) {
    return AbiEntry.function(name)
        .stateMutability(StateMutability.VIEW)
        .inputs(inputs)
        .outputs(outputs)
        .build();
  }

  private static AbiParameter param(final String name, final String type) {
    return AbiParameter.of(name, type);
  }

  private static AbiParameter bytes(final String name) {
    return AbiParameter.of(name, "bytes");
  }
}
