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

import io.kaleido.paladin.logging.PaladinLogging;
import org.apache.logging.log4j.Logger;
import org.apache.logging.log4j.message.FormattedMessage;

import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.CancellationException;
import java.util.concurrent.CompletableFuture;

public class InFlight<K, V> {
    private static final Logger LOGGER = PaladinLogging.getLogger(InFlight.class);

    private final Map<K, CompletableFuture<V>> requests = new HashMap<>();

    public class Request extends CompletableFuture<V> {
        private final K id;

        private Request(K id) {
            this.id = id;
        }

        @Override
        public boolean cancel(boolean mayInterruptIfRunning) {
            InFlight.this.cancelRequest(id);
            return super.cancel(mayInterruptIfRunning);
        }
    }

    public synchronized CompletableFuture<V> addRequest(K id) {
        if (requests.containsKey(id)) {
            throw new IllegalArgumentException("duplicate request");
        }
        CompletableFuture<V> req = new CompletableFuture<>();
        requests.put(id, req);
        LOGGER.debug("started request {}", id);
        return req;
     }

     public synchronized void completeRequest(K id, V value) {
        CompletableFuture<V> req = requests.remove(id);
        if (req == null) {
            LOGGER.warn("notified after cancel for request {}", id);
        } else {
            LOGGER.debug("completed request {}", id);
            req.complete(value);
        }
     }

    public synchronized void failRequest(K id, Throwable e) {
        CompletableFuture<V> req = requests.remove(id);
        LOGGER.error(new FormattedMessage("failed request {}", id), e);
        if (req != null) {
            req.completeExceptionally(e);
        }
    }

    private synchronized void cancelRequest(K id) {
        CompletableFuture<V> req = requests.remove(id);
        if (req != null) {
            LOGGER.debug("cancelled request {}", id);
            req.completeExceptionally(new CancellationException());
        }
    }

}
