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
package org.lfdt.paladin.sdk.client.websocket;

import java.net.URI;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/** Connection and retry settings for the WebSocket subscription client. */
public final class WebSocketClientConfig {

  private final URI uri;
  private final Duration connectTimeout;
  private final Duration requestTimeout;
  private final Duration heartbeatInterval;
  private final Duration reconnectDelay;
  private final Duration maxReconnectDelay;
  private final Map<String, String> headers;

  private WebSocketClientConfig(final Builder builder) {
    uri = builder.uri;
    connectTimeout = builder.connectTimeout;
    requestTimeout = builder.requestTimeout;
    heartbeatInterval = builder.heartbeatInterval;
    reconnectDelay = builder.reconnectDelay;
    maxReconnectDelay = builder.maxReconnectDelay;
    headers = Map.copyOf(builder.headers);
  }

  /**
   * Creates a configuration builder.
   *
   * @param url a {@code ws://} or {@code wss://} JSON-RPC endpoint
   * @return the builder
   */
  public static Builder builder(final String url) {
    return new Builder(url);
  }

  /**
   * Returns the WebSocket endpoint.
   *
   * @return the endpoint URI
   */
  public URI uri() {
    return uri;
  }

  /**
   * Returns the connection timeout.
   *
   * @return the timeout
   */
  public Duration connectTimeout() {
    return connectTimeout;
  }

  /**
   * Returns the response timeout for subscribe and unsubscribe requests.
   *
   * @return the timeout
   */
  public Duration requestTimeout() {
    return requestTimeout;
  }

  /**
   * Returns the ping interval.
   *
   * @return the interval
   */
  public Duration heartbeatInterval() {
    return heartbeatInterval;
  }

  /**
   * Returns the initial reconnect delay, or {@code null} when reconnection is disabled.
   *
   * @return the delay, or {@code null}
   */
  public Duration reconnectDelay() {
    return reconnectDelay;
  }

  /**
   * Returns the upper bound for exponential reconnect delays.
   *
   * @return the maximum delay
   */
  public Duration maxReconnectDelay() {
    return maxReconnectDelay;
  }

  /**
   * Returns handshake headers.
   *
   * @return the unmodifiable headers
   */
  public Map<String, String> headers() {
    return headers;
  }

  /** Builder for {@link WebSocketClientConfig}. */
  public static final class Builder {

    private final URI uri;
    private Duration connectTimeout = Duration.ofSeconds(30);
    private Duration requestTimeout = Duration.ofSeconds(30);
    private Duration heartbeatInterval = Duration.ofSeconds(30);
    private Duration reconnectDelay = Duration.ofSeconds(2);
    private Duration maxReconnectDelay = Duration.ofSeconds(30);
    private final Map<String, String> headers = new LinkedHashMap<>();

    private Builder(final String url) {
      uri = URI.create(Objects.requireNonNull(url, "url"));
      if (!"ws".equalsIgnoreCase(uri.getScheme()) && !"wss".equalsIgnoreCase(uri.getScheme())) {
        throw new IllegalArgumentException("WebSocket URL must use ws or wss");
      }
      if (uri.getHost() == null) {
        throw new IllegalArgumentException("WebSocket URL must contain a host");
      }
    }

    /**
     * Sets the connection timeout.
     *
     * @param value a positive duration
     * @return this builder
     */
    public Builder connectTimeout(final Duration value) {
      connectTimeout = positive(value, "connectTimeout");
      return this;
    }

    /**
     * Sets the response timeout for subscription lifecycle requests.
     *
     * @param value a positive duration
     * @return this builder
     */
    public Builder requestTimeout(final Duration value) {
      requestTimeout = positive(value, "requestTimeout");
      return this;
    }

    /**
     * Sets the ping interval.
     *
     * @param value a positive duration
     * @return this builder
     */
    public Builder heartbeatInterval(final Duration value) {
      heartbeatInterval = positive(value, "heartbeatInterval");
      return this;
    }

    /**
     * Sets the initial reconnect delay. Zero disables reconnection. It must not exceed the maximum
     * reconnect delay, which is checked by {@link #build()}.
     *
     * @param value a nonnegative duration
     * @return this builder
     */
    public Builder reconnectDelay(final Duration value) {
      Objects.requireNonNull(value, "reconnectDelay");
      if (value.isNegative()) {
        throw new IllegalArgumentException("reconnectDelay must not be negative");
      }
      reconnectDelay = value.isZero() ? null : value;
      return this;
    }

    /**
     * Sets the maximum reconnect delay.
     *
     * @param value a positive duration
     * @return this builder
     */
    public Builder maxReconnectDelay(final Duration value) {
      maxReconnectDelay = positive(value, "maxReconnectDelay");
      return this;
    }

    /**
     * Adds a handshake header, for example an authorization header.
     *
     * @param name the header name
     * @param value the header value
     * @return this builder
     */
    public Builder header(final String name, final String value) {
      headers.put(Objects.requireNonNull(name, "name"), Objects.requireNonNull(value, "value"));
      return this;
    }

    /**
     * Builds the immutable configuration.
     *
     * @return the configuration
     * @throws IllegalArgumentException if the reconnect delay exceeds the maximum reconnect delay
     */
    public WebSocketClientConfig build() {
      if (reconnectDelay != null && reconnectDelay.compareTo(maxReconnectDelay) > 0) {
        throw new IllegalArgumentException(
            "reconnectDelay must not exceed maxReconnectDelay (" + maxReconnectDelay + ")");
      }
      return new WebSocketClientConfig(this);
    }

    private static Duration positive(final Duration value, final String name) {
      Objects.requireNonNull(value, name);
      if (value.isZero() || value.isNegative()) {
        throw new IllegalArgumentException(name + " must be positive");
      }
      return value;
    }
  }
}
