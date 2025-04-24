package fungible

import (
	"context"
	"math/big"
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
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
