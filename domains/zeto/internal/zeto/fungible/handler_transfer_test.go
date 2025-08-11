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
	"encoding/json"
	"errors"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/smt"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestTransferValidateParams(t *testing.T) {
	h := NewTransferHandler(
		"test1",
		nil,
		&pb.StateSchema{Id: "coin"},
		&pb.StateSchema{Id: "merkle_tree_root"},
		&pb.StateSchema{Id: "merkle_tree_node"},
		&pb.StateSchema{Id: "data"},
	)
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expectedErr string
		validate    func(t *testing.T, result interface{}, err error)
	}{
		{
			name:  "Valid transfer within range",
			input: "{\"transfers\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":1267650600228229401496703205375}]}",
			validate: func(t *testing.T, result interface{}, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "0x1234567890123456789012345678901234567890", result.([]*types.FungibleTransferParamEntry)[0].To)
			},
		},
		{
			name:        "Invalid JSON",
			input:       "bad json",
			expectedErr: "invalid character 'b' looking for beginning of value",
		},
		{
			name:        "No transfer parameters",
			input:       "{}",
			expectedErr: "PD210024: No transfer parameters provided",
		},
		{
			name:        "Invalid transfers structure",
			input:       "{\"transfers\":{}}",
			expectedErr: "json: cannot unmarshal object into Go struct field FungibleTransferParams.transfers of type []*types.FungibleTransferParamEntry",
		},
		{
			name:        "Missing 'to' parameter",
			input:       "{\"transfers\":[{}]}",
			expectedErr: "PD210025: Parameter 'to' is required (index=0)",
		},
		{
			name:        "Invalid 'amount' parameter (0)",
			input:       "{\"transfers\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":0}]}",
			expectedErr: "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)",
		},
		{
			name:        "Invalid 'amount' parameter (-10)",
			input:       "{\"transfers\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":-10}]}",
			expectedErr: "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)",
		},
		{
			name:        "Total amount exceeds range",
			input:       "{\"transfers\":[{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":1267650600228229401496703205375},{\"to\":\"0x1234567890123456789012345678901234567890\",\"amount\":1000}]}",
			expectedErr: "PD210107: Total amount must be in the range (0, 2^100)",
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

func TestTransferInit(t *testing.T) {
	h := transferHandler{
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
		Transaction: &pb.TransactionSpecification{
			From: "Bob",
		},
	}
	req, err := h.Init(ctx, tx, nil)
	assert.NoError(t, err)
	assert.Len(t, req.RequiredVerifiers, 2)
	assert.Equal(t, "Bob", req.RequiredVerifiers[0].Lookup)
	assert.Equal(t, h.getAlgoZetoSnarkBJJ(), req.RequiredVerifiers[0].Algorithm)
	assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, req.RequiredVerifiers[0].VerifierType)
	assert.Equal(t, "Alice", req.RequiredVerifiers[1].Lookup)
}

func TestTransferAssemble(t *testing.T) {
	h := transferHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &pb.StateSchema{
					Id: "coin",
				},
				MerkleTreeRootSchema: &pb.StateSchema{
					Id: "merkle_tree_root",
				},
				MerkleTreeNodeSchema: &pb.StateSchema{
					Id: "merkle_tree_node",
				},
				DataSchema: &prototk.StateSchema{
					Id: "data",
				},
			},
		},
	}
	ctx := context.Background()
	txSpec := &pb.TransactionSpecification{
		From: "Bob",
		ContractInfo: &pb.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
		},
	}
	tx := &types.ParsedTransaction{
		Params: []*types.FungibleTransferParamEntry{
			{
				To:     "Alice",
				Amount: pldtypes.MustParseHexUint256("0x09"),
			},
		},
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "tokenContract1",
			Circuits: &zetosignerapi.Circuits{
				"deposit":  &zetosignerapi.Circuit{Name: "circuit-deposit"},
				"transfer": &zetosignerapi.Circuit{Name: "circuit-transfer"},
			},
		},
	}

	// Missing verifier for sender
	req := &pb.AssembleTransactionRequest{
		ResolvedVerifiers: []*pb.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x1234567890123456789012345678901234567890",
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
		Transaction: txSpec,
	}
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210036: Failed to resolve verifier: Bob")

	// Error querying states
	req.ResolvedVerifiers = append(req.ResolvedVerifiers, &pb.ResolvedVerifier{
		Lookup:       "Bob",
		Verifier:     "0x1234567890123456789012345678901234567890",
		Algorithm:    h.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
	})
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*pb.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h.callbacks = testCallbacks
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210039: Failed to prepare transaction inputs. PD210032: Failed to query the state store for available coins. test error")

	calls := 0
	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		defer func() { calls++ }()
		if calls == 0 {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
					},
				},
			}, nil
		} else {
			return nil, errors.New("test error")
		}
	}

	// Malformed verifier key
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210040: Failed to prepare transaction outputs. PD210037: Failed load owner public key. expected 32 bytes in hex string, got 20")

	calls = 0
	req.ResolvedVerifiers[0].Verifier = "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922"
	req.ResolvedVerifiers[1].Verifier = "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"

	// Missing verifier for receiver
	req.ResolvedVerifiers[0].Lookup = "Bob"
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210040: Failed to prepare transaction outputs. PD210036: Failed to resolve verifier: Alice")
	req.ResolvedVerifiers[0].Lookup = "Alice"

	// Initially find a state, but error when looking for more states
	_, err = h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210039: Failed to prepare transaction inputs. PD210032: Failed to query the state store for available coins. test error")

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{},
		}, nil
	}

	// No states found
	res, err := h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, prototk.AssembleTransactionResponse_REVERT, res.AssemblyResult)
	assert.Equal(t, "PD210033: Insufficient funds (available=0)", *res.RevertReason)

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
				},
			},
		}, nil
	}

	// Successful assembly with 1 input state
	res, err = h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.AssembledTransaction.InputStates, 1)
	assert.Len(t, res.AssembledTransaction.OutputStates, 2) // one for the receiver Alice, one for self as change
	var coin1 types.ZetoCoin
	err = json.Unmarshal([]byte(res.AssembledTransaction.OutputStates[0].StateDataJson), &coin1)
	assert.NoError(t, err)
	assert.Equal(t, "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922", coin1.Owner.String())
	assert.Equal(t, "0x09", coin1.Amount.String())

	assert.Len(t, res.AssembledTransaction.OutputStates[0].DistributionList, 1)
	assert.Equal(t, "Alice", res.AssembledTransaction.OutputStates[0].DistributionList[0])

	var coin2 types.ZetoCoin
	err = json.Unmarshal([]byte(res.AssembledTransaction.OutputStates[1].StateDataJson), &coin2)
	assert.NoError(t, err)
	assert.Equal(t, "0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025", coin2.Owner.String())
	assert.Equal(t, "0x06", coin2.Amount.String())

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x04\"}",
				},
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x04\"}",
				},
				{
					DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0a\"}",
				},
			},
		}, nil
	}

	// Successful assembly with 3 input states
	res, err = h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.AssembledTransaction.InputStates, 3)
	assert.Len(t, res.AssembledTransaction.OutputStates, 2) // one for the receiver Alice, one for self as change

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	(*tx.DomainConfig.Circuits)["transfer"] = &zetosignerapi.Circuit{Name: "anon_nullifier_transfer", Type: "transfer"}
	called := 0
	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		var dataJson string
		if called == 0 {
			dataJson = "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x0a\"}"
		} else if called == 1 {
			dataJson = "{\"rootIndex\": \"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}"
		} else {
			dataJson = "{\"index\":\"0x3801702a0a958207c485bbf0137ff64327bdf16ad9a5acdb4d5ab1469b87e326\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x89ea7fc1e5e9722566083823f288a45d6dc7ef30b68094f006530dfe9f5cf90f\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}"
		}
		called++
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: dataJson,
				},
			},
		}, nil

	}
	h.callbacks = testCallbacks
	res, err = h.Assemble(ctx, tx, req)
	assert.NoError(t, err)
	assert.Len(t, res.AssembledTransaction.OutputStates, 2)
}

func TestTransferEndorse(t *testing.T) {
	h := transferHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &pb.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestTransferPrepare(t *testing.T) {
	h := transferHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	txSpec := &pb.TransactionSpecification{
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
			TokenName: constants.TOKEN_ANON_ENC,
			Circuits: &zetosignerapi.Circuits{
				"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
			},
		},
	}
	req := &pb.PrepareTransactionRequest{
		InputStates: []*pb.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
		OutputStates: []*pb.EndorsableState{
			{
				SchemaId:      "coin",
				StateDataJson: "bad json",
			},
		},
		Transaction: txSpec,
	}
	ctx := context.Background()
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "PD210043: Did not find 'sender' attestation")

	at := zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK
	req.AttestationResult = []*pb.AttestationResult{
		{
			Name:            "sender",
			AttestationType: pb.AttestationType_ENDORSE,
			PayloadType:     &at,
			Payload:         []byte("bad payload"),
		},
	}
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210044: Failed to unmarshal proving response")

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
	assert.EqualError(t, err, "PD210045: Failed to parse input states. invalid character 'b' looking for beginning of value")

	req.InputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "PD210047: Failed to parse output states. invalid character 'b' looking for beginning of value")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210049: Failed to encode transaction data. PD210028: Failed to parse transaction id. PD020007: Invalid hex:")

	txSpec.TransactionId = "0x1234567890123456789012345678901234567890123456789012345678901234"

	res, err := h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"data\":\"0x00010000123456789012345678901234567890123456789012345678901234567890123400000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000\",\"ecdhPublicKey\":[\"\"],\"encryptedValues\":[\"0x1234567890\",\"0x1234567890\"],\"encryptionNonce\":\"0x1234567890\",\"inputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"outputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]}}", res.Transaction.ParamsJson)

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	(*tx.DomainConfig.Circuits)["transfer"] = &zetosignerapi.Circuit{Name: "anon_nullifier_transfer", Type: "transfer", UsesNullifiers: true}
	proofReq.PublicInputs["nullifiers"] = "0x1234567890,0x1234567890"
	proofReq.PublicInputs["root"] = "0x1234567890"
	payload, err = proto.Marshal(&proofReq)
	assert.NoError(t, err)
	req.AttestationResult[0].Payload = payload
	res, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"data\":\"0x00010000123456789012345678901234567890123456789012345678901234567890123400000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000\",\"nullifiers\":[\"0x1234567890\",\"0x1234567890\"],\"outputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]},\"root\":\"0x1234567890\"}", res.Transaction.ParamsJson)
}

func TestGenerateMerkleProofs(t *testing.T) {
	testCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*pb.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h := transferHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &pb.StateSchema{
					Id: "coin",
				},
				MerkleTreeRootSchema: &pb.StateSchema{
					Id: "merkle_tree_root",
				},
				MerkleTreeNodeSchema: &pb.StateSchema{
					Id: "merkle_tree_node",
				},
			},
		},
		callbacks: testCallbacks,
	}
	addr, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	assert.NoError(t, err)
	inputCoins := []*types.ZetoCoin{
		{
			Salt:   pldtypes.MustParseHexUint256("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			Owner:  pldtypes.MustParseHexBytes("0x1234"),
			Amount: pldtypes.MustParseHexUint256("0x0f"),
		},
	}
	ctx := context.Background()
	queryContext := "queryContext"
	smtName := smt.MerkleTreeName("Zeto_Anon", addr)
	_, err = common.NewMerkleTreeSpec(ctx, smtName, common.StatesTree, h.callbacks, h.stateSchemas.MerkleTreeRootSchema.Id, h.stateSchemas.MerkleTreeNodeSchema.Id, queryContext)
	assert.EqualError(t, err, "PD210019: Failed to create Merkle tree for smt_Zeto_Anon_0x1234567890123456789012345678901234567890: PD210065: Failed to find available states for the merkle tree. test error")

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		return &pb.FindAvailableStatesResponse{
			States: []*pb.StoredState{
				{
					DataJson: "{\"rootIndex\":\"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}",
				},
			},
		}, nil
	}
	mt, err := common.NewMerkleTreeSpec(ctx, smtName, common.StatesTree, h.callbacks, h.stateSchemas.MerkleTreeRootSchema.Id, h.stateSchemas.MerkleTreeNodeSchema.Id, queryContext)
	assert.NoError(t, err)
	_, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	assert.EqualError(t, err, "PD210037: Failed load owner public key. PD210072: Invalid compressed public key length: 2")

	inputCoins[0].Owner = pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025")
	_, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	// _, _, err = generateMerkleProofs(ctx, h.callbacks, h.stateSchemas.MerkleTreeRootSchema, h.stateSchemas.MerkleTreeNodeSchema, "Zeto_Anon", queryContext, addr, inputCoins, false)
	assert.EqualError(t, err, "PD210054: Failed to create new leaf node. inputs values not inside Finite Field")

	inputCoins[0].Salt = pldtypes.MustParseHexUint256("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec")
	calls := 0
	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		defer func() { calls++ }()
		if calls == 0 {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"rootIndex\":\"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}",
					},
				},
			}, nil
		} else {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{},
			}, nil
		}
	}
	mt, err = common.NewMerkleTreeSpec(ctx, smtName, common.StatesTree, h.callbacks, h.stateSchemas.MerkleTreeRootSchema.Id, h.stateSchemas.MerkleTreeNodeSchema.Id, queryContext)
	assert.NoError(t, err)
	_, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	assert.EqualError(t, err, "PD210055: Failed to query the smt DB for leaf node (ref=789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a). key not found")

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		defer func() { calls++ }()
		if calls == 0 {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"rootIndex\":\"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}",
					},
				},
			}, nil
		} else {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"index\":\"0x3801702a0a958207c485bbf0137ff64327bdf16ad9a5acdb4d5ab1469b87e326\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x89ea7fc1e5e9722566083823f288a45d6dc7ef30b68094f006530dfe9f5cf90f\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}",
					},
				},
			}, nil
		}
	}
	mt, err = common.NewMerkleTreeSpec(ctx, smtName, common.StatesTree, h.callbacks, h.stateSchemas.MerkleTreeRootSchema.Id, h.stateSchemas.MerkleTreeNodeSchema.Id, queryContext)
	assert.NoError(t, err)
	_, err = makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	assert.EqualError(t, err, "PD210057: Coin (ref=789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a) found in the merkle tree but the persisted hash 26e3879b46b15a4ddbaca5d96af1bd2743f67f13f0bb85c40782950a2a700138 (index=3801702a0a958207c485bbf0137ff64327bdf16ad9a5acdb4d5ab1469b87e326) did not match the expected hash 0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f (index=5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30)")

	testCallbacks.MockFindAvailableStates = func() (*pb.FindAvailableStatesResponse, error) {
		defer func() { calls++ }()
		if calls == 0 {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"rootIndex\":\"0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a\"}",
					},
				},
			}, nil
		} else {
			return &pb.FindAvailableStatesResponse{
				States: []*pb.StoredState{
					{
						DataJson: "{\"index\":\"0x5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}",
					},
				},
			}, nil
		}
	}
	mt, err = common.NewMerkleTreeSpec(ctx, smtName, common.StatesTree, h.callbacks, h.stateSchemas.MerkleTreeRootSchema.Id, h.stateSchemas.MerkleTreeNodeSchema.Id, queryContext)
	assert.NoError(t, err)
	indexes, err := makeLeafIndexesFromCoins(ctx, inputCoins, mt.Tree)
	assert.NoError(t, err)
	_, err = generateMerkleProofs(ctx, mt, indexes, 2)
	assert.NoError(t, err)
}

func TestFungibleTransferEndorse(t *testing.T) {
	h := transferHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &pb.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

var _ plugintk.DomainCallbacks = &domain.MockDomainCallbacks{
	MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
		return nil, errors.New("test error")
	},
}

type testDomainCallbacks struct {
	returnFunc func() (*pb.FindAvailableStatesResponse, error)
}

func (dc *testDomainCallbacks) FindAvailableStates(ctx context.Context, req *pb.FindAvailableStatesRequest) (*pb.FindAvailableStatesResponse, error) {
	return dc.returnFunc()
}

func (dc *testDomainCallbacks) EncodeData(ctx context.Context, req *pb.EncodeDataRequest) (*pb.EncodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) RecoverSigner(ctx context.Context, req *pb.RecoverSignerRequest) (*pb.RecoverSignerResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) DecodeData(context.Context, *pb.DecodeDataRequest) (*pb.DecodeDataResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) GetStatesByID(ctx context.Context, req *pb.GetStatesByIDRequest) (*pb.GetStatesByIDResponse, error) {
	return nil, nil
}
func (dc *testDomainCallbacks) LocalNodeName(context.Context, *pb.LocalNodeNameRequest) (*pb.LocalNodeNameResponse, error) {
	return nil, nil
}

func (dc *testDomainCallbacks) SendTransaction(ctx context.Context, tx *pb.SendTransactionRequest) (*pb.SendTransactionResponse, error) {
	return nil, nil
}
