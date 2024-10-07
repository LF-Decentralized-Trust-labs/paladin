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
	"encoding/json"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestLockValidateParams(t *testing.T) {
	h := lockHandler{
		zeto: &Zeto{
			name: "test1",
			config: &types.DomainFactoryConfig{
				DomainContracts: types.DomainConfigContracts{
					Implementations: []*types.DomainContract{
						{
							Name:      "Zeto_Anon",
							CircuitId: "anon",
							Abi:       "[{}]",
						},
					},
				},
			},
		},
	}
	config := &types.DomainInstanceConfig{
		TokenName: "test",
		CircuitId: "test",
	}
	ctx := context.Background()
	_, err := h.ValidateParams(ctx, config, "bad json")
	assert.EqualError(t, err, "failed to unmarshal lockProof parameters. invalid character 'b' looking for beginning of value")

	_, err = h.ValidateParams(ctx, config, "{}")
	assert.EqualError(t, err, "failed to decode the transfer call. contract test not found")

	config.TokenName = "Zeto_Anon"
	_, err = h.ValidateParams(ctx, config, "{}")
	assert.EqualError(t, err, "failed to decode the transfer call. Unknown function: transfer")

	h.zeto.config.DomainContracts.Implementations[0].Abi = "[{\"inputs\": [{\"internalType\": \"uint256[2]\",\"name\": \"inputs\",\"type\": \"uint256[2]\"}],\"name\": \"transfer\",\"outputs\": [],\"type\": \"function\"}]"
	lockParams := types.LockParams{
		Delegate: *tktypes.RandAddress(),
		Call:     tktypes.HexBytes([]byte("bad call")),
	}
	jsonBytes, err := json.Marshal(lockParams)
	assert.NoError(t, err)
	_, err = h.ValidateParams(ctx, config, string(jsonBytes))
	assert.ErrorContains(t, err, "failed to decode the transfer call. FF22049: Incorrect ID for signature transfer(uint256[2])")

	contractAbi, err := h.zeto.config.GetContractAbi(config.TokenName)
	assert.NoError(t, err)
	transfer := contractAbi.Functions()["transfer"]
	assert.NoError(t, err)
	params := map[string]interface{}{
		"inputs": []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
	}
	bytes, err := transfer.EncodeCallDataValues(params)
	assert.NoError(t, err)
	lockParams = types.LockParams{
		Delegate: *tktypes.RandAddress(),
		Call:     tktypes.HexBytes(bytes),
	}
	jsonBytes, err = json.Marshal(lockParams)
	assert.NoError(t, err)
	res, err := h.ValidateParams(ctx, config, string(jsonBytes))
	assert.NoError(t, err)
	assert.Equal(t, lockParams, *res.(*types.LockParams))
}

func TestLocktInit(t *testing.T) {
	h := lockHandler{
		zeto: &Zeto{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: *tktypes.RandAddress(),
			Call:     tktypes.HexBytes([]byte{0x01, 0x02, 0x03}),
		},
	}
	req := &prototk.InitTransactionRequest{}
	res, err := h.Init(ctx, tx, req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestLockAssemble(t *testing.T) {
	h := lockHandler{
		zeto: &Zeto{
			name: "test1",
			coinSchema: &prototk.StateSchema{
				Id: "coin",
			},
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: *tktypes.RandAddress(),
			Call:     tktypes.HexBytes([]byte{0x01, 0x02, 0x03}),
		},
		Transaction: &prototk.TransactionSpecification{},
	}
	req := &prototk.AssembleTransactionRequest{}
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestLockEndorse(t *testing.T) {
	h := lockHandler{
		zeto: &Zeto{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: *tktypes.RandAddress(),
			Call:     tktypes.HexBytes([]byte{0x01, 0x02, 0x03}),
		},
	}
	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestLockPrepare(t *testing.T) {
	h := lockHandler{
		zeto: &Zeto{
			name: "test1",
			config: &types.DomainFactoryConfig{
				DomainContracts: types.DomainConfigContracts{
					Implementations: []*types.DomainContract{
						{
							Name:      "Zeto_Anon",
							CircuitId: "anon",
							Abi:       "[{\"inputs\": [{\"internalType\": \"uint256[2]\",\"name\": \"inputs\",\"type\": \"uint256[2]\"}],\"name\": \"transfer\",\"outputs\": [],\"type\": \"function\"}]",
						},
					},
				},
			},
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: *tktypes.RandAddress(),
			Call:     tktypes.HexBytes([]byte{0x01, 0x02, 0x03}),
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "test",
		},
	}
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "bad hex",
		},
	}
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "failed to decode transfer call data. contract test not found")

	tx.DomainConfig.TokenName = "Zeto_Anon"
	contractAbi, err := h.zeto.config.GetContractAbi("Zeto_Anon")
	assert.NoError(t, err)
	transfer := contractAbi.Functions()["transfer"]
	assert.NoError(t, err)
	params := map[string]interface{}{
		"inputs": []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
	}
	bytes, err := transfer.EncodeCallDataValues(params)
	assert.NoError(t, err)
	tx.Params = &types.LockParams{
		Delegate: *tktypes.RandAddress(),
		Call:     tktypes.HexBytes(bytes),
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "failed to encode transaction data. failed to parse transaction id. PD020007: Invalid hex")

	req.Transaction.TransactionId = "0x1234567890123456789012345678901234567890"
	_, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "", req.Transaction.FunctionAbiJson)
}
