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
	"testing"

	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
)

func TestMintValidateParams(t *testing.T) {
	h := mintHandler{}
	ctx := context.Background()
	_, err := h.ValidateParams(ctx, nil, "bad json")
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")

	_, err = h.ValidateParams(ctx, nil, "{}")
	assert.EqualError(t, err, "parameter 'to' is required")

	_, err = h.ValidateParams(ctx, nil, "{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":0}")
	assert.EqualError(t, err, "parameter 'amount' must be greater than 0")

	_, err = h.ValidateParams(ctx, nil, "{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":-10}")
	assert.EqualError(t, err, "parameter 'amount' must be greater than 0")

	params, err := h.ValidateParams(ctx, nil, "{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":10}")
	assert.NoError(t, err)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", params.(*types.MintParams).To)
	assert.Equal(t, "0x0a", params.(*types.MintParams).Amount.String())
}

func TestMintInit(t *testing.T) {
	h := mintHandler{
		zeto: &Zeto{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.MintParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
	}
	req := &prototk.InitTransactionRequest{}
	res, err := h.Init(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.RequiredVerifiers, 1)
	assert.Equal(t, "Alice", res.RequiredVerifiers[0].Lookup)
	assert.Equal(t, zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, res.RequiredVerifiers[0].VerifierType)
	assert.Equal(t, zetosigner.AlgoDomainZetoSnarkBJJ("test1"), res.RequiredVerifiers[0].Algorithm)
}

func TestMintAssemble(t *testing.T) {
	h := mintHandler{
		zeto: &Zeto{
			name: "test1",
			coinSchema: &prototk.StateSchema{
				Id: "coin",
			},
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.MintParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Bob",
		},
	}
	req := &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{},
	}
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to resolve: Alice")

	req = &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Algorithm:    zetosigner.AlgoDomainZetoSnarkBJJ("test1"),
				VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				Verifier:     "0x1234567890123456789012345678901234567890",
			},
		},
	}
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to decode recipient public key. invalid compressed public key length: 20")

	privKey := babyjub.NewRandPrivKey()
	pubKey := privKey.Public()
	compressedKey := pubKey.Compress()
	req.ResolvedVerifiers[0].Verifier = compressedKey.String()
	tx.Params.(*types.MintParams).Amount = tktypes.MustParseHexUint256("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "inputs values not inside Finite Field")

	tx.Params.(*types.MintParams).Amount = tktypes.MustParseHexUint256("0x0f")
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.Equal(t, "coin", res.AssembledTransaction.OutputStates[0].SchemaId)
}

func TestMintEndorse(t *testing.T) {
	h := mintHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.MintParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Bob",
		},
	}

	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, prototk.EndorseTransactionResponse_ENDORSER_SUBMIT, res.EndorsementResult)
}

func TestMintPrepare(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}
	h := mintHandler{
		zeto: z,
	}
	txSpec := &prototk.TransactionSpecification{
		TransactionId: "bad hex",
		From:          "Bob",
	}
	tx := &types.ParsedTransaction{
		Params: &types.MintParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "tokenContract1",
		},
	}
	// privKey := babyjub.NewRandPrivKey()
	// ownerKey := privKey.Public()
	req := &prototk.PrepareTransactionRequest{
		OutputStates: []*prototk.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
		Transaction: txSpec,
	}
	// StateDataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}",
	ctx := context.Background()
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "failed to encode transaction data. failed to parse transaction id. PD020007: Invalid hex")

	txSpec.TransactionId = "0x1234567890123456789012345678901234567890"
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{
				{
					Name: "tokenContract2",
					Abi:  "{}",
				},
			},
		},
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "contract tokenContract1 not found")

	z.config.DomainContracts.Implementations = append(z.config.DomainContracts.Implementations, &types.DomainContract{
		Name: "tokenContract1",
		Abi:  "{}",
	})
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "json: cannot unmarshal object into Go value of type abi.ABI")

	z.config.DomainContracts.Implementations[1].Abi = "[{\"inputs\": [{\"internalType\": \"bytes32\",\"name\": \"transactionId\",\"type\": \"bytes32\"}],\"name\": \"mint\",\"outputs\": [],\"type\": \"function\"}]"
	_, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
}
