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

// TODO should we pass static config ( e.g. timeouts) to the guards instead of storing them in every transaction struct instance?
type Guard func(ctx context.Context, txn *Transaction) bool

// endorsed by all required endorsers
func guard_AttestationPlanFulfilled(ctx context.Context, txn *Transaction) bool {
	return !txn.hasUnfulfilledEndorsementRequirements(ctx)
}

func guard_NoDependenciesNotReady(ctx context.Context, txn *Transaction) bool {
	return !guard_HasDependenciesNotReady(ctx, txn)
}

func guard_HasDependenciesNotReady(ctx context.Context, txn *Transaction) bool {
	return txn.hasDependenciesNotReady(ctx)
}

func guard_AssembleTimeoutExceeded(ctx context.Context, txn *Transaction) bool {
	return txn.assembleTimeoutExceeded(ctx)
}

func guard_NoUnassembledDependencies(ctx context.Context, txn *Transaction) bool {
	return !txn.hasDependenciesNotAssembled(ctx)
}

func guard_HasUnassembledDependencies(ctx context.Context, txn *Transaction) bool {
	return txn.hasDependenciesNotAssembled(ctx)
}

func guard_HasUnknownDependencies(ctx context.Context, txn *Transaction) bool {
	return txn.hasUnknownDependencies(ctx)
}

// TODO add a guard_Not function to negate a guard
func guard_NoUnknownDependencies(ctx context.Context, txn *Transaction) bool {
	return !txn.hasUnknownDependencies(ctx)
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

func guard_Or(guards ...Guard) Guard {
	return func(ctx context.Context, txn *Transaction) bool {
		for _, guard := range guards {
			if guard(ctx, txn) {
				return true
			}
		}
		return false
	}
}
