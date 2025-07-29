/*
 * Copyright © 2025 Kaleido, Inc.
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
package transaction

import (
	"context"
)

func action_ResendAssembleSuccessResponse(ctx context.Context, txn *Transaction) error {
	return action_SendAssembleSuccessResponse(ctx, txn)
}

func action_ResendAssembleRevertResponse(ctx context.Context, txn *Transaction) error {
	return action_SendAssembleRevertResponse(ctx, txn)
}

func action_ResendAssembleParkResponse(ctx context.Context, txn *Transaction) error {
	return action_SendAssembleParkResponse(ctx, txn)
}

// True if the most recent assemble request has the same idempotency key as the most recent fulfilled assemble request
func guard_AssembleRequestMatchesPreviousResponse(ctx context.Context, txn *Transaction) bool {
	return txn.latestAssembleRequest.requestID == txn.latestFulfilledAssembleRequestID
}
