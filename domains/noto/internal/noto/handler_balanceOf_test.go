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

package noto

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceOfValidateParams(t *testing.T) {
	h := balanceOfHandler{}
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expectedErr string
		validate    func(t *testing.T, result interface{}, err error)
	}{
		{
			name:  "Valid balanceOf input",
			input: `{"account":"alice@node1"}`,
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "alice@node1", result.(*types.BalanceOfParam).Account)
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
			expectedErr: "PD200007: Parameter 'Account' is required",
		},
		{
			name:        "Empty account",
			input:       `{"account":""}`,
			expectedErr: "PD200007: Parameter 'Account' is required",
		},
		{
			name:        "Invalid balanceOf structure",
			input:       `{"transfers":{}}`,
			expectedErr: "PD200007: Parameter 'Account' is required",
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
	n := &Noto{
		Callbacks: mockCallbacks,
	}
	h := balanceOfHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: &types.BalanceOfParam{
			Account: "alice@node1",
		},
		DomainConfig: &types.NotoParsedConfig{
			NotaryLookup: "notary@node1",
		},
	}

	req := &prototk.InitCallRequest{}
	res, err := h.InitCall(ctx, parsedTx, req)

	assert.NoError(t, err)
	assert.Len(t, res.RequiredVerifiers, 2)
	assert.Equal(t, "notary@node1", res.RequiredVerifiers[0].Lookup)
	assert.Equal(t, "alice@node1", res.RequiredVerifiers[1].Lookup)
	assert.Equal(t, algorithms.ECDSA_SECP256K1, res.RequiredVerifiers[0].Algorithm)
	assert.Equal(t, verifiers.ETH_ADDRESS, res.RequiredVerifiers[0].VerifierType)
}

func TestBalanceOfExecCall(t *testing.T) {
	aliceKey, err := secp256k1.GenerateSecp256k1KeyPair()
	require.NoError(t, err)

	n := &Noto{
		Callbacks:  mockCallbacks,
		coinSchema: &prototk.StateSchema{Id: "coin"},
	}
	h := balanceOfHandler{noto: n}
	ctx := context.Background()

	parsedTx := &types.ParsedTransaction{
		Params: &types.BalanceOfParam{
			Account: "alice@node1",
		},
		DomainConfig: &types.NotoParsedConfig{
			NotaryLookup: "notary@node1",
		},
	}

	t.Run("Missing verifier for account", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{},
			StateQueryContext: "query123",
		}
		_, err := h.ExecCall(ctx, parsedTx, req)
		assert.Regexp(t, "PD200011: Error verifying 'account' address", err)
	})

	t.Run("Error querying states", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, assert.AnError
		}

		_, err := h.ExecCall(ctx, parsedTx, req)
		assert.ErrorContains(t, err, "Failed to get account balance")
		assert.ErrorContains(t, err, "alice@node1")
	})

	t.Run("No states found - balance should be 0", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{},
			}, nil
		}

		res, err := h.ExecCall(ctx, parsedTx, req)
		assert.NoError(t, err)
		assert.Equal(t, `{"totalBalance":"0x00","totalStates":"0x00","overflow":false}`, res.ResultJson)
	})

	t.Run("Single coin state found", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		coin := &types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&aliceKey.Address),
			Amount: pldtypes.Int64ToInt256(100),
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: mustParseJSON(coin),
					},
				},
			}, nil
		}

		res, err := h.ExecCall(ctx, parsedTx, req)
		assert.NoError(t, err)
		assert.Equal(t, `{"totalBalance":"0x64","totalStates":"0x01","overflow":false}`, res.ResultJson)
	})

	t.Run("Multiple coin states found - should sum them", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		coin1 := &types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&aliceKey.Address),
			Amount: pldtypes.Int64ToInt256(75),
		}
		coin2 := &types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&aliceKey.Address),
			Amount: pldtypes.Int64ToInt256(25),
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: mustParseJSON(coin1),
					},
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: mustParseJSON(coin2),
					},
				},
			}, nil
		}

		res, err := h.ExecCall(ctx, parsedTx, req)
		assert.NoError(t, err)
		assert.Equal(t, `{"totalBalance":"0x64","totalStates":"0x02","overflow":false}`, res.ResultJson)
	})

	t.Run("Test malformed coin data", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: "bad json",
					},
				},
			}, nil
		}

		_, err := h.ExecCall(ctx, parsedTx, req)
		assert.ErrorContains(t, err, "Failed to get account balance")
		assert.ErrorContains(t, err, "alice@node1")
	})

	t.Run("Test with zero amount coins", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		coin := &types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&aliceKey.Address),
			Amount: pldtypes.Int64ToInt256(0),
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: mustParseJSON(coin),
					},
				},
			}, nil
		}

		res, err := h.ExecCall(ctx, parsedTx, req)
		assert.NoError(t, err)
		assert.Equal(t, `{"totalBalance":"0x00","totalStates":"0x01","overflow":false}`, res.ResultJson)
	})

	t.Run("Test with large amounts", func(t *testing.T) {
		req := &prototk.ExecCallRequest{
			ResolvedVerifiers: []*prototk.ResolvedVerifier{
				{
					Lookup:       "alice@node1",
					Verifier:     aliceKey.Address.String(),
					Algorithm:    algorithms.ECDSA_SECP256K1,
					VerifierType: verifiers.ETH_ADDRESS,
				},
			},
			StateQueryContext: "query123",
		}

		// Large amount that would exceed int64
		largeAmount := pldtypes.MustParseHexUint256("0x1fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		coin := &types.NotoCoin{
			Salt:   pldtypes.RandBytes32(),
			Owner:  (*pldtypes.EthAddress)(&aliceKey.Address),
			Amount: largeAmount,
		}

		mockCallbacks.MockFindAvailableStates = func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						Id:       pldtypes.RandBytes32().String(),
						SchemaId: "coin",
						DataJson: mustParseJSON(coin),
					},
				},
			}, nil
		}

		res, err := h.ExecCall(ctx, parsedTx, req)
		assert.NoError(t, err)
		expected := `{"totalBalance":"` + largeAmount.HexString0xPrefix() + `","totalStates":"0x01","overflow":false}`
		assert.Equal(t, expected, res.ResultJson)
	})
}
