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
package org.lfdt.paladin.sdk.client.privacygroup;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.domain.DomainInvokeRPC;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroup;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMCall;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupEVMTXInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessage;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageInput;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageListener;
import org.lfdt.paladin.sdk.core.query.QueryJSON;
import org.lfdt.paladin.sdk.core.statestore.StateStatusQualifier;
import org.lfdt.paladin.sdk.core.types.EthAddress;
import org.lfdt.paladin.sdk.core.types.HexBytes;

/**
 * Client for the {@code pgroup_*} RPC namespace (privacy groups).
 *
 * <p>Each method maps one-to-one to a JSON-RPC call on the underlying {@link RpcClient} and returns
 * a {@link CompletableFuture}; failures complete it exceptionally with a {@code PaladinException}
 * subtype.
 *
 * <p>The namespace covers three things: the groups themselves, the EVM transactions and calls
 * executed inside them, and the off-chain messages members exchange. Message delivery is driven by
 * named listeners; use {@link org.lfdt.paladin.sdk.client.websocket.WebSocketSubscriptionClient} to
 * consume their streams.
 */
public final class PrivacyGroupClient {

  private final RpcClient rpc;

  /**
   * Creates a client over the given RPC transport.
   *
   * @param rpc the RPC client used to make calls; must not be {@code null}
   */
  public PrivacyGroupClient(final RpcClient rpc) {
    this.rpc = Objects.requireNonNull(rpc, "rpc");
  }

  /**
   * Creates a privacy group ({@code pgroup_createGroup}).
   *
   * @param spec the specification for the new group
   * @return a future completing with the created group; its contract address is filled in once the
   *     genesis transaction is mined
   */
  public CompletableFuture<PrivacyGroup> createGroup(final PrivacyGroupInput spec) {
    return rpc.callRpc(PrivacyGroup.class, "pgroup_createGroup", spec);
  }

  /**
   * Looks up a privacy group by its identifier ({@code pgroup_getGroupById}).
   *
   * @param domainName the domain the group belongs to
   * @param id the group identifier
   * @return a future completing with the group, or {@code null} if no such group exists
   */
  public CompletableFuture<PrivacyGroup> getGroupById(final String domainName, final HexBytes id) {
    return rpc.callRpc(PrivacyGroup.class, "pgroup_getGroupById", domainName, id);
  }

  /**
   * Looks up a privacy group by its on-chain contract address ({@code pgroup_getGroupByAddress}).
   *
   * @param address the contract address of the group
   * @return a future completing with the group, or {@code null} if no such group exists
   */
  public CompletableFuture<PrivacyGroup> getGroupByAddress(final EthAddress address) {
    return rpc.callRpc(PrivacyGroup.class, "pgroup_getGroupByAddress", address);
  }

  /**
   * Queries privacy groups ({@code pgroup_queryGroups}).
   *
   * @param query the query to run
   * @return a future completing with the matching groups
   */
  public CompletableFuture<List<PrivacyGroup>> queryGroups(final QueryJSON query) {
    return rpc.callRpc(new TypeReference<List<PrivacyGroup>>() {}, "pgroup_queryGroups", query);
  }

  /**
   * Queries privacy groups that a given identity is a member of ({@code
   * pgroup_queryGroupsWithMember}).
   *
   * @param member the identity locator of the member to filter on
   * @param query the query to run over the groups that member belongs to
   * @return a future completing with the matching groups
   */
  public CompletableFuture<List<PrivacyGroup>> queryGroupsWithMember(
      final String member, final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<PrivacyGroup>>() {}, "pgroup_queryGroupsWithMember", member, query);
  }

  /**
   * Sends a transaction to be executed inside a privacy group ({@code pgroup_sendTransaction}).
   *
   * @param tx the transaction to execute in the group
   * @return a future completing with the id of the submitted transaction
   */
  public CompletableFuture<UUID> sendTransaction(final PrivacyGroupEVMTXInput tx) {
    return rpc.callRpc(UUID.class, "pgroup_sendTransaction", tx);
  }

  /**
   * Executes a read-only call inside a privacy group ({@code pgroup_call}).
   *
   * @param call the call to execute in the group
   * @return a future completing with the raw result data, in the requested data format
   */
  public CompletableFuture<JsonNode> call(final PrivacyGroupEVMCall call) {
    return rpc.callRpc(JsonNode.class, "pgroup_call", call);
  }

  /**
   * Invokes a domain-specific RPC method in a privacy-group context ({@code pgroup_invokeRPC}).
   *
   * @param domainName the domain the group belongs to
   * @param groupId the identifier of the group to evaluate the request against
   * @param stateQualifier the state context in which to evaluate the request
   * @param rpcCall the domain-specific RPC request
   * @return a future completing with the domain RPC result
   */
  public CompletableFuture<JsonNode> invokeRPC(
      final String domainName,
      final HexBytes groupId,
      final StateStatusQualifier stateQualifier,
      final DomainInvokeRPC rpcCall) {
    return rpc.callRpc(
        JsonNode.class, "pgroup_invokeRPC", domainName, groupId, stateQualifier, rpcCall);
  }

  /**
   * Sends a message to the members of a privacy group ({@code pgroup_sendMessage}).
   *
   * @param message the message to send
   * @return a future completing with the id of the sent message
   */
  public CompletableFuture<UUID> sendMessage(final PrivacyGroupMessageInput message) {
    return rpc.callRpc(UUID.class, "pgroup_sendMessage", message);
  }

  /**
   * Looks up a privacy group message by id ({@code pgroup_getMessageById}).
   *
   * @param id the message id
   * @return a future completing with the message, or {@code null} if no such message exists
   */
  public CompletableFuture<PrivacyGroupMessage> getMessageById(final UUID id) {
    return rpc.callRpc(PrivacyGroupMessage.class, "pgroup_getMessageById", id);
  }

  /**
   * Queries privacy group messages ({@code pgroup_queryMessages}).
   *
   * @param query the query to run
   * @return a future completing with the matching messages
   */
  public CompletableFuture<List<PrivacyGroupMessage>> queryMessages(final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<PrivacyGroupMessage>>() {}, "pgroup_queryMessages", query);
  }

  /**
   * Creates a message listener ({@code pgroup_createMessageListener}).
   *
   * @param listener the listener to create
   * @return a future completing with {@code true} when the listener was created
   */
  public CompletableFuture<Boolean> createMessageListener(
      final PrivacyGroupMessageListener listener) {
    return rpc.callRpc(Boolean.class, "pgroup_createMessageListener", listener);
  }

  /**
   * Queries message listeners ({@code pgroup_queryMessageListeners}).
   *
   * @param query the query to run
   * @return a future completing with the matching listeners
   */
  public CompletableFuture<List<PrivacyGroupMessageListener>> queryMessageListeners(
      final QueryJSON query) {
    return rpc.callRpc(
        new TypeReference<List<PrivacyGroupMessageListener>>() {},
        "pgroup_queryMessageListeners",
        query);
  }

  /**
   * Looks up a message listener by name ({@code pgroup_getMessageListener}).
   *
   * @param listenerName the name of the listener
   * @return a future completing with the listener, or {@code null} if no such listener exists
   */
  public CompletableFuture<PrivacyGroupMessageListener> getMessageListener(
      final String listenerName) {
    return rpc.callRpc(
        PrivacyGroupMessageListener.class, "pgroup_getMessageListener", listenerName);
  }

  /**
   * Starts a message listener ({@code pgroup_startMessageListener}).
   *
   * @param listenerName the name of the listener to start
   * @return a future completing with {@code true} when the listener was started
   */
  public CompletableFuture<Boolean> startMessageListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "pgroup_startMessageListener", listenerName);
  }

  /**
   * Stops a message listener ({@code pgroup_stopMessageListener}).
   *
   * @param listenerName the name of the listener to stop
   * @return a future completing with {@code true} when the listener was stopped
   */
  public CompletableFuture<Boolean> stopMessageListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "pgroup_stopMessageListener", listenerName);
  }

  /**
   * Deletes a message listener ({@code pgroup_deleteMessageListener}).
   *
   * @param listenerName the name of the listener to delete
   * @return a future completing with {@code true} when the listener was deleted
   */
  public CompletableFuture<Boolean> deleteMessageListener(final String listenerName) {
    return rpc.callRpc(Boolean.class, "pgroup_deleteMessageListener", listenerName);
  }
}
