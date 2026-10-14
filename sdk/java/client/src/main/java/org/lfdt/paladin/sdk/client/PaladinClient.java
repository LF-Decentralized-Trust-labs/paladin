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
package org.lfdt.paladin.sdk.client;

import java.util.List;
import java.util.Objects;
import org.lfdt.paladin.sdk.client.blockindex.BlockIndexClient;
import org.lfdt.paladin.sdk.client.config.RpcClientConfig;
import org.lfdt.paladin.sdk.client.keymgr.KeyManagerClient;
import org.lfdt.paladin.sdk.client.privacygroup.PrivacyGroupClient;
import org.lfdt.paladin.sdk.client.ptx.PtxClient;
import org.lfdt.paladin.sdk.client.registry.RegistryClient;
import org.lfdt.paladin.sdk.client.rpc.HttpRpcClient;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.client.statestore.StateStoreClient;
import org.lfdt.paladin.sdk.client.transport.TransportClient;
import org.lfdt.paladin.sdk.client.tx.TxBuilder;
import org.lfdt.paladin.sdk.core.abi.AbiEntry;

/**
 * The entry point to a Paladin node, exposing every RPC namespace client over a shared transport.
 *
 * <p>Point it at a node and reach any namespace from the same handle:
 *
 * <pre>{@code
 * try (PaladinClient paladin = PaladinClient.http("http://localhost:8548")) {
 *   TransactionReceipt receipt =
 *       paladin
 *           .newTx()
 *           .publicTx()
 *           .from("alice")
 *           .to("0x0102030405060708090a0b0c0d0e0f1011121314")
 *           .function("ping()")
 *           .send()
 *           .waitForReceipt()
 *           .join();
 *
 *   List<WalletInfo> wallets = paladin.keyManager().wallets().join();
 *   IndexedBlock latest = paladin.blockIndex().getBlockByNumber(receipt.blockNumber()).join();
 * }
 * }</pre>
 *
 * <p>The namespace clients are created once and returned unchanged on every call, so holding on to
 * one (e.g. {@code PtxClient ptx = paladin.ptx()}) is equivalent to going through this object each
 * time. They all share this client's single {@link RpcClient}, and each is a thin, stateless
 * wrapper over it — there is no per-namespace connection.
 *
 * <p><b>Transport ownership.</b> A client built with {@link #http(String)} or {@link
 * #http(RpcClientConfig)} owns the transport it created and releases it on {@link #close()} — use
 * it in try-with-resources. A client built with {@link #wrap(RpcClient)} borrows a transport the
 * caller created, so {@link #close()} leaves it open; close that transport yourself when done.
 *
 * <p>This facade uses the HTTP transport. Use {@link
 * org.lfdt.paladin.sdk.client.websocket.WebSocketSubscriptionClient} for subscription streams.
 *
 * <p>Instances created with {@code http(...)} are thread-safe and intended to be shared for the
 * lifetime of the application. A wrapped client is thread-safe if its supplied {@link RpcClient} is
 * thread-safe. Transaction builders are mutable and should be used from a single thread.
 */
public final class PaladinClient implements AutoCloseable {

  private final RpcClient rpc;
  private final boolean ownsTransport;

  private final PtxClient ptx;
  private final KeyManagerClient keyManager;
  private final BlockIndexClient blockIndex;
  private final RegistryClient registry;
  private final StateStoreClient stateStore;
  private final TransportClient transport;
  private final PrivacyGroupClient privacyGroups;

  private PaladinClient(final RpcClient rpc, final boolean ownsTransport) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
    this.ownsTransport = ownsTransport;
    this.ptx = new PtxClient(rpc);
    this.keyManager = new KeyManagerClient(rpc);
    this.blockIndex = new BlockIndexClient(rpc);
    this.registry = new RegistryClient(rpc);
    this.stateStore = new StateStoreClient(rpc);
    this.transport = new TransportClient(rpc);
    this.privacyGroups = new PrivacyGroupClient(rpc);
  }

  /**
   * Creates a client against a node URL over HTTP, with the default transport settings (30s connect
   * and request timeouts, the default retry policy).
   *
   * <p>The returned client owns the transport it creates: close it when done.
   *
   * @param url the node's JSON-RPC endpoint URL (e.g. {@code http://localhost:8548})
   * @return a client connected to that node
   */
  public static PaladinClient http(final String url) {
    return http(RpcClientConfig.builder(url).build());
  }

  /**
   * Creates a client over HTTP with explicit transport configuration — timeouts, extra headers
   * (authentication, routing), and the retry policy.
   *
   * <p>The returned client owns the transport it creates: close it when done.
   *
   * @param config the transport configuration
   * @return a client connected to the configured node
   */
  public static PaladinClient http(final RpcClientConfig config) {
    return new PaladinClient(
        new HttpRpcClient(Objects.requireNonNull(config, "config")), /* ownsTransport= */ true);
  }

  /**
   * Wraps a transport the caller already created and owns, for sharing one transport across clients
   * or plugging in an alternative {@link RpcClient} implementation (a test double, say).
   *
   * <p>{@link #close()} on the returned client does <em>not</em> close the given transport.
   *
   * @param rpc the RPC transport to use; must not be {@code null}
   * @return a client over that transport
   */
  public static PaladinClient wrap(final RpcClient rpc) {
    return new PaladinClient(rpc, /* ownsTransport= */ false);
  }

  /**
   * The underlying JSON-RPC transport, for calling methods this SDK does not yet wrap.
   *
   * @return the RPC transport backing every namespace client
   */
  public RpcClient rpc() {
    return rpc;
  }

  /**
   * The {@code ptx_*} namespace: the transaction lifecycle, receipts, states, stored ABIs, and
   * listeners.
   *
   * @return the Paladin transaction client
   */
  public PtxClient ptx() {
    return ptx;
  }

  /**
   * The {@code keymgr_*} namespace: wallets, key resolution, and verifier lookup.
   *
   * @return the key manager client
   */
  public KeyManagerClient keyManager() {
    return keyManager;
  }

  /**
   * The {@code bidx_*} namespace: indexed blocks, transactions, and events from the base ledger.
   *
   * @return the block index client
   */
  public BlockIndexClient blockIndex() {
    return blockIndex;
  }

  /**
   * The {@code reg_*} namespace: registries and their entries and properties.
   *
   * @return the registry client
   */
  public RegistryClient registry() {
    return registry;
  }

  /**
   * The {@code pstate_*} namespace: schemas, states, and nullifiers.
   *
   * @return the state store client
   */
  public StateStoreClient stateStore() {
    return stateStore;
  }

  /**
   * The {@code transport_*} namespace: local node identity, peers, and reliable messages.
   *
   * @return the transport client
   */
  public TransportClient transport() {
    return transport;
  }

  /**
   * The {@code pgroup_*} namespace: privacy groups, their transactions, and their messages.
   *
   * @return the privacy group client
   */
  public PrivacyGroupClient privacyGroups() {
    return privacyGroups;
  }

  /**
   * Starts a fluent transaction builder against this node. Shorthand for {@code ptx().newTx()}.
   *
   * @return a new builder bound to this client's {@code ptx} namespace
   */
  public TxBuilder newTx() {
    return ptx.newTx();
  }

  /**
   * Starts a transaction builder with the ABI already set. Shorthand for {@code newTx().abi(abi)}.
   *
   * @param abi the contract ABI the transaction is built against
   * @return a new builder bound to this client, carrying the given ABI
   */
  public TxBuilder forAbi(final List<AbiEntry> abi) {
    return newTx().abi(abi);
  }

  /**
   * Releases the transport if this client created it ({@link #http(String)} / {@link
   * #http(RpcClientConfig)}); a no-op for a client built with {@link #wrap(RpcClient)}, whose
   * transport belongs to the caller.
   *
   * <p>Calling this more than once is safe.
   */
  @Override
  public void close() {
    if (ownsTransport) {
      rpc.close();
    }
  }
}
