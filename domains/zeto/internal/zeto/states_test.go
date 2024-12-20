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

package zeto

import (
	"context"
	"errors"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestGetStateSchemas(t *testing.T) {
	schemas, err := getStateSchemas(context.Background())
	assert.NoError(t, err)
	assert.Len(t, schemas, 3)
}

func TestPrepareInputs(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	zeto := &Zeto{
		name:      "test1",
		Callbacks: testCallbacks,
		coinSchema: &prototk.StateSchema{
			Id: "coin",
		},
		merkleTreeRootSchema: &prototk.StateSchema{
			Id: "merkle_tree_root",
		},
		merkleTreeNodeSchema: &prototk.StateSchema{
			Id: "merkle_tree_node",
		},
	}

	stateQueryContext := "test"
	ctx := context.Background()
	_, _, _, _, err := zeto.prepareInputsForTransfer(ctx, false, stateQueryContext, "Alice", []*types.TransferParamEntry{{Amount: tktypes.Uint64ToUint256(100)}})
	assert.EqualError(t, err, "PD210032: Failed to query the state store for available coins. test error")

	testCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{}, nil
	}
	_, _, _, _, err = zeto.prepareInputsForTransfer(ctx, false, stateQueryContext, "Alice", []*types.TransferParamEntry{{Amount: tktypes.Uint64ToUint256(100)}})
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
	_, _, _, _, err = zeto.prepareInputsForTransfer(ctx, false, stateQueryContext, "Alice", []*types.TransferParamEntry{{Amount: tktypes.Uint64ToUint256(100)}})
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
	_, _, _, _, err = zeto.prepareInputsForTransfer(ctx, false, stateQueryContext, "Alice", []*types.TransferParamEntry{{Amount: tktypes.Uint64ToUint256(200)}})
	assert.EqualError(t, err, "PD210035: Need more than maximum number (10) of coins to fulfill the transfer amount total")

	_, _, _, _, err = zeto.prepareInputsForWithdraw(ctx, false, stateQueryContext, "Alice", tktypes.Uint64ToUint256(100))
	assert.NoError(t, err)
}

func TestPrepareOutputs(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	zeto := &Zeto{
		name:      "test1",
		Callbacks: testCallbacks,
		coinSchema: &prototk.StateSchema{
			Id: "coin",
		},
		merkleTreeRootSchema: &prototk.StateSchema{
			Id: "merkle_tree_root",
		},
		merkleTreeNodeSchema: &prototk.StateSchema{
			Id: "merkle_tree_node",
		},
	}

	ctx := context.Background()
	sender := &prototk.ResolvedVerifier{
		Lookup:   "Alice",
		Verifier: "7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025",
	}

	_, _, err := zeto.prepareOutputsForDeposit(ctx, false, tktypes.Uint64ToUint256(100), sender)
	assert.NoError(t, err)

	sender.Verifier = "bad key"
	_, _, err = zeto.prepareOutputForWithdraw(ctx, tktypes.Uint64ToUint256(100), sender)
	assert.ErrorContains(t, err, "PD210037: Failed load owner public key.")

	sender.Verifier = "7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"
	_, _, err = zeto.prepareOutputForWithdraw(ctx, tktypes.Uint64ToUint256(100), sender)
	assert.NoError(t, err)
}
