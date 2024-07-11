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

// Create a class that includes a method receiving a callback function
public abstract class TransactionRequest {
    private TransactionHandler transactionHandler;
    private TransactionResponseHandler responseHandler;

    public TransactionRequest(TransactionHandler transactionHandler, TransactionResponseHandler responseHandler) {
        this.transactionHandler = transactionHandler;
        this.responseHandler = responseHandler;
    }

    // Method that receives a callback function
    public void send(TransactionResponseHandler responseHandler) throws Exception {
        this.transactionHandler.submitTransaction(this);
    }
    
    public abstract Transaction.TransactionRequest getRequestMessage();

    public TransactionResponseHandler getResponseHandler() {
        return responseHandler;
    }
}