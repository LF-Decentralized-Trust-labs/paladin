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

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/require"
)

func TestDispatchActionsUnknownActionIgnored(t *testing.T) {
	ctx, txm, _, done := newTestPublicTxManager(t, false)
	defer done()

	err := txm.dispatchAction(ctx, *pldtypes.RandAddress(), 12345, AsyncRequestType(42))
	require.NoError(t, err)
}

func TestDispatchCompletedActionForNonInflightIgnored(t *testing.T) {
	ctx, txm, _, done := newTestPublicTxManager(t, false)
	defer done()

	err := txm.dispatchAction(ctx, *pldtypes.RandAddress(), 12345, ActionCompleted)
	require.NoError(t, err)
}
