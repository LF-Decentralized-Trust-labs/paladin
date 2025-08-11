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

package fungible

import (
	"context"
	"errors"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestPrepareInputs(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	callbacks := testCallbacks
	coinSchema := &prototk.StateSchema{
		Id: "coin",
	}

	stateQueryContext := "test"
	ctx := context.Background()
	_, _, _, err := prepareInputsForTransfer(ctx, callbacks, coinSchema, false, stateQueryContext, "Alice", []*types.FungibleTransferParamEntry{{Amount: pldtypes.Uint64ToUint256(100)}})
	assert.EqualError(t, err, "PD210032: Failed to query the state store for available coins. test error")

	testCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{}, nil
	}
	_, _, _, err = prepareInputsForTransfer(ctx, callbacks, coinSchema, false, stateQueryContext, "Alice", []*types.FungibleTransferParamEntry{{Amount: pldtypes.Uint64ToUint256(100)}})
	assert.EqualError(t, err, "PD210033: Insufficient funds (available=0)")

	testCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					Id:       "state-1",
					DataJson: "bad json",
				},
			},
		}, nil
	}
	_, _, _, err = prepareInputsForTransfer(ctx, callbacks, coinSchema, false, stateQueryContext, "Alice", []*types.FungibleTransferParamEntry{{Amount: pldtypes.Uint64ToUint256(100)}})
	assert.EqualError(t, err, "PD210034: Coin state-1 is invalid: invalid character 'b' looking for beginning of value")

	testCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{Id: "state-1", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-2", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-3", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-4", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-5", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-6", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-7", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-8", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-9", DataJson: "{\"amount\": \"10\"}"},
				{Id: "state-10", DataJson: "{\"amount\": \"10\"}"},
			},
		}, nil
	}
	_, _, _, err = prepareInputsForTransfer(ctx, callbacks, coinSchema, false, stateQueryContext, "Alice", []*types.FungibleTransferParamEntry{{Amount: pldtypes.Uint64ToUint256(200)}})
	assert.EqualError(t, err, "PD210035: Need more than maximum number (10) of coins to fulfill the transfer amount total")
}
