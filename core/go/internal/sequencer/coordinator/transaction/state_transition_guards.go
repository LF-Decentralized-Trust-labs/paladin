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

type Guard func(ctx context.Context, txn *Transaction) bool

// endorsed by all required endorsers
func guard_AttestationPlanFulfilled(ctx context.Context, txn *Transaction) bool {
	return !txn.hasOutstandingEndorsementRequests(ctx)
}

func guard_NoDependenciesNotReady(ctx context.Context, txn *Transaction) bool {
	return !guard_HasDependenciesNotReady(ctx, txn)
}

func guard_HasDependenciesNotReady(ctx context.Context, txn *Transaction) bool {
	return txn.hasDependenciesNotReady(ctx)
}

func guard_And(guards ...Guard) Guard {
	return func(ctx context.Context, txn *Transaction) bool {
		for _, guard := range guards {
			if !guard(ctx, txn) {
				return false
			}
		}
		return true
	}
}
