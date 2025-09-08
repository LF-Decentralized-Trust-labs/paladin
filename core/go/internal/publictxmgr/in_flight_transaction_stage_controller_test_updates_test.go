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
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTXStageControllerUpdate(t *testing.T) {
	ctx, o, _, done := newTestOrchestrator(t)
	defer done()
	it, _ := newInflightTransaction(o, 1)
	it.testOnlyNoActionMode = true

	it.UpdateTransaction(ctx, &DBPublicTxn{
		Gas: 1000,
		FixedGasPricing: pldtypes.JSONString(pldapi.PublicTxGasPricing{
			GasPrice: pldtypes.Uint64ToUint256(10),
		}),
	})

	it.ProduceLatestInFlightStageContext(ctx, &OrchestratorContext{})

	require.Len(t, it.stateManager.GetGenerations(ctx), 2)

	assert.Nil(t, it.stateManager.GetGeneration(ctx, 0).GetRunningStageContext(ctx))

	rsc := it.stateManager.GetCurrentGeneration(ctx).GetRunningStageContext(ctx)
	require.NotNil(t, rsc)
	assert.Equal(t, InFlightTxStageSigning, rsc.Stage)
}
