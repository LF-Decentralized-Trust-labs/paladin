package nonfungible

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockPubKey = func() *babyjub.PublicKey {
	x, _ := new(big.Int).SetString("20324599009286821207881465153085764126595806822268060878040393292028608397602", 0)
	y, _ := new(big.Int).SetString("6667720951847887467326343771312468792334056297732558024347070059459187374673", 0)
	return &babyjub.PublicKey{
		X: x,
		Y: y,
	}
}

var _ plugintk.DomainCallbacks = &testDomainCallbacks{}

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

func (dc *testDomainCallbacks) DecodeData(ctx context.Context, req *pb.DecodeDataRequest) (*pb.DecodeDataResponse, error) {
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

func TestProcessTokens(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name                string
		tokens              []*types.ZetoNFToken
		expectErr           bool
		expectedCommitments []string
		expectedSalts       []string
		expectedURIs        []string
		expectedTokenIDs    []string
		expectedOwners      []string
	}{
		{
			name:      "empty tokens",
			tokens:    []*types.ZetoNFToken{},
			expectErr: true,
		},
		{
			name: "valid token",
			tokens: []*types.ZetoNFToken{
				types.NewZetoNFToken(
					(*pldtypes.HexUint256)(big.NewInt(456)),
					"https://example.com",
					mockPubKey(),
					big.NewInt(123),
				)},
			expectErr:           false,
			expectedCommitments: []string{"11e84f5f703728d1f231655c59597678524e3a14ce684d07a0b653bd51ccd650"},
			expectedSalts:       []string{"7b"},
			expectedURIs:        []string{"https://example.com"},
			expectedTokenIDs:    []string{"0x01c8"},
			expectedOwners:      []string{"51fa904bb6142e89f85aebb2a933a879e2efd5b682021deec4f717a8dbcbbd8e"},
		},
		{
			name: "hash error",
			tokens: []*types.ZetoNFToken{
				types.NewZetoNFToken(
					(*pldtypes.HexUint256)(big.NewInt(456)),
					"",
					mockPubKey(),
					big.NewInt(123),
				)},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			commitments, salts, uris, tokenIDs, owners, err := processTokens(ctx, tc.tokens)
			if tc.expectErr {
				require.Error(t, err, "expected an error for test case %q", tc.name)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
				assert.Equal(t, tc.expectedCommitments, commitments, "commitments mismatch")
				assert.Equal(t, tc.expectedSalts, salts, "salts mismatch")
				assert.Equal(t, tc.expectedURIs, uris, "URIs mismatch")
				assert.Equal(t, tc.expectedTokenIDs, tokenIDs, "tokenIDs mismatch")
				assert.Equal(t, tc.expectedOwners, owners, "owners mismatch")
			}
		})
	}
}

// dummyRand256 returns a fixed 256‑bit number (e.g. 999).
func dummyRand256() (*big.Int, error) {
	return big.NewInt(999), nil
}

// dummyNewSalt returns a fixed salt value (e.g. 123).
func dummyNewSalt() *big.Int {
	return big.NewInt(123)
}
func TestPrepareOutputsForTransfer(t *testing.T) {
	ctx := context.Background()

	// Override external dependencies
	Rand256 = dummyRand256
	NewSalt = dummyNewSalt

	tests := []struct {
		name               string
		useNullifiers      bool
		params             []*types.NonFungibleTransferParamEntry
		resolvedVerifiers  []*pb.ResolvedVerifier
		stateSchema        *pb.StateSchema
		algoName           string
		expectErr          bool
		errCode            string
		expectedTokenCount int
		expectedStateCount int
	}{
		{
			name:          "successful transfer with provided tokenID",
			useNullifiers: true,
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient1",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(456)), // provided tokenID; non-zero
				},
			},
			resolvedVerifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "recipient1",
					Algorithm:    getAlgoZetoSnarkBJJ("test"),
					VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
					Verifier:     zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()),
				},
			},
			stateSchema: &pb.StateSchema{
				Id: "schema1",
			},
			algoName:           "test",
			expectErr:          false,
			expectedTokenCount: 1,
			expectedStateCount: 1,
		},
		{
			name:          "failure: no resolved verifier found",
			useNullifiers: false,
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "nonexistent",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
				},
			},
			// Provide a resolved verifier list that does not include the lookup "nonexistent".
			resolvedVerifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "someoneElse",
					Algorithm:    getAlgoZetoSnarkBJJ("test"),
					VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
					Verifier:     zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()),
				},
			},
			stateSchema: &pb.StateSchema{Id: "schema1"},
			algoName:    "test",
			expectErr:   true,
			errCode:     "PD210036",
		},
		{
			name:          "failure: LoadBabyJubKey error",
			useNullifiers: false,
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient1",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
				},
			},
			resolvedVerifiers: []*pb.ResolvedVerifier{
				{
					Lookup:       "recipient1",
					Algorithm:    getAlgoZetoSnarkBJJ("test"),
					VerifierType: zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
					Verifier:     "bad",
				},
			},
			stateSchema: &pb.StateSchema{Id: "schema1"},
			algoName:    "test",
			expectErr:   true,
			errCode:     "PD210037",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens, newStates, err := prepareOutputsForTransfer(ctx, tc.useNullifiers, tc.params, tc.resolvedVerifiers, tc.stateSchema, tc.algoName)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				if tc.errCode != "" {
					assert.Contains(t, err.Error(), tc.errCode)
				}
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				assert.Len(t, tokens, tc.expectedTokenCount, "token count mismatch")
				assert.Len(t, newStates, tc.expectedStateCount, "state count mismatch")
				// Additional checks: verify that the token's URI and TokenID match the input.
				token := tokens[0]
				assert.Equal(t, tc.params[0].URI, token.URI, "token URI mismatch")
				assert.Equal(t, tc.params[0].TokenID.Int().String(), token.TokenID.Int().String(), "tokenID mismatch")
			}
		})
	}
}

func TestMakeNFToken(t *testing.T) {
	tests := []struct {
		name         string
		stateData    string
		expectError  bool
		expectedURI  string
		expectedSalt string
		expectedTID  string
		expectedOwn  string
	}{
		{
			name: "valid JSON",
			stateData: `{
				"salt": "123",
				"uri": "https://example.com",
				"owner": "0xabcdef",
				"tokenID": "456"
			}`,
			expectError:  false,
			expectedURI:  "https://example.com",
			expectedSalt: "123",
			expectedTID:  "456",
			expectedOwn:  "abcdef",
		},
		{
			name:        "invalid JSON",
			stateData:   `{"salt": "123", "uri": "https://example.com", "owner": "0xabcdef", "tokenID": }`,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token, err := makeNFToken(tc.stateData)
			if tc.expectError {
				require.Error(t, err, "expected an error for invalid JSON")
			} else {
				require.NoError(t, err, "unexpected error for valid JSON")
				require.NotNil(t, token, "expected non-nil token for valid JSON")
				assert.Equal(t, tc.expectedURI, token.URI, "URI mismatch")
				assert.Equal(t, tc.expectedSalt, token.Salt.Int().String(), "salt mismatch")
				assert.Equal(t, tc.expectedTID, token.TokenID.Int().String(), "tokenID mismatch")
				assert.Equal(t, tc.expectedOwn, token.Owner.HexString(), "owner mismatch")
			}
		})
	}
}

func TestMakeNewState(t *testing.T) {
	ctx := context.Background()

	stateSchema := &pb.StateSchema{Id: "schema1"}
	algoName := "test"
	ownerStr := "owner1"
	validToken := types.NewZetoNFToken((*pldtypes.HexUint256)(big.NewInt(456)), "https://example.com", mockPubKey(), big.NewInt(123))

	// Compute expected hash string by calling the common helper.
	expectedHashStr := "11e84f5f703728d1f231655c59597678524e3a14ce684d07a0b653bd51ccd650"

	// Also, compute the JSON representation of the token.
	tokenJSON, err := json.Marshal(validToken)
	require.NoError(t, err)

	tests := []struct {
		name                string
		useNullifiers       bool
		token               *types.ZetoNFToken
		expectErr           bool
		expectedHash        *string // pointer to expected hash string
		expectedStateJSON   string  // expected JSON string of the token
		expectNullifierSpec bool
	}{
		{
			name:                "success without nullifiers",
			useNullifiers:       false,
			token:               validToken,
			expectErr:           false,
			expectedHash:        &expectedHashStr,
			expectedStateJSON:   string(tokenJSON),
			expectNullifierSpec: false,
		},
		{
			name:                "success with nullifiers",
			useNullifiers:       true,
			token:               validToken,
			expectErr:           false,
			expectedHash:        &expectedHashStr,
			expectedStateJSON:   string(tokenJSON),
			expectNullifierSpec: true,
		},
		{
			name:          "failure: invalid token",
			useNullifiers: false,
			token:         types.NewZetoNFToken((*pldtypes.HexUint256)(big.NewInt(456)), "", mockPubKey(), big.NewInt(123)),
			expectErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns, err := makeNewState(ctx, stateSchema, tc.useNullifiers, tc.token, algoName, ownerStr)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				return
			}
			require.NoError(t, err, "unexpected error in test case %q", tc.name)

			// Verify the state ID.
			require.NotNil(t, ns.Id, "state Id should not be nil")
			assert.Equal(t, *tc.expectedHash, *ns.Id, "state Id mismatch")

			// Verify SchemaId and JSON data.
			assert.Equal(t, stateSchema.Id, ns.SchemaId, "SchemaId mismatch")
			assert.Equal(t, tc.expectedStateJSON, ns.StateDataJson, "StateDataJson mismatch")

			// Verify distribution list.
			assert.Equal(t, []string{ownerStr}, ns.DistributionList, "DistributionList mismatch")
			if tc.expectNullifierSpec {
				require.NotEmpty(t, ns.NullifierSpecs, "expected non-empty NullifierSpecs")
				nspec := ns.NullifierSpecs[0]
				expectedAlgo := getAlgoZetoSnarkBJJ(algoName)
				assert.Equal(t, ownerStr, nspec.Party, "Nullifier Party mismatch")
				assert.Equal(t, expectedAlgo, nspec.Algorithm, "Nullifier Algorithm mismatch")
				assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, nspec.VerifierType, "Nullifier VerifierType mismatch")
				assert.Equal(t, zetosignerapi.PAYLOAD_DOMAIN_ZETO_NULLIFIER, nspec.PayloadType, "Nullifier PayloadType mismatch")
			} else {
				assert.Empty(t, ns.NullifierSpecs, "expected empty NullifierSpecs when useNullifiers is false")
			}
		})
	}
}

func TestFindAvailableStates(t *testing.T) {
	ctx := context.Background()
	stateSchema := &pb.StateSchema{Id: "schema1"}
	stateQueryContext := "myQueryContext"
	query := `{"foo": "bar"}`
	useNullifiers := true

	tests := []struct {
		name           string
		callbackErr    bool
		responseStates []*pb.StoredState
		wantErr        bool
		expectedStates []*pb.StoredState
	}{
		{
			name:        "callback error",
			callbackErr: true,
			wantErr:     true,
		},
		{
			name: "successful response",
			responseStates: []*pb.StoredState{
				{
					Id:        "state1",
					SchemaId:  "schema1",
					CreatedAt: 123,
					DataJson:  `{"foo": "bar"}`,
					Locks:     nil,
				},
				{
					Id:        "state2",
					SchemaId:  "schema1",
					CreatedAt: 456,
					DataJson:  `{"baz": "qux"}`,
					Locks:     nil,
				},
			},
			wantErr: false,
			expectedStates: []*pb.StoredState{
				{
					Id:        "state1",
					SchemaId:  "schema1",
					CreatedAt: 123,
					DataJson:  `{"foo": "bar"}`,
					Locks:     nil,
				},
				{
					Id:        "state2",
					SchemaId:  "schema1",
					CreatedAt: 456,
					DataJson:  `{"baz": "qux"}`,
					Locks:     nil,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup the test callbacks.
			var callbacks testDomainCallbacks
			if tc.callbackErr {
				callbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
					return nil, fmt.Errorf("callback error")
				}
			} else {
				callbacks.returnFunc = func() (*pb.FindAvailableStatesResponse, error) {
					return &pb.FindAvailableStatesResponse{
						States: tc.responseStates,
					}, nil
				}
			}

			states, err := findAvailableStates(ctx, &callbacks, stateSchema, useNullifiers, stateQueryContext, query)
			if tc.wantErr {
				require.Error(t, err, "expected error from findAvailableStates")
			} else {
				require.NoError(t, err, "unexpected error from findAvailableStates")
				assert.Equal(t, tc.expectedStates, states, "returned states mismatch")
			}
		})
	}
}
func TestZetoNFToken_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		expected    *types.ZetoNFToken
	}{
		{
			name:        "valid JSON",
			jsonData:    `{"salt": "123", "uri": "https://example.com", "owner": "0xabcdef", "tokenID": "456"}`,
			expectError: false,
			expected: &types.ZetoNFToken{
				Salt:    (*pldtypes.HexUint256)(big.NewInt(123)),
				URI:     "https://example.com",
				Owner:   pldtypes.MustParseHexBytes("0xabcdef"),
				TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
			},
		},
		{
			name:        "invalid JSON",
			jsonData:    `{"salt": "123", "uri": "https://example.com", "owner": "0xabcdef", "tokenID": }`,
			expectError: true,
		},
		{
			name:        "missing fields",
			jsonData:    `{"salt": "123", "uri": "https://example.com"}`,
			expectError: false,
			expected: &types.ZetoNFToken{
				Salt:    (*pldtypes.HexUint256)(big.NewInt(123)),
				URI:     "https://example.com",
				Owner:   nil,
				TokenID: nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var token types.ZetoNFToken
			err := json.Unmarshal([]byte(tc.jsonData), &token)
			if tc.expectError {
				require.Error(t, err, "expected an error for test case %q", tc.name)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
				assert.Equal(t, tc.expected, &token, "token mismatch for test case %q", tc.name)
			}
		})
	}
}
