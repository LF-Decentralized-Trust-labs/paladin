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

package publictxmgr

import (
	"context"
	"testing"
)

func TestMetrics(t *testing.T) {
	// none of the functions are actually implemented, so it's purely for test coverage
	btem := &publicTxEngineMetrics{}
	ctx := context.Background()
	btem.InitMetrics(ctx)
	btem.RecordCompletedTransactionCountMetrics(ctx, "success")
	btem.RecordOperationMetrics(ctx, "test", "success", 12)
	btem.RecordStageChangeMetrics(ctx, "test", 12)
	btem.RecordInFlightOrchestratorPoolMetrics(ctx, nil, 1)
	btem.RecordInFlightTxQueueMetrics(ctx, nil, 1)
	btem.RecordCompletedTransactionCountMetrics(ctx, "test")
}
