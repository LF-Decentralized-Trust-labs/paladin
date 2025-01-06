package zeto

import (
	"context"
	"errors"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestWithdrawValidateParams(t *testing.T) {
	h := &withdrawHandler{}
	ctx := context.Background()
	config := &types.DomainInstanceConfig{}
	v, err := h.ValidateParams(ctx, config, "{\"amount\":100}")
	require.NoError(t, err)
	require.Equal(t, "0x64", v.(*tktypes.HexUint256).String())

	_, err = h.ValidateParams(ctx, config, "bad json")
	require.ErrorContains(t, err, "PD210106: Failed to decode the withdraw call.")

	_, err = h.ValidateParams(ctx, config, "{\"amount\":-100}")
	require.ErrorContains(t, err, "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)")
}

func TestWithdrawInit(t *testing.T) {
	h := withdrawHandler{
		zeto: &Zeto{
			name: "test1",
		},
	}
	ctx := context.Background()
	tx := &types.ParsedTransaction{
		Transaction: &prototk.TransactionSpecification{
			From: "Alice",
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

func TestWithdrawAssemble(t *testing.T) {
	h := withdrawHandler{
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
		From: "Bob",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "bad address",
		},
	}
	tx := &types.ParsedTransaction{
		Params:      tktypes.MustParseHexUint256("100"),
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
				Verifier:     "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922",
				Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
				VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
			},
		},
		Transaction: txSpec,
	}
	_, err := h.Assemble(ctx, tx, req)
	assert.EqualError(t, err, "PD210036: Failed to resolve verifier: Bob")

	req.ResolvedVerifiers = append(req.ResolvedVerifiers, &prototk.ResolvedVerifier{
		Lookup:       "Bob",
		Verifier:     "0x1234567890123456789012345678901234567890",
		Algorithm:    h.zeto.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
	})
	testCallbacks := &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return nil, errors.New("test error")
		},
	}
	h.zeto.Callbacks = testCallbacks
	_, err = h.Assemble(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210039: Failed to prepare transaction inputs. PD210032: Failed to query the state store for available coins. test error")

	h.zeto.Callbacks = &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: "{\"salt\":\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
					},
				},
			}, nil
		},
	}
	req.ResolvedVerifiers[1].Verifier = "bad key"
	_, err = h.Assemble(ctx, tx, req)
	require.ErrorContains(t, err, "PD210040: Failed to prepare transaction outputs. PD210037: Failed load owner public key.")

	req.ResolvedVerifiers[1].Verifier = "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922"
	_, err = h.Assemble(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210017: Failed to decode contract address.")

	txSpec.ContractInfo.ContractAddress = "0x1234567890123456789012345678901234567890"
	_, err = h.Assemble(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210042: Failed to format proving request.")

	h.zeto.Callbacks = &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922\",\"amount\":\"0x0f\"}",
					},
				},
			}, nil
		},
	}
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, "100", *res.AssembledTransaction.DomainData)

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	tx.DomainConfig.CircuitId = constants.CIRCUIT_ANON_NULLIFIER
	called := 0
	h.zeto.Callbacks = &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			var dataJson string
			if called == 0 {
				dataJson = "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x65\"}"
			} else if called == 1 {
				dataJson = "{\"rootIndex\": \"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}"
			} else {
				dataJson = "{\"index\":\"bad index\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x89ea7fc1e5e9722566083823f288a45d6dc7ef30b68094f006530dfe9f5cf90f\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}"
			}
			called++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: dataJson,
					},
				},
			}, nil
		},
	}
	res, err = h.Assemble(ctx, tx, req)
	require.ErrorContains(t, err, "PD210042: Failed to format proving request. PD210052: Failed to generate merkle proofs.")

	called = 0
	h.zeto.Callbacks = &testDomainCallbacks{
		returnFunc: func() (*prototk.FindAvailableStatesResponse, error) {
			var dataJson string
			if called == 0 {
				dataJson = "{\"salt\":\"0x13de02d64a5736a56b2d35d2a83dd60397ba70aae6f8347629f0960d4fee5d58\",\"owner\":\"0xc1d218cf8993f940e75eabd3fee23dadc4e89cd1de479f03a61e91727959281b\",\"amount\":\"0x65\"}"
			} else if called == 1 {
				dataJson = "{\"rootIndex\": \"0x28025a624a1e83687e84451d04190f081d79d470f9d50a7059508476be02d401\"}"
			} else {
				dataJson = "{\"index\":\"0xb6025832e11338c178467dda6472d74c15aac53d0781f51681df082840e2ca25\",\"leftChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"refKey\":\"0x89ea7fc1e5e9722566083823f288a45d6dc7ef30b68094f006530dfe9f5cf90f\",\"rightChild\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"type\":\"0x02\"}"
			}
			called++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{
					{
						DataJson: dataJson,
					},
				},
			}, nil
		},
	}
	res, err = h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, "100", *res.AssembledTransaction.DomainData)
}

func TestWithdrawEndorse(t *testing.T) {
	h := withdrawHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestWithdrawPrepare(t *testing.T) {
	z := &Zeto{
		name: "test1",
	}
	h := withdrawHandler{
		zeto: z,
	}
	txSpec := &prototk.TransactionSpecification{
		TransactionId: "bad hex",
		From:          "Bob",
	}
	tx := &types.ParsedTransaction{
		Params:      tktypes.MustParseHexUint256("100"),
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: constants.TOKEN_ANON_ENC,
		},
	}
	amountStr := "100"
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
		DomainData:  &amountStr,
	}
	ctx := context.Background()
	_, err := h.Prepare(ctx, tx, req)
	assert.EqualError(t, err, "PD210043: Did not find 'sender' attestation")

	at := zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK
	req.AttestationResult = []*prototk.AttestationResult{
		{
			Name:            "sender",
			AttestationType: prototk.AttestationType_ENDORSE,
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
	assert.ErrorContains(t, err, "PD210045: Failed to parse input states.")

	req.InputStates[0].StateDataJson = "{\"salt\":\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210046: Failed to create Poseidon hash for an input coin.")

	req.InputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210047: Failed to parse output states.")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210048: Failed to create Poseidon hash for an output coin.")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210049: Failed to encode transaction data. PD210028: Failed to parse transaction id.")

	txSpec.TransactionId = "0x1234567890123456789012345678901234567890123456789012345678901234"
	z.config = &types.DomainFactoryConfig{
		DomainContracts: types.DomainConfigContracts{
			Implementations: []*types.DomainContract{},
		},
	}
	z.config.DomainContracts.Implementations = []*types.DomainContract{
		{
			Name: constants.TOKEN_ANON_ENC,
		},
	}
	res, err := h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"amount\":\"100\",\"data\":\"0x000100001234567890123456789012345678901234567890123456789012345678901234\",\"inputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"output\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]}}", res.Transaction.ParamsJson)

	tx.DomainConfig.TokenName = constants.TOKEN_ANON_NULLIFIER
	tx.DomainConfig.CircuitId = constants.CIRCUIT_ANON_NULLIFIER
	proofReq.PublicInputs = map[string]string{
		"nullifiers": "0x1234567890,0x1234567890",
		"root":       "0x1234567890",
	}
	payload, err = proto.Marshal(&proofReq)
	require.NoError(t, err)
	req.AttestationResult[0].Payload = payload
	res, err = h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"amount\":\"100\",\"data\":\"0x000100001234567890123456789012345678901234567890123456789012345678901234\",\"nullifiers\":[\"0x1234567890\",\"0x1234567890\"],\"output\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]},\"root\":\"0x1234567890\"}", res.Transaction.ParamsJson)
}
