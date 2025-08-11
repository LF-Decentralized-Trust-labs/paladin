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

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestBalanceOfValidateParams(t *testing.T) {
	h := NewBalanceOfHandler("test1", nil, &pb.StateSchema{
		Id: "coin",
	})
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expectedErr string
		validate    func(t *testing.T, result interface{}, err error)
	}{
		{
			name:  "Valid balanceOf input",
			input: `{"account":"0x1234567890123456789012345678901234567890"}`,
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "0x1234567890123456789012345678901234567890", result.(*types.FungibleBalanceOfParam).Account)
			},
		},
		{
			name:        "Invalid JSON",
			input:       "bad json",
			expectedErr: "invalid character 'b' looking for beginning of value",
		},
		{
			name:        "No parameters",
			input:       "{}",
			expectedErr: "PD210135: Parameter 'account' is required",
		},
		{
			name:        "Invalid balanceOf structure",
			input:       "{\"transfers\":{}}",
			expectedErr: "PD210135: Parameter 'account' is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := h.ValidateParams(ctx, nil, tc.input)

			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			}

			if tc.validate != nil {
				tc.validate(t, result, err)
			}
		})
	}
}

func TestBalanceOfInitCall(t *testing.T) {
	h := balanceOfHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.FungibleBalanceOfParam{
			Account: "Alice",
		},
		Transaction: &pb.TransactionSpecification{
			From: "Bob",
		},
	}
	req, err := h.InitCall(ctx, tx, nil)
	assert.NoError(t, err)
	assert.Len(t, req.RequiredVerifiers, 1)
	assert.Equal(t, "Alice", req.RequiredVerifiers[0].Lookup)
	assert.Equal(t, h.getAlgoZetoSnarkBJJ(), req.RequiredVerifiers[0].Algorithm)
	assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.RequiredVerifiers[0].VerifierType)
}

func TestBalanceOfExecCall(t *testing.T) {
	h := balanceOfHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &pb.StateSchema{
					Id: "coin",
				},
			},
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.FungibleBalanceOfParam{
			Account: "Alice",
		},
		Transaction: &pb.TransactionSpecification{
			From: "Bob",
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "tokenContract1",
		},
	}

	// Missing verifier for account
	req := &pb.ExecCallRequest{
		ResolvedVerifiers: []*pb.ResolvedVerifier{},
		StateQueryContext: "query123",
	}
	_, err := h.ExecCall(ctx, tx, req)
	assert.EqualError(t, err, "PD210036: Failed to resolve verifier: Alice")

	// Add correct verifier for the account (Account field)
	req = &pb.ExecCallRequest{
		ResolvedVerifiers: []*pb.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x1234567890123456789012345678901234567890",
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
		StateQueryContext: "query123",
	}

	// Error querying states
	testCallbacks := &testDomainCallbacks{
		returnFunc: func() (*pb.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h.callbacks = testCallbacks
	_, err = h.ExecCall(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210138: Failed to get account balance. Alice: PD210032: Failed to query the state store for available coins. test error")

	// No states found - balance should be 0
	testCallbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{},
		}, nil
	}
	res, err := h.ExecCall(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, `{"totalBalance":"0x00","totalStates":"0x00","overflow":false}`, res.ResultJson)

	// Single coin state found
	testCallbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
				},
			},
		}, nil
	}
	res, err = h.ExecCall(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, `{"totalBalance":"0x0f","totalStates":"0x01","overflow":false}`, res.ResultJson)

	// Multiple coin states found - should sum them
	testCallbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0a\"}",
				},
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x05\"}",
				},
			},
		}, nil
	}
	res, err = h.ExecCall(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, `{"totalBalance":"0x0f","totalStates":"0x02","overflow":false}`, res.ResultJson)

	// Test with nullifiers token type
	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	res, err = h.ExecCall(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, `{"totalBalance":"0x0f","totalStates":"0x02","overflow":false}`, res.ResultJson)

	// Test malformed coin data
	testCallbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "bad json",
				},
			},
		}, nil
	}
	_, err = h.ExecCall(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210138: Failed to get account balance. Alice: PD210034: Coin  is invalid: invalid character 'b' looking for beginning of value")
}
