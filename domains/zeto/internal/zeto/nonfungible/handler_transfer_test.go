package nonfungible

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var _ plugintk.DomainCallbacks = &testDomainCallbacksT{}

type testDomainCallbacksT struct {
	testDomainCallbacks // Embed the default test implementation
	retStates           []*pb.StoredState
	retErr              error
}

func (dc *testDomainCallbacksT) FindAvailableStates(ctx context.Context, req *pb.FindAvailableStatesRequest) (*pb.FindAvailableStatesResponse, error) {
	if dc.retErr != nil {
		return nil, dc.retErr
	}
	return &pb.FindAvailableStatesResponse{
		States: dc.retStates,
	}, nil
}

func TestPrepareInputsForTransfer(t *testing.T) {
	ctx := context.Background()

	// Create a transfer parameter with tokenID 456.
	param := &types.NonFungibleTransferParamEntry{
		To:      "recipient1",
		URI:     "",
		TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
	}

	tests := []struct {
		name             string
		callbacks        *testDomainCallbacksT
		params           []*types.NonFungibleTransferParamEntry
		expectErr        bool
		errContains      string
		expectedTokens   int
		expectedStateRs  int
		expectedParamURI string
	}{
		{
			name: "success: valid state found",
			callbacks: &testDomainCallbacksT{
				retStates: []*pb.StoredState{
					{
						Id:       "state1",
						SchemaId: "schema1",
						DataJson: `{"salt": "123", "uri": "https://example.com", "owner": "0xabcdef", "tokenID": "456"}`,
					},
				},
				retErr: nil,
			},
			params:           []*types.NonFungibleTransferParamEntry{param},
			expectErr:        false,
			expectedTokens:   1,
			expectedStateRs:  1,
			expectedParamURI: "https://example.com",
		},
		{
			name: "failure: callback error",
			callbacks: &testDomainCallbacksT{
				retStates: nil,
				retErr:    fmt.Errorf("callback failure"),
			},
			params:      []*types.NonFungibleTransferParamEntry{param},
			expectErr:   true,
			errContains: "PD210032",
		},
		{
			name: "failure: no available states",
			callbacks: &testDomainCallbacksT{
				retStates: []*pb.StoredState{}, // empty result
				retErr:    nil,
			},
			params:      []*types.NonFungibleTransferParamEntry{param},
			expectErr:   true,
			errContains: "PD210033",
		},
		{
			name: "failure: invalid token JSON",
			callbacks: &testDomainCallbacksT{
				retStates: []*pb.StoredState{
					{
						Id:       "state1",
						SchemaId: "schema1",
						DataJson: `{"salt": "123", "uri": "https://example.com", "owner": "0xabcdef", "tokenID":}`,
					},
				},
				retErr: nil,
			},
			params:      []*types.NonFungibleTransferParamEntry{param},
			expectErr:   true,
			errContains: "PD210034",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function under test.
			tokens, stateRefs, err := prepareInputsForTransfer(ctx, tc.callbacks, &pb.StateSchema{Id: "schema1"}, false, "queryContext", "sender1", tc.params)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				assert.Len(t, tokens, tc.expectedTokens, "token count mismatch")
				assert.Len(t, stateRefs, tc.expectedStateRs, "state reference count mismatch")
				// Also, verify that each transfer parameter's URI was updated.
				for _, p := range tc.params {
					assert.Equal(t, tc.expectedParamURI, p.URI, "parameter URI was not updated correctly")
				}
			}
		})
	}
}

func TestValidateTransferParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		params      []*types.NonFungibleTransferParamEntry
		expectErr   bool
		errContains string
	}{
		{
			name: "valid parameter",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
				},
			},
			expectErr: false,
		},
		{
			name:        "no parameters provided",
			params:      []*types.NonFungibleTransferParamEntry{},
			expectErr:   true,
			errContains: "PD210024",
		},
		{
			name: "empty To field",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "",
					URI:     "https://example.com",
					TokenID: (*pldtypes.HexUint256)(big.NewInt(456)),
				},
			},
			expectErr:   true,
			errContains: "PD210025",
		},
		{
			name: "nil TokenID",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "recipient",
					URI:     "https://example.com",
					TokenID: nil,
				},
			},
			expectErr:   true,
			errContains: "PD210114",
		},
		{
			name: "zero TokenID",
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:  "recipient",
					URI: "https://example.com",
					// Assuming that a tokenID equal to zero is considered invalid.
					TokenID: (*pldtypes.HexUint256)(big.NewInt(0)),
				},
			},
			expectErr:   true,
			errContains: "PD210114",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTransferParams(ctx, tc.params)
			if tc.expectErr {
				require.Error(t, err, "expected an error for test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
			}
		})
	}
}

func TestGetAlgoZetoSnarkBJJ_transferHandler(t *testing.T) {
	h := &transferHandler{
		baseHandler: baseHandler{
			name: "transfer",
		},
	}
	assert.Equal(t, "domain:transfer:snark:babyjubjub", h.getAlgoZetoSnarkBJJ())
}

func TestFormatProvingRequest(t *testing.T) {
	ctx := context.Background()
	handler := &transferHandler{}

	contractAddr := pldtypes.MustEthAddress("0xabc123abc123abc123abc123abc123abc123abc1")

	token := types.NewZetoNFToken(
		(*pldtypes.HexUint256)(big.NewInt(1)),
		"https://input.com",
		mockPubKey(),
		big.NewInt(2),
	)

	// Prepare table-driven test cases.
	tests := []struct {
		name               string
		inputTokens        []*types.ZetoNFToken
		outputTokens       []*types.ZetoNFToken
		circuit            *zetosignerapi.Circuit
		tokenName          string
		queryContext       string
		contractAddr       *pldtypes.EthAddress
		expectErr          bool
		errContains        string
		expectedCircuitId  string
		expectedInputOwner string
	}{
		{
			name:               "success: valid input and output tokens",
			inputTokens:        []*types.ZetoNFToken{token},
			outputTokens:       []*types.ZetoNFToken{token},
			circuit:            &zetosignerapi.Circuit{Name: "circuit123"},
			tokenName:          "nonfungible",
			queryContext:       "ctx1",
			contractAddr:       contractAddr,
			expectErr:          false,
			expectedCircuitId:  "circuit123",
			expectedInputOwner: "51fa904bb6142e89f85aebb2a933a879e2efd5b682021deec4f717a8dbcbbd8e",
		},
		{
			name:         "failure: empty input tokens",
			inputTokens:  []*types.ZetoNFToken{},
			outputTokens: []*types.ZetoNFToken{token},
			circuit:      &zetosignerapi.Circuit{Name: "circuit123"},
			tokenName:    "nonfungible",
			queryContext: "ctx1",
			contractAddr: contractAddr,
			expectErr:    true,
			errContains:  "PD210113", // from processTokens error
		},
		{
			name:         "failure: empty output tokens",
			inputTokens:  []*types.ZetoNFToken{token},
			outputTokens: []*types.ZetoNFToken{}, // output error
			circuit:      &zetosignerapi.Circuit{Name: "circuit123"},
			tokenName:    "nonfungible",
			queryContext: "ctx1",
			contractAddr: contractAddr,
			expectErr:    true,
			errContains:  "PD210113",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payloadBytes, err := handler.formatProvingRequest(ctx, tc.inputTokens, tc.outputTokens, tc.circuit, tc.tokenName, tc.queryContext, tc.contractAddr)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				return
			}
			require.NoError(t, err, "unexpected error in test case %q", tc.name)
			require.NotNil(t, payloadBytes, "payload should not be nil")

			// Check corepb.ProvingRequest
			var req corepb.ProvingRequest
			err = proto.Unmarshal(payloadBytes, &req)
			require.NoError(t, err, "failed to unmarshal payload")

			assert.Equal(t, tc.expectedCircuitId, req.Circuit.Name, "CircuitId mismatch")
			require.NotNil(t, req.Common, "Common must not be nil")
			assert.Equal(t, tc.expectedInputOwner, req.Common.InputOwner, "InputOwner mismatch")

			assert.Equal(t, corepb.TokenType_nunFungible, req.Common.TokenType, "TokenType mismatch")
			// Check corepb.ProvingRequestExtras_NonFungible

			var tokenSecrets corepb.TokenSecrets_NonFungible
			err = json.Unmarshal(req.Common.TokenSecrets, &tokenSecrets)
			require.NoError(t, err, "failed to unmarshal tokenSecrets")

			require.Len(t, tokenSecrets.TokenIds, 1, "expected one tokenId in extras")
			require.Len(t, tokenSecrets.TokenUris, 1, "expected one tokenUri in extras")
			assert.Equal(t, tc.inputTokens[0].TokenID.String(), tokenSecrets.TokenIds[0], "TokenId mismatch in extras")
			assert.Equal(t, tc.inputTokens[0].URI, tokenSecrets.TokenUris[0], "TokenUri mismatch in extras")
		})
	}
}

func TestPrepareState(t *testing.T) {
	ctx := context.Background()

	validOwnerStr := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())).String()

	// Build a valid JSON string for a token.
	validStateJSON := fmt.Sprintf(`{
		"salt": "123",
		"uri": "https://example.com",
		"owner": "%s",
		"tokenID": "456"
	}`, validOwnerStr)

	tests := []struct {
		name               string
		stateDataJSON      string
		expectErr          bool
		errContains        string
		expectHashNonEmpty bool
	}{
		{
			name:               "success",
			stateDataJSON:      validStateJSON,
			expectErr:          false,
			expectHashNonEmpty: true,
		},
		{
			name:          "invalid JSON",
			stateDataJSON: `{"bad_json": }`,
			expectErr:     true,
			errContains:   "PD210045",
		},
		{
			name: "hash error",
			stateDataJSON: fmt.Sprintf(`{
				"salt": "123",
				"uri": "",
				"owner": "%s",
				"tokenID": "0"
			}`, validOwnerStr),
			expectErr:   true,
			errContains: "PD210046",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Construct an EndorsableState with the test JSON.
			state := &pb.EndorsableState{
				StateDataJson: tc.stateDataJSON,
			}
			hashStr, err := prepareState(ctx, state)
			if tc.expectErr {
				require.Error(t, err, "expected error for test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
				if tc.expectHashNonEmpty {
					assert.NotEmpty(t, hashStr, "expected non-empty hash string")
				}
			}
		})
	}
}

func dummyFindAttestationSuccess(name string, attestations []*pb.AttestationResult) *pb.AttestationResult {
	proofRes := &corepb.ProvingResponse{
		Proof: &corepb.SnarkProof{
			A: []string{"a1", "a2"},
			B: []*corepb.B_Item{
				{Items: []string{"b00", "b01"}},
				{Items: []string{"b10", "b11"}},
			},
			C: []string{"c1", "c2"},
		},
		PublicInputs: map[string]string{
			"nullifier": "n1,n2",
			"root":      "r1",
		},
	}
	payload, _ := proto.Marshal(proofRes)
	return &pb.AttestationResult{
		Name:    "sender",
		Payload: payload,
	}
}
func dummyFindAttestationFailed(string, []*pb.AttestationResult) *pb.AttestationResult { return nil }
func dummyFindAttestationBadPayload(string, []*pb.AttestationResult) *pb.AttestationResult {
	return &pb.AttestationResult{Payload: []byte("bad payload")}
}

// dummyEncodeTxData returns fixed transaction data.
func dummyEncodeTxData(ctx context.Context, transaction *pb.TransactionSpecification, infoStates []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
	return []byte("txdata"), nil
}

// dummyEncodeTxDataFailed returns an error
func dummyEncodeTxDataFailed(context.Context, *pb.TransactionSpecification, []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
	return nil, fmt.Errorf("dummyEncodeTxDataFailed")
}

// dummyEncodeProof returns a fixed proof map.
func dummyEncodeProof(proof *corepb.SnarkProof) map[string]interface{} {
	return map[string]interface{}{
		"pA": []string{"a1", "a2"},
		"pB": [][]string{
			{"b01", "b00"},
			{"b11", "b10"},
		},
		"pC": []string{"c1", "c2"},
	}
}

func TestPrepare(t *testing.T) {
	ctx := context.Background()
	defer defaultHelpers()

	validOwnerStr := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())).String()
	validStateJSON := fmt.Sprintf(`{
		"salt": "123",
		"uri": "https://example.com",
		"owner": "%s",
		"tokenID": "456"
	}`, validOwnerStr)
	inputState := &pb.EndorsableState{StateDataJson: validStateJSON}
	outputState := &pb.EndorsableState{StateDataJson: validStateJSON}

	req := &pb.PrepareTransactionRequest{
		AttestationResult: []*pb.AttestationResult{
			{Name: "sender"},
		},
		InputStates:  []*pb.EndorsableState{inputState},
		OutputStates: []*pb.EndorsableState{outputState},
		Transaction:  &pb.TransactionSpecification{TransactionId: "0x1234"},
	}

	tests := []struct {
		name          string
		tx            *types.ParsedTransaction
		req           *pb.PrepareTransactionRequest
		expectErr     bool
		errContains   string
		nullifiers    bool
		assertionFunc func(string, []*pb.AttestationResult) *pb.AttestationResult
		encodeTxFunc  func(context.Context, *pb.TransactionSpecification, []*prototk.EndorsableState) (pldtypes.HexBytes, error)
	}{
		{
			name: "success non-nullifier",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{},
				DomainConfig: &types.DomainInstanceConfig{
					TokenName: constants.TOKEN_NF_ANON,
					Circuits:  &zetosignerapi.Circuits{},
				},
			},
			assertionFunc: dummyFindAttestationSuccess,
			encodeTxFunc:  dummyEncodeTxData,
			req:           req,
			expectErr:     false,
			nullifiers:    false,
		},
		{
			name: "success nullifier",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{},
				DomainConfig: &types.DomainInstanceConfig{
					TokenName: constants.TOKEN_NF_ANON_NULLIFIER,
					Circuits:  &zetosignerapi.Circuits{},
				},
			},
			assertionFunc: dummyFindAttestationSuccess,
			encodeTxFunc:  dummyEncodeTxData,
			req:           req,
			expectErr:     false,
			nullifiers:    true,
		},
		{
			name: "failure: missing attestation",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{},
				DomainConfig: &types.DomainInstanceConfig{
					TokenName: constants.TOKEN_NF_ANON,
					Circuits:  &zetosignerapi.Circuits{},
				},
			},
			req: &pb.PrepareTransactionRequest{
				AttestationResult: []*pb.AttestationResult{}, // empty attestation list.
				InputStates:       []*pb.EndorsableState{inputState},
				OutputStates:      []*pb.EndorsableState{outputState},
				Transaction:       &pb.TransactionSpecification{},
			},
			assertionFunc: dummyFindAttestationFailed,
			encodeTxFunc:  dummyEncodeTxData,
			expectErr:     true,
			errContains:   "PD210043",
		},
		{
			name: "failure: invalid attestation payload",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{},
				DomainConfig: &types.DomainInstanceConfig{
					TokenName: constants.TOKEN_NF_ANON,
					Circuits:  &zetosignerapi.Circuits{},
				},
			},
			req: func() *pb.PrepareTransactionRequest {
				return &pb.PrepareTransactionRequest{
					AttestationResult: []*pb.AttestationResult{},
					InputStates:       []*pb.EndorsableState{inputState},
					OutputStates:      []*pb.EndorsableState{outputState},
					Transaction:       &pb.TransactionSpecification{},
				}
			}(),
			assertionFunc: dummyFindAttestationBadPayload,
			encodeTxFunc:  dummyEncodeTxData,
			expectErr:     true,
			errContains:   "PD210044",
		},
		{
			name: "failure: failed decoding transaction data",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{},
				DomainConfig: &types.DomainInstanceConfig{
					TokenName: constants.TOKEN_NF_ANON,
					Circuits:  &zetosignerapi.Circuits{},
				},
			},
			req: func() *pb.PrepareTransactionRequest {
				return &pb.PrepareTransactionRequest{
					AttestationResult: []*pb.AttestationResult{},
					InputStates:       []*pb.EndorsableState{inputState},
					OutputStates:      []*pb.EndorsableState{outputState},
					Transaction:       &pb.TransactionSpecification{},
				}
			}(),
			assertionFunc: dummyFindAttestationSuccess,
			encodeTxFunc:  dummyEncodeTxDataFailed,
			expectErr:     true,
			errContains:   "PD210049",
		},
	}

	handler := &transferHandler{
		callbacks: nil,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			findAttestationFunc = tc.assertionFunc
			encodeTransactionDataFunc = tc.encodeTxFunc
			encodeProofFunc = dummyEncodeProof

			resp, err := handler.Prepare(ctx, tc.tx, tc.req)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
				}
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				require.NotNil(t, resp, "response should not be nil")

				// Unmarshal function ABI and params from the response.
				var abiObj interface{}
				err = json.Unmarshal([]byte(resp.Transaction.FunctionAbiJson), &abiObj)
				require.NoError(t, err, "failed to unmarshal FunctionAbiJson")

				var params map[string]interface{}
				err = json.Unmarshal([]byte(resp.Transaction.ParamsJson), &params)
				require.NoError(t, err, "failed to unmarshal ParamsJson")

				// For non-nullifier tokens, expect "input" key.
				_, inputExists := params["input"]
				if tc.nullifiers {
					assert.False(t, inputExists, "input key should be removed for nullifier tokens")
					_, nullifierExists := params["nullifier"]
					assert.True(t, nullifierExists, "nullifier key should be present for nullifier tokens")
					_, rootExists := params["root"]
					assert.True(t, rootExists, "root key should be present for nullifier tokens")
				} else {
					assert.True(t, inputExists, "input key should be present for non-nullifier tokens")
				}

				assert.Equal(t, "0x747864617461", params["data"], "data mismatch")
				_, proofExists := params["proof"]
				assert.True(t, proofExists, "proof should be present")
			}
		})
	}
}

func dummyFindVerifier(from string, _ string, _ string, _ []*pb.ResolvedVerifier) *pb.ResolvedVerifier {
	if from == "" {
		return nil
	}
	return &pb.ResolvedVerifier{
		Verifier:     "0x9db52aad8d7fa393ab89fa85b57e69651d2b9e3490cf1743f7c7df503f6e4984",
		VerifierType: "iden3_pubkey_babyjubjub_compressed_0x",
		Lookup:       fmt.Sprintf("%s@node1", from),
		Algorithm:    "domain:zeto_c6dcf5778e528940:snark:babyjubjub",
	}
}

// TestAssemble tests the Assemble method of transferHandler.
func TestAssemble(t *testing.T) {
	ctx := context.Background()
	defer defaultHelpers()

	findVerifierFunc = dummyFindVerifier
	req := &pb.AssembleTransactionRequest{
		Transaction: &pb.TransactionSpecification{
			ContractInfo: &pb.ContractInfo{ContractAddress: "0x04823d1549e948188633bf537e1762b2c43bfb53"},
		},
	}

	domainConfig := &types.DomainInstanceConfig{
		TokenName: constants.TOKEN_NF_ANON,
		Circuits: &zetosignerapi.Circuits{
			"transfer": &zetosignerapi.Circuit{
				Name: "circuit123",
			},
		},
	}

	retStates := []*pb.StoredState{
		{
			Id:       "state1",
			SchemaId: "schema1",
			DataJson: `{"owner":"0x2ed27cebd83a8e05a76fde8f5fbe47fcdf82561dbbcda5e6247f612e8ee59b16","salt":"18555011917455081896159178727281079825727310023017875145998739683254216175481","tokenID":"123","uri":"https://example.com"}`,
		},
	}

	params := []*types.NonFungibleTransferParamEntry{
		{
			To:      "receiver", // empty to field, this should cause an error.
			TokenID: (*pldtypes.HexUint256)(big.NewInt(123)),
		},
	}

	tx := &types.ParsedTransaction{
		Transaction: &pb.TransactionSpecification{
			From:         "sender",
			ContractInfo: &pb.ContractInfo{ContractAddress: "0x04823d1549e948188633bf537e1762b2c43bfb53"},
		},
	}

	tests := []struct {
		name         string
		tx           *types.ParsedTransaction
		req          *pb.AssembleTransactionRequest
		domainConfig *types.DomainInstanceConfig
		params       []*types.NonFungibleTransferParamEntry
		callbacks    plugintk.DomainCallbacks
		expectErr    bool
		errContains  string
		nullifiers   bool // if true, then the token is considered a nullifier token.
	}{
		{
			name:         "success non-nullifier",
			tx:           tx,
			req:          req,
			expectErr:    false,
			nullifiers:   false,
			domainConfig: domainConfig,
			params:       params,
			callbacks: &testDomainCallbacksT{
				retStates: retStates,
			},
		},
		{
			name: "verifier not found",
			tx: &types.ParsedTransaction{
				Transaction: &pb.TransactionSpecification{
					From: "", // empty from field, this should cause an error.
				},
			},
			req:         req,
			expectErr:   true,
			errContains: "PD210036",
			nullifiers:  false,
			params:      []*types.NonFungibleTransferParamEntry{},
		},
		{
			name: "error prepare inputs",
			tx:   tx,
			callbacks: &testDomainCallbacksT{
				retErr: fmt.Errorf("states not found"),
			},
			domainConfig: domainConfig,
			params:       params,
			req:          req,
			expectErr:    true,
			errContains:  "PD210039",
			nullifiers:   false,
		},
		{
			name: "error prepare outputs",
			tx:   tx,
			callbacks: &testDomainCallbacksT{
				retStates: retStates,
			},
			domainConfig: domainConfig,
			params: []*types.NonFungibleTransferParamEntry{
				{
					To:      "", // empty to field, this should cause an error.
					TokenID: (*pldtypes.HexUint256)(big.NewInt(123)),
				},
			},
			req:         req,
			expectErr:   true,
			errContains: "PD210040",
			nullifiers:  false,
		},
		{
			name: "error parsing eth address",
			tx:   tx,
			callbacks: &testDomainCallbacksT{
				retStates: retStates,
			},
			domainConfig: domainConfig,
			params:       params,
			req: &pb.AssembleTransactionRequest{
				Transaction: &pb.TransactionSpecification{
					ContractInfo: &pb.ContractInfo{ContractAddress: "invalid"},
				},
			},
			expectErr:   true,
			errContains: "PD210017",
			nullifiers:  false,
		},
	}

	// Instantiate a transferHandler and override its helper functions.
	handler := &transferHandler{
		callbacks: nil,
		nftSchema: &pb.StateSchema{Id: "schema1"},
		baseHandler: baseHandler{
			name: "transfer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			handler.callbacks = tc.callbacks
			tc.tx.Params = tc.params
			tc.tx.DomainConfig = tc.domainConfig

			resp, err := handler.Assemble(ctx, tc.tx, tc.req)
			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				require.NotNil(t, resp, "response should not be nil")

				// test resp.AssembledTransaction
				assert.Equal(t, 1, len(resp.AssembledTransaction.InputStates))
				assert.Equal(t, 1, len(resp.AssembledTransaction.OutputStates))

				// test resp.AttestationPlan
				require.Equal(t, 1, len(resp.AttestationPlan), "expected one attestation request")
				a := resp.AttestationPlan[0]
				assert.Equal(t, "sender", a.Name, "attestation name mismatch")
				assert.Equal(t, "domain:transfer:snark:babyjubjub", a.Algorithm, "attestation algorithm mismatch")
				assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, a.VerifierType, "attestation verifier type mismatch")
				assert.Equal(t, zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, a.PayloadType, "attestation payload type mismatch")
				assert.Equal(t, tc.tx.Transaction.From, a.Parties[0], "attestation party mismatch")
				assert.NotEmpty(t, a.Payload, "attestation payload should not be empty")
			}
		})
	}
}

func TestValidateParams(t *testing.T) {
	ctx := context.Background()
	// Create a dummy DomainInstanceConfig. (The function doesn't use config, but it must be provided.)
	config := &types.DomainInstanceConfig{}

	// Prepare table test cases.
	tests := []struct {
		name          string
		inputJSON     string
		expectErr     bool
		errContains   string
		expectedCount int
	}{
		{
			name: "valid parameters",
			inputJSON: `{
				"transfers": [
					{
						"to": "recipient1",
						"uri": "https://example.com",
						"tokenID": "123"
					}
				]
			}`,
			expectErr:     false,
			expectedCount: 1,
		},
		{
			name: "empty transfers",
			inputJSON: `{
				"transfers": []
			}`,
			expectErr:   true,
			errContains: "PD210024",
		},
		{
			name: "missing to field",
			inputJSON: `{
				"transfers": [
					{
						"to": "",
						"uri": "https://example.com",
						"tokenID": "123"
					}
				]
			}`,
			expectErr:   true,
			errContains: "PD210025",
		},
		{
			name: "zero tokenID",
			inputJSON: `{
				"transfers": [
					{
						"to": "recipient1",
						"uri": "https://example.com",
						"tokenID": "0"
					}
				]
			}`,
			expectErr:   true,
			errContains: "PD210114",
		},
	}

	// Create a dummy transferHandler (in the same package, so we can access unexported methods).
	handler := &transferHandler{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Call ValidateParams.
			result, err := handler.ValidateParams(ctx, config, tc.inputJSON)
			if tc.expectErr {
				require.Error(t, err, "expected error for test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
				transfers, ok := result.([]*types.NonFungibleTransferParamEntry)
				require.True(t, ok, "result type mismatch")
				assert.Equal(t, tc.expectedCount, len(transfers), "unexpected number of transfer entries")

				if len(transfers) > 0 {
					assert.Equal(t, "recipient1", transfers[0].To)
					assert.Equal(t, "https://example.com", transfers[0].URI)
					// Compare tokenID as a string (converted from big.Int).
					expectedTokenID := big.NewInt(123).String()
					assert.Equal(t, expectedTokenID, transfers[0].TokenID.Int().String())
				}
			}
		})
	}
}

// Dummy transfer parameters for testing.
func newTransferParam(to string, tokenIDValue int64, uri string) *types.NonFungibleTransferParamEntry {
	return &types.NonFungibleTransferParamEntry{
		To:      to,
		URI:     uri,
		TokenID: (*pldtypes.HexUint256)(big.NewInt(tokenIDValue)),
	}
}

// TestInit tests the Init method.
func TestInit(t *testing.T) {
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
	handler := &transferHandler{}

	tests := []struct {
		name              string
		params            []*types.NonFungibleTransferParamEntry
		expectedVerifiers int // expected total number of required verifiers
	}{
		{
			name:              "no transfer params",
			params:            []*types.NonFungibleTransferParamEntry{},
			expectedVerifiers: 1, // Only the sender is added.
		},
		{
			name: "one transfer param",
			params: []*types.NonFungibleTransferParamEntry{
				newTransferParam("recipient1", 123, "https://example.com"),
			},
			expectedVerifiers: 2, // Sender + one recipient.
		},
		{
			name: "multiple transfer params",
			params: []*types.NonFungibleTransferParamEntry{
				newTransferParam("recipient1", 123, "https://example.com"),
				newTransferParam("recipient2", 456, "https://example.org"),
			},
			expectedVerifiers: 3, // Sender + two recipients.
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
				assert.Equal(t, "sender", rv[0].Lookup, "sender verifier lookup mismatch")
				assert.Equal(t, dummyAlgo, rv[0].Algorithm, "sender algorithm mismatch")
				assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, rv[0].VerifierType, "sender verifier type mismatch")
			}
			// Verify that each transfer parameter has generated a verifier.
			for i, param := range tc.params {
				// The i-th transfer param results in the (i+1)-th entry.
				verifier := rv[i+1]
				assert.Equal(t, param.To, verifier.Lookup, "transfer param lookup mismatch at index %d", i)
				assert.Equal(t, dummyAlgo, verifier.Algorithm, "transfer param algorithm mismatch at index %d", i)
				assert.Equal(t, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, verifier.VerifierType, "transfer param verifier type mismatch at index %d", i)
			}
		})
	}
}
func TestNonFungibleTransferEndorse(t *testing.T) {
	h := transferHandler{}
	ctx := context.Background()
	tx := &types.ParsedTransaction{}
	req := &pb.EndorseTransactionRequest{}
	res, err := h.Endorse(ctx, tx, req)
	assert.NoError(t, err)
	assert.Nil(t, res)
}
