package nonfungible

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/common"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAlgoZetoSnarkBJJ_mintHandler(t *testing.T) {
	h := &mintHandler{
		baseHandler: baseHandler{
			name: "mint",
		},
	}
	assert.Equal(t, "domain:mint:snark:babyjubjub", h.getAlgoZetoSnarkBJJ())
}

func TestMintHandler_Prepare(t *testing.T) {
	ctx := context.Background()
	defer defaultHelpers()

	tests := []struct {
		name         string
		encodeFunc   func(context.Context, *pb.TransactionSpecification, []*prototk.EndorsableState) (pldtypes.HexBytes, error)
		outputStates []*pb.EndorsableState
		expectErr    bool
		errContains  string
	}{
		{
			name: "success",
			outputStates: []*pb.EndorsableState{
				{
					StateDataJson: `{"owner":"0x638e6824da3eb00687eefdeefb17dc646ba9f00fae6020f1b6d640487b07fdac","salt":"3949625438621963838695705020414673764457825239260453211443343787973144679466","tokenID":"12889917038846740459390665944266706251653790785225711651704434901540173766845","uri":"https://example.com/token/name2"}`,
				},
			},
			encodeFunc: common.EncodeTransactionData,
			expectErr:  false,
		},
		{
			name: "invalid state data",
			outputStates: []*pb.EndorsableState{
				{
					StateDataJson: `invalid`,
				},
			},
			encodeFunc:  common.EncodeTransactionData,
			expectErr:   true,
			errContains: "invalid character",
		},
		{
			name: "invalid hash",
			outputStates: []*pb.EndorsableState{
				{
					StateDataJson: `{"owner":"0x638e6824da3eb00687eefdeefb17dc646ba9f00fae6020f1b6d640487b07fdac","salt":"3949625438621963838695705020414673764457825239260453211443343787973144679466","tokenID":"12889917038846740459390665944266706251653790785225711651704434901540173766845","uri":""}`,
				},
			},
			encodeFunc:  common.EncodeTransactionData,
			expectErr:   true,
			errContains: "PD210112",
		},
		{
			name: "failure: encode tx data error",
			outputStates: []*pb.EndorsableState{
				{
					StateDataJson: `{"owner":"0x638e6824da3eb00687eefdeefb17dc646ba9f00fae6020f1b6d640487b07fdac","salt":"3949625438621963838695705020414673764457825239260453211443343787973144679466","tokenID":"12889917038846740459390665944266706251653790785225711651704434901540173766845","uri":"https://example.com/token/name2"}`,
				},
			},
			encodeFunc: func(context.Context, *pb.TransactionSpecification, []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
				return nil, assert.AnError
			},
			expectErr:   true,
			errContains: "PD210049",
		},
	}

	// Create a dummy PrepareTransactionRequest.
	req := &pb.PrepareTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			TransactionId: "0x87229d205a0f48bcf0da37542fc140a9bdfc3b4a55c0beffcb62efe25a770a7f",
			From:          "minterAddress",
		},
		OutputStates: []*pb.EndorsableState{
			{
				StateDataJson: `{"owner":"0x638e6824da3eb00687eefdeefb17dc646ba9f00fae6020f1b6d640487b07fdac","salt":"3949625438621963838695705020414673764457825239260453211443343787973144679466","tokenID":"12889917038846740459390665944266706251653790785225711651704434901540173766845","uri":"https://example.com/token/name2"}`,
			},
		},
	}

	// Create a dummy parsed transaction.
	tx := &types.ParsedTransaction{
		Transaction:  req.Transaction,
		DomainConfig: &types.DomainInstanceConfig{},
	}

	// Instantiate the mintHandler.
	handler := &mintHandler{}

	// Override the global encodeTransactionDataFunc.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encodeTransactionDataFunc = tc.encodeFunc
			req.OutputStates = tc.outputStates

			resp, err := handler.Prepare(ctx, tx, req)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				require.NotNil(t, resp, "response should not be nil")
				// Verify that the function ABI JSON is as expected.
				// In our dummy, mintABI marshals to JSON with "name":"mint".
				assert.Contains(t, resp.Transaction.FunctionAbiJson, `"name":"mint"`, "FunctionAbiJson should contain mint ABI")
				// Unmarshal ParamsJson.
				var paramsMap map[string]interface{}
				err = json.Unmarshal([]byte(resp.Transaction.ParamsJson), &paramsMap)
				require.NoError(t, err, "failed to unmarshal ParamsJson")
				// Expect "utxos" key with at least one value.
				utxos, ok := paramsMap["utxos"].([]interface{})
				require.True(t, ok, "utxos should be an array")
				require.Len(t, utxos, len(tc.outputStates), "unexpected number of utxos")
				assert.Equal(t,
					"0x0001000087229d205a0f48bcf0da37542fc140a9bdfc3b4a55c0beffcb62efe25a770a7f00000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000",
					paramsMap["data"], "data mismatch")
				assert.Equal(t, "minterAddress", *resp.Transaction.RequiredSigner, "RequiredSigner mismatch")
			}
		})
	}
}

func TestMintHandler_Assemble(t *testing.T) {
	ctx := context.Background()

	// Save original prepareOutputsForTransfer and restore after tests.

	tests := []struct {
		name       string
		params     []*types.NonFungibleTransferParamEntry
		expectErr  bool
		errMessage string
	}{
		{
			name: "success",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:  "recipient",
					URI: "https://example.com",
				},
			},
			expectErr: false,
		},
		{
			name: "failure",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:  "", // Empty recipient
					URI: "https://example.com",
				},
			},
			expectErr:  true,
			errMessage: "PD210036",
		},
	}

	defer defaultHelpers()
	findVerifierFunc = dummyFindVerifier

	// Create a dummy parsed transaction.
	tx := &types.ParsedTransaction{
		DomainConfig: &types.DomainInstanceConfig{
			TokenName: constants.TOKEN_NF_ANON,
			Circuits:  &zetosignerapi.Circuits{},
		},
	}

	// Create a dummy AssembleTransactionRequest.
	req := &pb.AssembleTransactionRequest{
		ResolvedVerifiers: []*pb.ResolvedVerifier{}, // For this test, they are not used by dummyPrepareOutputsForTransfer.
	}

	// Instantiate a mintHandler with a valid state schema.
	handler := &mintHandler{
		baseHandler: baseHandler{
			"mint",
		},
		stateSchema: &pb.StateSchema{
			Id: "schema1",
		},
	}

	// Run tests.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx.Params = tc.params
			resp, err := handler.Assemble(ctx, tx, req)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errMessage, "error message should contain %q", tc.errMessage)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				require.NotNil(t, resp, "response should not be nil")

				require.NotNil(t, resp.AssembledTransaction, "AssembledTransaction should not be nil")
				assert.Len(t, resp.AssembledTransaction.OutputStates, 1, "expected one output state")
				assert.NotEmpty(t, resp.AssembledTransaction.OutputStates[0].GetId(), "output state Id empty")
				assert.Equal(t, "schema1", resp.AssembledTransaction.OutputStates[0].SchemaId, "schemaId mismatch")
				assert.Equal(t, pb.AssembleTransactionResponse_OK, resp.AssemblyResult, "AssemblyResult mismatch")
				assert.Empty(t, resp.AttestationPlan, "expected empty attestation plan")
			}
		})
	}
}

func TestInit_mint(t *testing.T) {
	ctx := context.Background()

	// Create a dummy parsed transaction.
	tx := &types.ParsedTransaction{
		Transaction: &pb.TransactionSpecification{
			From: "sender",
		},
	}

	// Create a dummy InitTransactionRequest.
	req := &pb.InitTransactionRequest{}

	// Override getAlgoZetoSnarkBJJ to return a fixed value.
	dummyAlgo := "domain::snark:babyjubjub"
	handler := &mintHandler{}

	tests := []struct {
		name              string
		params            []*types.NonFungibleTransferParamEntry
		expectedVerifiers int // expected total number of required verifiers
	}{
		{
			name:              "no transfer params",
			params:            []*types.NonFungibleTransferParamEntry{},
			expectedVerifiers: 0, // Only the sender is added.
		},
		{
			name: "one transfer param",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:  "controller",
					URI: "https://example.com",
				},
			},
			expectedVerifiers: 1, // Sender + one recipient.
		},
		{
			name: "multiple transfer params",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:  "controller",
					URI: "https://example.com",
				},
				{
					To:  "controller",
					URI: "https://example.org",
				},
			},
			expectedVerifiers: 2, // Sender + two recipients.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx.Params = tc.params

			resp, err := handler.Init(ctx, tx, req)
			require.NoError(t, err, "unexpected error in test case %q", tc.name)
			require.NotNil(t, resp, "response should not be nil")

			// Verify the number of required verifiers.
			rv := resp.RequiredVerifiers
			require.Len(t, rv, tc.expectedVerifiers, "unexpected number of required verifiers")
			if tc.expectedVerifiers > 0 {
				// The first required verifier should come from the sender.
				assert.Equal(t, "controller", rv[0].Lookup, "sender verifier lookup mismatch")
				assert.Equal(t, dummyAlgo, rv[0].Algorithm, "sender algorithm mismatch")
				assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, rv[0].VerifierType, "sender verifier type mismatch")
			}
		})
	}
}

func TestValidateMintParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		params    []*types.NonFungibleTransferParamEntry
		expectErr bool
	}{
		{
			name:      "no parameters provided",
			params:    nil,
			expectErr: true,
		},
		{
			name:      "empty parameters slice",
			params:    []*types.NonFungibleTransferParamEntry{},
			expectErr: true,
		},
		{
			name: "empty 'to' field",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(0)), // zero is acceptable
				},
			},
			expectErr: true,
		},
		{
			name: "non-zero tokenID",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(123)), // non-zero tokenID is not allowed for mint
				},
			},
			expectErr: true,
		},
		{
			name: "empty URI",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient",
					URI:     "",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(0)), // zero tokenID is acceptable
				},
			},
			expectErr: true,
		},
		{
			name: "valid mint parameters",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(0)),
				},
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMintParams(ctx, tc.params)
			if tc.expectErr {
				require.Error(t, err, "expected error for test case %q", tc.name)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
			}
		})
	}
}
func TestNewMintHandler(t *testing.T) {
	name := "testHandler"
	stateSchema := &pb.StateSchema{
		Id: "testSchema",
	}

	handler := NewMintHandler(name, stateSchema)

	assert.NotNil(t, handler, "handler should not be nil")
	assert.Equal(t, name, handler.name, "handler name mismatch")
	assert.Equal(t, stateSchema, handler.stateSchema, "handler stateSchema mismatch")
}
func TestMintHandler_ValidateParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		params     string
		expectErr  bool
		errMessage string
	}{
		{
			name:      "valid params",
			params:    `{"mints":[{"to":"recipient","uri":"https://example.com","tokenID":"0"}]}`,
			expectErr: false,
		},
		{
			name:       "invalid JSON",
			params:     `invalid`,
			expectErr:  true,
			errMessage: "invalid character",
		},
		{
			name:       "empty mints",
			params:     `{"mints":[]}`,
			expectErr:  true,
			errMessage: "PD210024:",
		},
		{
			name:       "missing 'to' field",
			params:     `{"mints":[{"to":"","uri":"https://example.com","tokenID":"0"}]}`,
			expectErr:  true,
			errMessage: "PD210025",
		},
		{
			name:       "non-zero tokenID",
			params:     `{"mints":[{"to":"recipient","uri":"https://example.com","tokenID":"123"}]}`,
			expectErr:  true,
			errMessage: "PD210114",
		},
		{
			name:       "empty URI",
			params:     `{"mints":[{"to":"recipient","uri":"","tokenID":"0"}]}`,
			expectErr:  true,
			errMessage: "PD210115",
		},
	}

	handler := &mintHandler{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.ValidateParams(ctx, &types.DomainInstanceConfig{}, tc.params)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errMessage, "error message should contain %q", tc.errMessage)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
			}
		})
	}
}
func TestMintHandler_Endorse(t *testing.T) {
	ctx := context.Background()

	// Create a dummy parsed transaction.
	tx := &types.ParsedTransaction{
		Transaction: &pb.TransactionSpecification{
			From: "sender",
		},
	}

	// Create a dummy EndorseTransactionRequest.
	req := &pb.EndorseTransactionRequest{}

	// Instantiate the mintHandler.
	handler := &mintHandler{}

	// Call the Endorse method.
	resp, err := handler.Endorse(ctx, tx, req)

	// Validate the response.
	require.NoError(t, err, "unexpected error in Endorse")
	assert.Nil(t, resp, "response should be nil")
}
