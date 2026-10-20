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

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import org.lfdt.paladin.sdk.client.rpc.RpcClient;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;

/** In-memory {@link RpcClient} that records each call and answers from canned JSON results. */
final class FakeRpcClient implements RpcClient {

  /** One recorded call: the method and its positional parameters as JSON. */
  record Call(String method, JsonNode params) {

    /** The first positional parameter, which carries the request body for most methods. */
    JsonNode body() {
      return params.get(0);
    }
  }

  private static final ObjectMapper MAPPER = PaladinObjectMapper.shared();

  private final Map<String, String> results = new HashMap<>();
  private final List<Call> calls = new ArrayList<>();

  /** Answers every later call to {@code method} with {@code resultJson}. */
  FakeRpcClient respond(final String method, final String resultJson) {
    results.put(method, resultJson);
    return this;
  }

  List<Call> calls() {
    return calls;
  }

  /** The most recent call to {@code method}; fails the test if there was none. */
  Call lastCall(final String method) {
    for (int i = calls.size() - 1; i >= 0; i--) {
      if (calls.get(i).method().equals(method)) {
        return calls.get(i);
      }
    }
    throw new AssertionError("no call to " + method + " in " + calls);
  }

  @Override
  public <T> CompletableFuture<T> callRpc(
      final Class<T> resultType, final String method, final Object... params) {
    return answer(method, params).thenApply(node -> MAPPER.convertValue(node, resultType));
  }

  @Override
  public <T> CompletableFuture<T> callRpc(
      final TypeReference<T> resultType, final String method, final Object... params) {
    return answer(method, params).thenApply(node -> MAPPER.convertValue(node, resultType));
  }

  private CompletableFuture<JsonNode> answer(final String method, final Object... params) {
    calls.add(new Call(method, MAPPER.valueToTree(params)));
    final String result = results.get(method);
    if (result == null) {
      return CompletableFuture.failedFuture(new IllegalStateException("unexpected call " + method));
    }
    try {
      return CompletableFuture.completedFuture(MAPPER.readTree(result));
    } catch (final JsonProcessingException e) {
      return CompletableFuture.failedFuture(e);
    }
  }

  @Override
  public void close() {}
}
