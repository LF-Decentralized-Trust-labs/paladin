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
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
)

func TestMintValidateParams(t *testing.T) {
	h := NewMintHandler(
		"test1",
		&prototk.StateSchema{Id: "coin"},
		&prototk.StateSchema{Id: "data"},
	)
	ctx := context.Background()
	_, err := h.ValidateParams(ctx, nil, "bad json")
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")

	_, err = h.ValidateParams(ctx, nil, "{}")
	assert.EqualError(t, err, "PD210024: No transfer parameters provided")

	_, err = h.ValidateParams(ctx, nil, "{\"mints\":{}}")
	assert.EqualError(t, err, "json: cannot unmarshal object into Go struct field FungibleMintParams.mints of type []*types.FungibleTransferParamEntry")

	_, err = h.ValidateParams(ctx, nil, "{\"mints\":[{}]}")
	assert.EqualError(t, err, "PD210025: Parameter 'to' is required (index=0)")

	_, err = h.ValidateParams(ctx, nil, "{\"mints\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":0}]}")
	assert.EqualError(t, err, "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)")

	_, err = h.ValidateParams(ctx, nil, "{\"mints\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":-10}]}")
	assert.EqualError(t, err, "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)")

	max := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(100), nil).Text(10)
	_, err = h.ValidateParams(ctx, nil, "{\"mints\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":"+max+"}]}")
	assert.EqualError(t, err, "PD210107: Total amount must be in the range (0, 2^100)")

	params, err := h.ValidateParams(ctx, nil, "{\"mints\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":10}]}")
	assert.NoError(t, err)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", params.([]*types.FungibleTransferParamEntry)[0].To)
	assert.Equal(t, "0x0a", params.([]*types.FungibleTransferParamEntry)[0].Amount.String())
}

func TestMintInit(t *testing.T) {
	h := mintHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: []*types.FungibleTransferParamEntry{
			{
				To:     "Alice",
				Amount: pldtypes.MustParseHexUint256("0x0a"),
			},
		},
	}
	req := &prototk.InitTransactionRequest{}
	res, err := h.Init(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.RequiredVerifiers, 1)
	assert.Equal(t, "Alice", res.RequiredVerifiers[0].Lookup)
	assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, res.RequiredVerifiers[0].VerifierType)
	assert.Equal(t, zetosignerapi.AlgoDomainZetoSnarkBJJ("test1"), res.RequiredVerifiers[0].Algorithm)
}

func TestMintAssemble(t *testing.T) {
	h := mintHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &prototk.StateSchema{
					Id: "coin",
				},
				DataSchema: &prototk.StateSchema{
					Id: "data",
				},
			},
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: []*types.FungibleTransferParamEntry{
			{
				To:     "Alice",
				Amount: pldtypes.MustParseHexUint256("0x0a"),
			},
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Bob",
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "Anon",
		},
	}
	req := &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{},
	}
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210036: Failed to resolve verifier: Alice")

	req = &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Algorithm:    zetosignerapi.AlgoDomainZetoSnarkBJJ("test1"),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
				Verifier:     "0x1234567890123456789012345678901234567890",
			},
		},
	}
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210037: Failed load owner public key. expected 32 bytes in hex string, got 20")

	privKey := babyjub.NewRandPrivKey()
	pubKey := privKey.Public()
	compressedKey := pubKey.Compress()
	req.ResolvedVerifiers[0].Verifier = compressedKey.String()
	tx.Params.([]*types.FungibleTransferParamEntry)[0].Amount = pldtypes.MustParseHexUint256("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210038: Failed to create new state. inputs values not inside Finite Field")

	tx.Params.([]*types.FungibleTransferParamEntry)[0].Amount = pldtypes.MustParseHexUint256("0x0f")
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_OK, res.AssemblyResult)
	assert.Equal(t, "coin", res.AssembledTransaction.OutputStates[0].SchemaId)
}

func TestMintEndorse(t *testing.T) {
	h := mintHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestMintPrepare(t *testing.T) {
	h := mintHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	txSpec := &prototk.TransactionSpecification{
		TransactionId: "bad hex",
		From:          "Bob",
	}
	tx := &types.ParsedTransaction{
		Params: []*types.FungibleTransferParamEntry{
			{
				To:     "Alice",
				Amount: pldtypes.MustParseHexUint256("0x0a"),
			},
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
	// StateDataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}",
	ctx := context.Background()
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210049: Failed to encode transaction data. PD210028: Failed to parse transaction id. PD020007: Invalid hex")

	txSpec.TransactionId = "0x87229d205a0f48bcf0da37542fc140a9bdfc3b4a55c0beffcb62efe25a770a7f"
	_, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
}
