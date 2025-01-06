/*
 * Copyright © 2024 Kaleido, Inc.
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

package io.kaleido.paladin.toolkit;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.io.Closeable;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicLong;

public class JsonRpcClient implements Closeable {

    private static final Logger LOGGER = PaladinLogging.getLogger(JsonRpcClient.class);

    private final String uriString;

    private final HttpClient httpClient;

    public JsonRpcClient(String uriString) {
        this(uriString, HttpClient.newBuilder().build());
    }

    public JsonRpcClient(String uriString, HttpClient httpClient) {
        this.uriString = uriString;
        this.httpClient = httpClient;
    }

    private static final AtomicLong nextRequest = new AtomicLong(10000000);

    @Override
    public void close() throws IOException {
        httpClient.close();
    }

    private record JSONRPCRequest(
            @JsonProperty
            String jsonrpc,
            @JsonProperty
            long id,
            @JsonProperty
            String method,
            @JsonProperty
            List<Object> params
    ) {}

    private record JSONRPCError(
        @JsonProperty
        long code,
        @JsonProperty
        String message,
        @JsonProperty
        Map<String, Object> data
    ) {}

    private record JSONRPCResponse<ResultType>(
            @JsonProperty
            String jsonrpc,
            @JsonProperty
            long id,
            @JsonProperty
            ResultType result,
            @JsonProperty
            JSONRPCError error
    ) {}

    private final Duration requestTimeout = Duration.ofSeconds(30);

    public <ResultType> ResultType request(String method, Object ...params) throws IOException {
        long requestId = nextRequest.getAndIncrement();
        try {
            // Build JSON request body
            ObjectMapper objectMapper = new ObjectMapper();
            String requestBody = objectMapper.writeValueAsString(
                    new JSONRPCRequest(
                            "2.0",
                            requestId,
                            method,
                            Arrays.asList(params)
                    ));

            // Build HTTP request
            HttpRequest req = HttpRequest.newBuilder()
                    .timeout(requestTimeout)
                    .uri(new URI(uriString))
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(requestBody))
                    .build();
            LOGGER.debug("--> RPC[{}] {}", requestId, method);
            LOGGER.trace("--> RPC[{}] {}: {}", requestId, method, requestBody);
            HttpResponse<String> res = httpClient.send(req, HttpResponse.BodyHandlers.ofString());

            // Parse the response body
            JSONRPCResponse<ResultType> rpcRes = objectMapper.readValue(res.body(), new TypeReference<>() {});
            if (res.statusCode() < 200 || res.statusCode() >= 300 ||
                    (rpcRes.error() != null && rpcRes.error().code < 0)) {
                String message = "";
                if (rpcRes.error() != null) {
                    message = rpcRes.error().message();
                } else {
                    message = res.body();
                }
                throw new IOException(message);
            }
            LOGGER.trace("<-- RPC[{}] {}: {}", requestId, method, res.body());
            LOGGER.debug("<-- RPC[{}] {} [{}]", requestId, method, res.statusCode());
            return rpcRes.result();
        } catch(Throwable e) {
            LOGGER.error(new FormattedMessage("<-- RPC[{}] {} ERROR", requestId, method), e);
            if (e instanceof IOException) {
                throw (IOException)(e);
            }
            if (e instanceof RuntimeException) {
                throw (RuntimeException)(e);
            }
            throw new IOException(e);
        }
    }

}
