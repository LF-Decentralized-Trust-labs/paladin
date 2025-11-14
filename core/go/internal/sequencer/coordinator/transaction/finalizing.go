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

	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
)

func guard_HasGracePeriodPassedSinceStateChange(ctx context.Context, txn *Transaction) bool {
	// Has this transaction been in the same state for longer than the finalizing grace period?
	// most useful to know this once we have reached one of the terminal states - Reverted or Committed
	return txn.heartbeatIntervalsSinceStateChange >= txn.finalizingGracePeriod
}

func action_Cleanup(ctx context.Context, txn *Transaction) error {
	log.L(ctx).Infof("action_Cleanup - cleaning up transaction %s", txn.ID.String())
	return txn.cleanup(ctx)
}
