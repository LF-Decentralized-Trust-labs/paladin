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
	"math/big"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func sampleTransferPayload() map[string]any {
	return map[string]interface{}{
		"inputs":  []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
		"outputs": []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
		"proof": map[string]interface{}{
			"pA": []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
			"pB": [][]string{
				{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
				{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
			},
			"pC": []string{"0x1234567890123456789012345678901234567890", "0x1234567890123456789012345678901234567890"},
		},
		"data": "0xfeedbeef",
	}
}

func TestLockValidateParams(t *testing.T) {
	h := lockHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	config := &types.DomainInstanceConfig{
		TokenName: "test",
		Circuits: &zetosignerapi.Circuits{
			"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
		},
	}
	ctx := context.Background()
	_, err := h.ValidateParams(ctx, config, "bad json")
	assert.EqualError(t, err, "PD210059: Failed to unmarshal lock parameters. invalid character 'b' looking for beginning of value")

	config.TokenName = "Zeto_Anon"
	_, err = h.ValidateParams(ctx, config, "{\"delegate\":\"0x1234567890123456789012345678901234567890\"}")
	assert.ErrorContains(t, err, "PD210026: Parameter 'amount' is required (index=0)")

	_, err = h.ValidateParams(ctx, config, "{\"amount\":-10,\"delegate\":\"0x1234567890123456789012345678901234567890\"}")
	assert.ErrorContains(t, err, "PD210107: Total amount must be in the range (0, 2^100)")

	amt1 := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(100), nil) // max allowed
	amt1.Sub(amt1, big.NewInt(1))
	_, err = h.ValidateParams(ctx, nil, "{\"amount\":"+amt1.Text(10)+",\"delegate\":\"0x1234567890123456789012345678901234567890\"}")
	assert.NoError(t, err)
	amt1.Add(amt1, big.NewInt(2))
	_, err = h.ValidateParams(ctx, nil, "{\"amount\":"+amt1.Text(10)+",\"delegate\":\"0x1234567890123456789012345678901234567890\"}")
	assert.EqualError(t, err, "PD210107: Total amount must be in the range (0, 2^100)")
}

func TestLockInit(t *testing.T) {
	h := lockHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: pldtypes.RandAddress(),
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Alice",
		},
	}
	req := &prototk.InitTransactionRequest{}
	res, err := h.Init(ctx, tx, req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestLockAssemble(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
					},
					{
						DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
					},
				},
			}, nil
		},
	}

	h := lockHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &prototk.StateSchema{
					Id: "coin",
				},
			},
		},
		callbacks: testCallbacks,
	}
	ctx := context.Background()

	config := &types.DomainInstanceConfig{
		TokenName: "test1",
		Circuits: &zetosignerapi.Circuits{
			"deposit":  &zetosignerapi.Circuit{Name: "circuit-deposit"},
			"transfer": &zetosignerapi.Circuit{Name: "circuit-transfer"},
		},
	}

	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Amount:   pldtypes.Uint64ToUint256(100),
			Delegate: pldtypes.RandAddress(),
		},
		DomainConfig: config,
		Transaction: &prototk.TransactionSpecification{
			From: "Alice",
		},
	}
	req := &prototk.AssembleTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			ContractInfo: &prototk.ContractInfo{
				ContractAddress: "0x1234567890123456789012345678901234567890",
			},
		},
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Bob",
				Verifier:     "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922",
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
	}

	// Missing verifier for sender
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210036: Failed to resolve verifier: Alice")
	req.ResolvedVerifiers[0].Lookup = "Alice"

	badCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h.callbacks = badCallbacks

	// Error querying states
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210039: Failed to prepare transaction inputs. PD210032: Failed to query the state store for available coins. test error")
	h.callbacks = testCallbacks

	// Bad contract address
	req.Transaction.ContractInfo.ContractAddress = "bad hex"
	_, err = h.Assemble(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210017: Failed to decode contract address. bad address")

	h.callbacks = &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{}, nil
		},
	}

	// No states found
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "PD210033: Insufficient funds (available=0)", *res.RevertReason)
	h.callbacks = testCallbacks

	// Successful assembly
	req.Transaction.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	h.callbacks = testCallbacks
	res, err = h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestLockEndorse(t *testing.T) {
	h := lockHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestLockPrepare(t *testing.T) {
	h := lockHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Params: &types.LockParams{
			Delegate: pldtypes.RandAddress(),
		},
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "test1",
		},
		Transaction: &prototk.TransactionSpecification{
			From: "Alice",
		},
	}

	at := zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK
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
	payload, _ := proto.Marshal(&proofReq)
	req := &prototk.PrepareTransactionRequest{
		Transaction: &prototk.TransactionSpecification{
			TransactionId: "bad hex",
		},
		InputStates: []*prototk.EndorsableState{
			{
				StateDataJson: "bad json",
			},
		},
		InfoStates: []*prototk.EndorsableState{
			{
				StateDataJson: "bad json",
			},
		},
		OutputStates: []*prototk.EndorsableState{
			{
				StateDataJson: "bad json",
			},
		},
	}
	_, err := h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210043: Did not find 'sender' attestation")

	req.AttestationResult = append(req.AttestationResult, &prototk.AttestationResult{
		Name:            "sender",
		AttestationType: prototk.AttestationType_ENDORSE,
		PayloadType:     &at,
		Payload:         payload,
	})
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210045: Failed to parse input states.")

	req.InputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210087: Failed to unmarshal state data")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210049: Failed to encode transaction data. PD020008: Failed to parse value as 32 byte hex string")

	req.Transaction.TransactionId = "0x87229d205a0f48bcf0da37542fc140a9bdfc3b4a55c0beffcb62efe25a770a7f"
	req.AttestationResult[0].Payload = []byte("bad json")
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210044: Failed to unmarshal proving response.", "cannot parse invalid wire-format data")

	tx.DomainConfig.TokenName = "Zeto_Anon"
	tx.Params = &types.LockParams{
		Delegate: pldtypes.RandAddress(),
	}
	req.AttestationResult[0].Payload = payload
	req.InfoStates = nil
	_, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)

	tx.DomainConfig.TokenName = "Zeto_AnonNullifier"
	_, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
}
