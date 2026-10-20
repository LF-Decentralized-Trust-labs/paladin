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

import java.time.Duration;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.PaladinClient;
import org.lfdt.paladin.sdk.client.exception.PaladinException;
import org.lfdt.paladin.sdk.client.tx.SentTransaction;
import org.lfdt.paladin.sdk.core.transaction.TransactionReceipt;

/**
 * An in-flight Noto deployment, returned by {@link NotoFactory#newNoto}. Wait for the token with
 * {@link #waitForDeploy()}, or use {@link #transaction()} to inspect the deployment transaction.
 */
public final class NotoDeployment {

  private final PaladinClient paladin;
  private final String domain;
  private final SentTransaction transaction;

  NotoDeployment(
      final PaladinClient paladin, final String domain, final SentTransaction transaction) {
    this.paladin = paladin;
    this.domain = domain;
    this.transaction = transaction;
  }

  /**
   * The submitted deployment transaction.
   *
   * @return the transaction handle
   */
  public SentTransaction transaction() {
    return transaction;
  }

  /**
   * The node-assigned id of the deployment transaction.
   *
   * @return a future completing with the transaction id
   */
  public CompletableFuture<UUID> id() {
    return transaction.id();
  }

  /**
   * Waits for the deployment to complete, using the default receipt timeout.
   *
   * @return a future completing with the deployed token; failing with a {@link PaladinException} if
   *     the deployment failed, or with the errors described on {@link
   *     SentTransaction#waitForReceipt()}
   */
  public CompletableFuture<NotoInstance> waitForDeploy() {
    return transaction.waitForReceipt().thenApply(this::toInstance);
  }

  /**
   * Waits for the deployment to complete, with an explicit timeout.
   *
   * @param timeout the total time to wait; must be positive
   * @return a future completing with the deployed token; failing with a {@link PaladinException} if
   *     the deployment failed, or with the errors described on {@link
   *     SentTransaction#waitForReceipt(Duration)}
   */
  public CompletableFuture<NotoInstance> waitForDeploy(final Duration timeout) {
    return transaction.waitForReceipt(timeout).thenApply(this::toInstance);
  }

  private NotoInstance toInstance(final TransactionReceipt receipt) {
    if (!receipt.success() || receipt.contractAddress() == null) {
      throw new PaladinException(
          "Noto deployment " + receipt.id() + " failed: " + receipt.failureMessage());
    }
    return new NotoInstance(paladin, domain, receipt.contractAddress());
  }
}
