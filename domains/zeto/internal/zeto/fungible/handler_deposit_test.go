package fungible

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/internal/zeto/common"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDepositValidateParams(t *testing.T) {
	h := &depositHandler{}
	ctx := context.Background()
	config := &types.DomainInstanceConfig{}
	v, err := h.ValidateParams(ctx, config, "{\"amount\":100}")
	require.NoError(t, err)
	require.Equal(t, "0x64", v.(*pldtypes.HexUint256).String())

	_, err = h.ValidateParams(ctx, config, "bad json")
	require.ErrorContains(t, err, "PD210105: Failed to decode the deposit call.")

	_, err = h.ValidateParams(ctx, config, "{\"amount\":-100}")
	require.ErrorContains(t, err, "PD210027: Parameter 'amount' must be in the range (0, 2^100) (index=0)")
}

func TestDepositInit(t *testing.T) {
	h := depositHandler{
		baseHandler: baseHandler{
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

func TestDepositAssemble(t *testing.T) {
	h := depositHandler{
		baseHandler: baseHandler{
			name: "test1",
			stateSchemas: &common.StateSchemas{
				CoinSchema: &prototk.StateSchema{
					Id: "coin",
				},
			},
		},
	}
	ctx := context.Background()
	txSpec := &prototk.TransactionSpecification{
		From: "Bob",
		ContractInfo: &prototk.ContractInfo{
			ContractAddress: "0x1234567890123456789012345678901234567890",
		},
	}
	tx := &types.ParsedTransaction{
		Params:      pldtypes.MustParseHexUint256("100"),
		Transaction: txSpec,
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: "tokenContract1",
			Circuits: &zetosignerapi.Circuits{
				"deposit": &zetosignerapi.Circuit{Name: "circuit-deposit"},
			},
		},
	}
	req := &prototk.AssembleTransactionRequest{
		ResolvedVerifiers: []*prototk.ResolvedVerifier{
			{
				Lookup:       "Alice",
				Verifier:     "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922",
				Algorithm:    h.getAlgoZetoSnarkBJJ(),
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
		Algorithm:    h.getAlgoZetoSnarkBJJ(),
		VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
	})
	_, err = h.Assemble(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210040: Failed to prepare transaction outputs. PD210037: Failed load owner public key")

	req.ResolvedVerifiers[1].Verifier = "0x19d2ee6b9770a4f8d7c3b7906bc7595684509166fa42d718d1d880b62bcb7922"
	res, err := h.Assemble(ctx, tx, req)
	require.NoError(t, err)
	assert.Equal(t, "100", *res.AssembledTransaction.DomainData)
}

func TestDepositEndorse(t *testing.T) {
	h := depositHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &prototk.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestDepositPrepare(t *testing.T) {
	h := depositHandler{
		baseHandler: baseHandler{
			name: "test1",
		},
	}
	txSpec := &prototk.TransactionSpecification{
		TransactionId: "bad hex",
		From:          "Bob",
	}
	tx := &types.ParsedTransaction{
		Params:      pldtypes.MustParseHexUint256("100"),
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
	assert.ErrorContains(t, err, "PD210047: Failed to parse output states.")

	req.OutputStates[0].StateDataJson = "{\"salt\":\"0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec\",\"owner\":\"0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025\",\"amount\":\"0x0f\",\"hash\":\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\"}"
	_, err = h.Prepare(ctx, tx, req)
	assert.ErrorContains(t, err, "PD210049: Failed to encode transaction data. PD210028: Failed to parse transaction id. PD020007: Invalid hex:")

	txSpec.TransactionId = "0x1234567890123456789012345678901234567890123456789012345678901234"
	res, err := h.Prepare(ctx, tx, req)
	assert.NoError(t, err)
	assert.Equal(t, "{\"amount\":\"100\",\"data\":\"0x00010000123456789012345678901234567890123456789012345678901234567890123400000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000\",\"outputs\":[\"0x303eb034d22aacc5dff09647928d757017a35e64e696d48609a250a6505e5d5f\",\"0\"],\"proof\":{\"pA\":[\"0x1234567890\",\"0x1234567890\"],\"pB\":[[\"0x1234567890\",\"0x1234567890\"],[\"0x1234567890\",\"0x1234567890\"]],\"pC\":[\"0x1234567890\",\"0x1234567890\"]}}", res.Transaction.ParamsJson)
}
func TestNewDepositHandler(t *testing.T) {
	name := "testHandler"
	coinSchema := &prototk.StateSchema{
		Id: "coinSchema",
	}

	handler := NewDepositHandler(name, coinSchema)

	assert.NotNil(t, handler)
	assert.Equal(t, name, handler.name)
	assert.Equal(t, coinSchema, handler.stateSchemas.CoinSchema)
}
