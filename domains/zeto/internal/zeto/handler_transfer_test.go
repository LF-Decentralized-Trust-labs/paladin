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
	"errors"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestTransferValidateParams(t *testing.T) {
	h := transferHandler{}
	ctx := context.Background()
	_, err := h.ValidateParams(ctx, "bad json")
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")

	_, err = h.ValidateParams(ctx, "{}")
	assert.EqualError(t, err, "parameter 'to' is required")

	_, err = h.ValidateParams(ctx, "{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":0}")
	assert.NoError(t, err)
}

func TestTransferInit(t *testing.T) {
	h := transferHandler{
		zeto: &Zeto{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.TransferParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Bob",
		},
	}
	req, err := h.Init(ctx, tx, nil)
	assert.NoError(t, err)
	assert.Len(t, req.RequiredVerifiers, 2)
	assert.Equal(t, "Bob", req.RequiredVerifiers[0].Lookup)
	assert.Equal(t, h.zeto.getAlgoZetoSnarkBJJ(), req.RequiredVerifiers[0].Algorithm)
	assert.Equal(t, zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.RequiredVerifiers[0].VerifierType)
	assert.Equal(t, "Alice", req.RequiredVerifiers[1].Lookup)
}

func TestTransferAssemble(t *testing.T) {
	h := transferHandler{
		zeto: &Zeto{
			name: "test1",
			coinSchema: &prototk.StateSchema{
				Id: "coin",
			},
			merkleTreeRootSchema: &prototk.StateSchema{
				Id: "merkle_tree_root",
			},
			merkleTreeNodeSchema: &prototk.StateSchema{
				Id: "merkle_tree_node",
			},
		},
	}
	ctx := context.Background()
	txSpec := &prototk.TransactionSpecification{
		From:            "Bob",
		ContractAddress: "0x1234567890123456789012345678901234567890",
	}
	tx := &types.ParsedTransaction{
		Params: &types.TransferParams{
			To:     "Alice",
			Amount: tktypes.MustParseHexUint256("0x0a"),
		},
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "tokenContract1",
			CircuitId: "circuit1",
		},
	}
	req := &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x1234567890123456789012345678901234567890",
				Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
		Transaction: txSpec,
	}
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to resolve: Bob")

	req.ResolvedVerifiers[0].Lookup = "Bob"
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to resolve: Alice")

	req.ResolvedVerifiers = append(req.ResolvedVerifiers, &prototk.ResolvedVerifier{
		Lookup:       "Alice",
		Verifier:     "0x1234567890123456789012345678901234567890",
		Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosigner.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
	})
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to load sender public key. expected 32 bytes in hex string, got 20")

	req.ResolvedVerifiers[0].Verifier = "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed load receiver public key. expected 32 bytes in hex string, got 20")

	testCallbacks := &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h.zeto.Callbacks = testCallbacks
	req.ResolvedVerifiers[1].Verifier = "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922"
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "failed to prepare inputs. test error")

	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
				},
			},
		}, nil
	}
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.AssembledTransaction.OutputStates, 2) // one for the receiver Alice, one for self as change
	var coin1 types.ZetoCoin
	err = json.Unmarshal([]byte(res.AssembledTransaction.OutputStates[0].StateDataJson), &coin1)
	assert.NoError(t, err)
	assert.Equal(t, "Alice", coin1.Owner)
	assert.Equal(t, "0x0a", coin1.Amount.String())

	var coin2 types.ZetoCoin
	err = json.Unmarshal([]byte(res.AssembledTransaction.OutputStates[1].StateDataJson), &coin2)
	assert.NoError(t, err)
	assert.Equal(t, "Bob", coin2.Owner)
	assert.Equal(t, "0x05", coin2.Amount.String())

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	tx.DomainConfig.CircuitId = constants.CIRCUIT_ANON_NULLIFIER
	called := 0
	testCallbacks.returnFunc = func() (*prototk.FindAvailableStatesResponse, error) {
		var dataJson string
		if called == 0 {
			dataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}"
		} else if called == 1 {
			dataJson = "{\"rootIndex\": \"0x113842d0d1c21d02baf98d1ddbf9a88e6c2a8e2aaf48ee1e35659f7de53da5f0\"}"
		} else {
			dataJson = "{\"index\":\"0x113842d0d1c21d02baf98d1ddbf9a88e6c2a8e2aaf48ee1e35659f7de53da5f0\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x113842d0d1c21d02baf98d1ddbf9a88e6c2a8e2aaf48ee1e35659f7de53da5f0\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}"
		}
		called++
		return &prototk.FindAvailableStatesResponse{
			States: []*prototk.StoredState{
				{
					DataJson: dataJson,
				},
			},
		}, nil

	}
	res, err = h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.AssembledTransaction.OutputStates, 2)
}

func TestTransferEndorse(t *testing.T) {
	h := transferHandler{}
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

func TestTransferPrepare(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}
	h := transferHandler{
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
			TokenName: constants.TOKEN_ANON_ENC,
		},
	}
	req := &prototk.PrepareTransactionRequest{
		InputStates: []*prototk.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
		OutputStates: []*prototk.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
		Transaction: txSpec,
	}
	ctx := context.Background()
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "did not find 'sender' attestation")

	at := zetosigner.PAYLOAD_DOMAIN_ZETO_SNARK
	req.AttestationResult = []*prototk.AttestationResult{
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_ENDORSE,
			PayloadType:     &at,
			Payload:         []byte("bad payload"),
		},
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "failed to unmarshal proving response")

	proofReq := corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"0x1234567890", "0x1234567890"},
			B: []*corepb.B_Item{
				{
					Items: []string{"0x1234567890", "0x1234567890"},
				},
				{
					Items: []string{"0x1234567890", "0x1234567890"},
				},
			},
			C: []string{"0x1234567890", "0x1234567890"},
		},
		PublicInputs: map[string]string{
			"encryptionNonce": "0x1234567890",
			"encryptedValues": "0x1234567890,0x1234567890",
		},
	}
	payload, err := proto.Marshal(&proofReq)
	assert.NoError(t, err)
	req.AttestationResult[0].Payload = payload
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "failed to parse input states. invalid character 'b' looking for beginning of value")

	req.InputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Alice\",\"ownerKey\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "failed to parse output states. invalid character 'b' looking for beginning of value")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"Bob\",\"ownerKey\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "failed to encode transaction data. failed to parse transaction id. PD020007: Invalid hex")

	txSpec.TransactionId = "0x1234567890123456789012345678901234567890123456789012345678901234"
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{},
		},
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "failed to find abi for the token contract Zeto_AnonEnc. contract Zeto_AnonEnc not found")

	z.config.DomainContracts.Implementations = []*types.DomainContract{
		{
			Name: constants.TOKEN_ANON_ENC,
			Abi:  "{}",
		},
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "failed to find abi for the token contract Zeto_AnonEnc. json: cannot unmarshal object into Go value of type abi.ABI")

	z.config.DomainContracts.Implementations[0].Abi = "[{\"inputs\": [{\"internalType\": \"bytes32\",\"name\": \"transactionId\",\"type\": \"bytes32\"}],\"name\": \"transfer\",\"outputs\": [],\"type\": \"function\"}]"
	res, err := h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"data\":\"0x000100001234567890123456789012345678901234567890123456789012345678901234\",\"encryptedValues\":[\"0x1234567890\",\"0x1234567890\"],\"encryptionNonce\":\"0x1234567890\",\"inputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"outputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]}}", res.Transaction.ParamsJson)

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	tx.DomainConfig.CircuitId = constants.CIRCUIT_ANON_NULLIFIER
	proofReq.PublicInputs["nullifiers"] = "0x1234567890,0x1234567890"
	proofReq.PublicInputs["root"] = "0x1234567890"
	payload, err = proto.Marshal(&proofReq)
	assert.NoError(t, err)
	req.AttestationResult[0].Payload = payload
	z.config.DomainContracts.Implementations[0].Name = constants.TOKEN_ANON_NULLIFIER
	res, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"data\":\"0x000100001234567890123456789012345678901234567890123456789012345678901234\",\"nullifiers\":[\"0x1234567890\",\"0x1234567890\"],\"outputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]},\"root\":\"0x1234567890\"}", res.Transaction.ParamsJson)
}

type testDomainCallbacks struct {
	returnFunc func() (*prototk.FindAvailableStatesResponse, error)
}

func (dc *testDomainCallbacks) FindAvailableStates(ctx context.Context, req *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error) {
	return dc.returnFunc()
}

func (dc *testDomainCallbacks) EncodeData(ctx context.Context, req *prototk.EncodeDataRequest) (*prototk.EncodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) RecoverSigner(ctx context.Context, req *prototk.RecoverSignerRequest) (*prototk.RecoverSignerResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) DecodeData(context.Context, *prototk.DecodeDataRequest) (*prototk.DecodeDataResponse, error) {
	return nil, nil
}
