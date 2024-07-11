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

package io.kaleido.transaction;

import paladin.transaction.Transaction;

public class SubmitTransactionRequest extends TransactionRequest {

    private final String contractAddress;
    private final String from;
    private final String idempotencyKey;
    private final String payloadJSON;

    public SubmitTransactionRequest(
            TransactionHandler transactionHandler,
            TransactionResponseHandler responseHandler,
            String contractAddress,
            String from,
            String idempotencyKey,
            String payloadJSON) {
        super(transactionHandler, responseHandler);
        this.contractAddress = contractAddress;
        this.from = from;
        this.idempotencyKey = idempotencyKey;
        this.payloadJSON = payloadJSON;
    }

    @Override
    public Transaction.TransactionRequest getRequestMessage() {
        return Transaction.TransactionRequest.newBuilder()
                .setType(Transaction.REQUEST_TYPE.SUBMIT_TRANSACTION_REQUEST)
                .setSubmitTransactionRequest(Transaction.SubmitTransactionRequest.newBuilder()
                        .setContractAddress(this.contractAddress)
                        .setFrom(this.from)
                        .setIdempotencyKey(this.idempotencyKey)
                        .setPayloadJSON(this.payloadJSON))
                .build();
    }

}
