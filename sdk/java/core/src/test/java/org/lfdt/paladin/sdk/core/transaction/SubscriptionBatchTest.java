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
package org.lfdt.paladin.sdk.core.transaction;

import static org.junit.jupiter.api.Assertions.assertEquals;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.lfdt.paladin.sdk.core.json.PaladinObjectMapper;
import org.lfdt.paladin.sdk.core.privacygroup.PrivacyGroupMessageBatch;

class SubscriptionBatchTest {

  private final ObjectMapper mapper = PaladinObjectMapper.shared();

  @Test
  void receiptBatchRoundTrips() throws Exception {
    final TransactionReceiptBatch batch =
        mapper.readValue("{\"batchId\":12,\"receipts\":[]}", TransactionReceiptBatch.class);
    assertEquals(12L, batch.batchId());
    assertEquals(List.of(), batch.receipts());
    assertEquals(
        batch, mapper.readValue(mapper.writeValueAsString(batch), TransactionReceiptBatch.class));
  }

  @Test
  void eventBatchUsesUuidString() throws Exception {
    final TransactionEventBatch batch =
        mapper.readValue("{\"batchId\":\"abc-def\",\"events\":[]}", TransactionEventBatch.class);
    assertEquals("abc-def", batch.batchId());
    assertEquals(List.of(), batch.events());
    assertEquals(
        batch, mapper.readValue(mapper.writeValueAsString(batch), TransactionEventBatch.class));
  }

  @Test
  void messageBatchRoundTrips() throws Exception {
    final PrivacyGroupMessageBatch batch =
        mapper.readValue("{\"batchId\":14,\"messages\":[]}", PrivacyGroupMessageBatch.class);
    assertEquals(14L, batch.batchId());
    assertEquals(List.of(), batch.messages());
    assertEquals(
        batch, mapper.readValue(mapper.writeValueAsString(batch), PrivacyGroupMessageBatch.class));
  }
}
