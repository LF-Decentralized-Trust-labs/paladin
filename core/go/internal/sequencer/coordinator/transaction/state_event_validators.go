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

type EventValidator func(ctx context.Context, txn *Transaction, event Event) (bool, error)

func validator_MatchesPendingAssembleRequest(ctx context.Context, txn *Transaction, event Event) (bool, error) {
	//TODO is there some generics magic that would make this more elegant
	switch event := event.(type) {
	case *AssembleSuccessEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	case *AssembleRevertResponseEvent:
		return txn.pendingAssembleRequest != nil && txn.pendingAssembleRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}

func validator_MatchesPendingDispatchConfirmationRequest(ctx context.Context, txn *Transaction, event Event) (bool, error) {
	switch event := event.(type) {
	case *DispatchConfirmedEvent:
		return txn.pendingDispatchConfirmationRequest != nil && txn.pendingDispatchConfirmationRequest.IdempotencyKey() == event.RequestID, nil
	}
	return false, nil
}
