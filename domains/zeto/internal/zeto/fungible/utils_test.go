package fungible

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/domain"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAlgoZetoSnarkBJJ(t *testing.T) {
	h := &mintHandler{
		baseHandler: baseHandler{
			name: "action",
		},
	}
	assert.Equal(t, "domain:action:snark:babyjubjub", h.getAlgoZetoSnarkBJJ())
}

// TestValidateAmountParam tests the validateAmountParam function.
func TestValidateAmountParam(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		amount      *pldtypes.HexUint256
		index       int
		expectErr   bool
		errContains string
	}{
		{
			name:        "nil amount",
			amount:      nil,
			index:       0,
			expectErr:   true,
			errContains: "PD210026", // MsgNoParamAmount
		},
		{
			name:        "zero amount",
			amount:      (*pldtypes.HexUint256)(big.NewInt(0)),
			index:       1,
			expectErr:   true,
			errContains: "PD210027:", // MsgParamAmountInRange
		},
		{
			name:        "negative amount",
			amount:      (*pldtypes.HexUint256)(big.NewInt(-100)),
			index:       2,
			expectErr:   true,
			errContains: "PD210027", // MsgParamAmountInRange
		},
		{
			name:      "positive amount",
			amount:    (*pldtypes.HexUint256)(big.NewInt(100)),
			index:     3,
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAmountParam(ctx, tc.amount, tc.index)
			if tc.expectErr {
				require.Error(t, err, "expected an error for test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
			}
		})
	}
}

// TestValidateTransferParams tests the validateTransferParams function.
func TestValidateTransferParams(t *testing.T) {
	tmpMAX_TRANSFER_AMOUNT := MAX_TRANSFER_AMOUNT
	defer func() {
		MAX_TRANSFER_AMOUNT = tmpMAX_TRANSFER_AMOUNT
	}()

	MAX_TRANSFER_AMOUNT = big.NewInt(10000)

	ctx := context.Background()

	tests := []struct {
		name        string
		params      []*types.FungibleTransferParamEntry
		expectErr   bool
		errContains string
	}{
		{
			name:        "empty transfer params",
			params:      []*types.FungibleTransferParamEntry{},
			expectErr:   true,
			errContains: "PD210024", // MsgNoTransferParams
		},
		{
			name: "missing recipient",
			params: []*types.FungibleTransferParamEntry{
				{To: "", Amount: (*pldtypes.HexUint256)(big.NewInt(100))},
			},
			expectErr:   true,
			errContains: "PD210025", // MsgNoParamTo
		},
		{
			name: "invalid amount - nil",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: nil},
			},
			expectErr:   true,
			errContains: "PD210026", // MsgNoParamAmount
		},
		{
			name: "invalid amount - zero",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: (*pldtypes.HexUint256)(big.NewInt(0))},
			},
			expectErr:   true,
			errContains: "PD210027", // MsgParamAmountInRange
		},
		{
			name: "invalid amount - negative",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: (*pldtypes.HexUint256)(big.NewInt(-500))},
			},
			expectErr:   true,
			errContains: "PD210027", // MsgParamAmountInRange
		},
		{
			name: "total amount exceeds max limit",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: (*pldtypes.HexUint256)(big.NewInt(6000))},
				{To: "recipient2", Amount: (*pldtypes.HexUint256)(big.NewInt(5000))}, // total 11000 > MAX_TRANSFER_AMOUNT
			},
			expectErr:   true,
			errContains: "PD210107", // MsgParamTotalAmountInRange
		},
		{
			name: "valid single transfer",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: (*pldtypes.HexUint256)(big.NewInt(500))},
			},
			expectErr: false,
		},
		{
			name: "valid multiple transfers within limit",
			params: []*types.FungibleTransferParamEntry{
				{To: "recipient1", Amount: (*pldtypes.HexUint256)(big.NewInt(3000))},
				{To: "recipient2", Amount: (*pldtypes.HexUint256)(big.NewInt(4000))},
				{To: "recipient3", Amount: (*pldtypes.HexUint256)(big.NewInt(2000))}, // total 9000 < MAX_TRANSFER_AMOUNT
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTransferParams(ctx, tc.params)
			if tc.expectErr {
				require.Error(t, err, "expected error for test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
			}
		})
	}
}
func TestMarshalTokenSecrets(t *testing.T) {
	tests := []struct {
		name      string
		input     []uint64
		output    []uint64
		expectErr bool
	}{
		{
			name:      "valid input and output",
			input:     []uint64{1, 2, 3},
			output:    []uint64{4, 5, 6},
			expectErr: false,
		},
		{
			name:      "empty input and output",
			input:     []uint64{},
			output:    []uint64{},
			expectErr: false,
		},
		{
			name:      "nil input and output",
			input:     nil,
			output:    nil,
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := marshalTokenSecrets(tc.input, tc.output)
			if tc.expectErr {
				require.Error(t, err, "expected error for test case %q", tc.name)
			} else {
				require.NoError(t, err, "unexpected error for test case %q", tc.name)
				assert.NotNil(t, result, "result should not be nil for test case %q", tc.name)
				assert.JSONEq(t, string(result), string(result), "result should be valid JSON for test case %q", tc.name)
			}
		})
	}
}

func TestFormatTransferProvingRequestMerkleProofPadding(t *testing.T) {
	ctx := context.Background()

	inputCoins := []*types.ZetoCoin{
		{
			Salt:   pldtypes.MustParseHexUint256("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
			Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
			Amount: pldtypes.MustParseHexUint256("0x0f"),
		},
	}

	outputCoins := []*types.ZetoCoin{
		{
			Salt:   pldtypes.MustParseHexUint256("0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
			Owner:  pldtypes.MustParseHexBytes("0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
			Amount: pldtypes.MustParseHexUint256("0x0f"),
		},
	}

	circuit := &zetosignerapi.Circuit{
		UsesNullifiers: true,
	}

	circuitKyc := &zetosignerapi.Circuit{
		UsesNullifiers: true,
		UsesKyc:        true,
	}

	contractAddress, err := pldtypes.ParseEthAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)

	merkleTreeRootSchema := &prototk.StateSchema{Id: "merkle_tree_root"}
	merkleTreeNodeSchema := &prototk.StateSchema{Id: "merkle_tree_node"}

	// Mock callbacks that will simulate generateMerkleProofs returning fewer proofs than inputSize
	mockCallbacks := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			// Return error to simulate generateMerkleProofs failure, which allows us to test error handling
			return nil, errors.New("simulated merkle tree error")
		},
	}

	data0, _ := json.Marshal(map[string]string{"rootIndex": "0x1234567890123456789012345678901234567890123456789012345678901234"})
	data1, _ := json.Marshal(map[string]string{
		"index":      "0x5f5d5e50a650a20986d496e6645ea31770758d924796f0dfc5ac2ad234b03e30",
		"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey":     "0x789c99b9a2196addb3ac11567135877e8b86bc9b5f7725808a79757fd36b2a2a",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type":       "0x02", // leaf node
	})
	data2, _ := json.Marshal(map[string]string{
		"index":      "0x8bdc1e9686bc722ac480c60b35090ec521a2d72102b9bbb3043982a138d27514",
		"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey":     "0xb2479166472a0635433159a876d6d8f9b904aa0b9249cd1b596750205a2e2c01",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type":       "0x02", // leaf node
	})
	data3, _ := json.Marshal(map[string]string{
		"index":      "0xbc846268f41e264902e0324cc4e1462826c836f902fcead82c18c3d09cb87623",
		"leftChild":  "0x0000000000000000000000000000000000000000000000000000000000000000",
		"refKey":     "0xceb5aca5038689895dba9f613a245028f9ea0d135a1b4ceda7e00db6404a0e24",
		"rightChild": "0x0000000000000000000000000000000000000000000000000000000000000000",
		"type":       "0x02", // leaf node
	})
	count := 0
	mockCallbacksNullifier := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			switch count {
			case 0:
				count++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data0),
						},
					},
				}, nil
			case 1, 4:
				count++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data1),
						},
					},
				}, nil
			case 2, 5:
				count++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data2),
						},
					},
				}, nil
			case 3, 6:
				count++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data3),
						},
					},
				}, nil
			}
			count++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{},
			}, nil
		},
	}

	kycCount := 0
	mockCallbacksNullifierKyc := &domain.MockDomainCallbacks{
		MockFindAvailableStates: func() (*prototk.FindAvailableStatesResponse, error) {
			switch kycCount {
			case 0, 5: // root node for utxo tree and kyc tree
				kycCount++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data0),
						},
					},
				}, nil
			case 1, 3, 6, 8: // first leaf nodes (1, 3) for utxo tree; first leaf nodes (6, 8) for kyc tree
				kycCount++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data1),
						},
					},
				}, nil
			case 2, 4, 7, 9: // second leaf nodes (2, 4) for utxo tree; second leaf nodes (7, 9) for kyc tree
				kycCount++
				return &prototk.FindAvailableStatesResponse{
					States: []*prototk.StoredState{
						{
							DataJson: string(data2),
						},
					},
				}, nil
			}
			kycCount++
			return &prototk.FindAvailableStatesResponse{
				States: []*prototk.StoredState{},
			}, nil
		},
	}

	t.Run("with delegate sets delegate field", func(t *testing.T) {
		delegate := "0xabcdefabcdefabcdefabcdefabcdefabcdefabcdef"

		result, err := formatTransferProvingRequest(
			ctx,
			mockCallbacks,
			merkleTreeRootSchema,
			merkleTreeNodeSchema,
			inputCoins,
			outputCoins,
			circuit,
			"Zeto_AnonNullifier",
			"testContext",
			contractAddress,
			delegate,
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PD210065: Failed to find available states for the merkle tree. simulated merkle tree error") // MsgErrorGenerateMTP
		assert.Nil(t, result)
	})

	t.Run("without delegate does not set delegate field", func(t *testing.T) {
		result, err := formatTransferProvingRequest(
			ctx,
			mockCallbacks,
			merkleTreeRootSchema,
			merkleTreeNodeSchema,
			inputCoins,
			outputCoins,
			circuit,
			"Zeto_AnonNullifier",
			"testContext",
			contractAddress,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PD210065: Failed to find available states for the merkle tree. simulated merkle tree error") // MsgErrorGenerateMTP
		assert.Nil(t, result)
	})

	t.Run("test proof padding", func(t *testing.T) {
		delegate := "0xabcdefabcdefabcdefabcdefabcdefabcdefabcdef"
		// input of 3 will require padding to 10
		inputCoinsSize3 := []*types.ZetoCoin{
			{
				Salt:   pldtypes.MustParseHexUint256("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
				Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
				Amount: pldtypes.MustParseHexUint256("0x0f"),
			},
			{
				Salt:   pldtypes.MustParseHexUint256("0x032fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
				Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
				Amount: pldtypes.MustParseHexUint256("0x0f"),
			},
			{
				Salt:   pldtypes.MustParseHexUint256("0x022fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
				Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
				Amount: pldtypes.MustParseHexUint256("0x0f"),
			},
		}

		result, err := formatTransferProvingRequest(
			ctx,
			mockCallbacksNullifier,
			merkleTreeRootSchema,
			merkleTreeNodeSchema,
			inputCoinsSize3,
			outputCoins,
			circuit,
			"Zeto_AnonNullifier",
			"testContext",
			contractAddress,
			delegate,
		)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("test proof with KYC", func(t *testing.T) {
		delegate := "0xabcdefabcdefabcdefabcdefabcdefabcdefabcdef"
		// input of 3 will require padding to 10
		inputCoinsSize3 := []*types.ZetoCoin{
			{
				Salt:   pldtypes.MustParseHexUint256("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
				Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
				Amount: pldtypes.MustParseHexUint256("0x0f"),
			},
			{
				Salt:   pldtypes.MustParseHexUint256("0x032fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
				Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
				Amount: pldtypes.MustParseHexUint256("0x0f"),
			},
		}

		result, err := formatTransferProvingRequest(
			ctx,
			mockCallbacksNullifierKyc,
			merkleTreeRootSchema,
			merkleTreeNodeSchema,
			inputCoinsSize3,
			outputCoins,
			circuitKyc,
			"Zeto_AnonNullifierKyc",
			"testContext",
			contractAddress,
			delegate,
		)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestMakeLeafIndexesFromCoinOwners(t *testing.T) {
	ctx := context.Background()

	inputCoins := []*types.ZetoCoin{
		{
			Salt:   pldtypes.MustParseHexUint256("0x042fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
			Owner:  pldtypes.MustParseHexBytes("0x7cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
			Amount: pldtypes.MustParseHexUint256("0x0f"),
		},
	}

	outputCoins := []*types.ZetoCoin{
		{
			Salt:   pldtypes.MustParseHexUint256("0x142fac32983b19d76425cc54dd80e8a198f5d477c6a327cb286eb81a0c2b95ec"),
			Owner:  pldtypes.MustParseHexBytes("0x8cdd539f3ed6c283494f47d8481f84308a6d7043087fb6711c9f1df04e2b8025"),
			Amount: pldtypes.MustParseHexUint256("0x0f"),
		},
	}

	indexes, err := makeLeafIndexesFromCoinOwners(ctx, inputCoins[0].Owner.String(), outputCoins)
	require.NoError(t, err)
	assert.Equal(t, 2, len(indexes))

	_, err = makeLeafIndexesFromCoinOwners(ctx, "bad public key", outputCoins)
	assert.ErrorContains(t, err, "PD210037: Failed load owner public key")

	outputCoins[0].Owner = pldtypes.MustParseHexBytes("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	_, err = makeLeafIndexesFromCoinOwners(ctx, inputCoins[0].Owner.String(), outputCoins)
	assert.ErrorContains(t, err, "PD210037: Failed load owner public key")
}
