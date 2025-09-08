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

package witness

import (
	"context"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNonFungibleWitnessInputs(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         *pb.ProvingRequestCommon
		expectErr   bool
		errContains string
	}{
		{
			name: "Successful Validation",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr: false,
		},
		{
			name: "Mismatched Input Commitments and Salts",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3"},
				TokenSecrets:     []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210095",
		},
		{
			name: "Invalid Token Type",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210123",
		},
		{
			name: "Invalid Token Secrets",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`invalid`),
			},
			expectErr:   true,
			errContains: "PD210122",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NonFungibleWitnessInputs{}
			err := f.Validate(ctx, tc.req)

			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
			}
		})
	}
}
func TestValidateFungibleWitnessInputs(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         *pb.ProvingRequestCommon
		expectErr   bool
		errContains string
	}{
		{
			name: "Successful Validation",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr: false,
		},
		{
			name: "Mismatched Input Commitments and Salts",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3"},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr:   true,
			errContains: "PD210095",
		},
		{
			name: "Invalid Token Type",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr:   true,
			errContains: "PD210123",
		},
		{
			name: "Invalid Token Secrets",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`invalid`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr:   true,
			errContains: "PD210121",
		},
		{
			name: "Mismatched Input Commitments and Input Values",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"inputValues":[10],"outputValues":[30,0]}`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr:   true,
			errContains: "PD210095",
		},
		{
			name: "Mismatched Output Values and Output Owners",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_fungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30]}`),
				OutputOwners:     []string{"owner1", "owner2"},
			},
			expectErr:   true,
			errContains: "PD210098",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := FungibleWitnessInputs{}
			err := f.Validate(ctx, tc.req)

			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
			}
		})
	}
}
func TestBuildNonFungibleWitnessInputs(t *testing.T) {
	ctx := context.Background()

	alice := common.NewTestKeypair()
	sender := alice.PublicKey.Compress().String()
	bob := common.NewTestKeypair()
	receiver := bob.PublicKey.Compress().String()

	tests := []struct {
		name         string
		req          *pb.ProvingRequestCommon
		expectErr    bool
		errContains  string
		validateFunc func(*testing.T, *NonFungibleWitnessInputs)
	}{
		{
			name: "Successful Non-Fungible Circuit Input Build",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"1", "2"},
				InputSalts:        []string{"3", "4"},
				OutputCommitments: []string{"10", "20"},
				OutputSalts:       []string{"5", "6"},
				OutputOwners:      []string{sender, receiver},
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr: false,
			validateFunc: func(t *testing.T, inputs *NonFungibleWitnessInputs) {
				assert.Equal(t, 2, len(inputs.outputOwnerPublicKeys))
				assert.Equal(t, alice.PublicKey.X.Text(10), inputs.outputOwnerPublicKeys[0][0].Text(10))
				assert.Equal(t, alice.PublicKey.Y.Text(10), inputs.outputOwnerPublicKeys[0][1].Text(10))
				assert.Equal(t, bob.PublicKey.X.Text(10), inputs.outputOwnerPublicKeys[1][0].Text(10))
				assert.Equal(t, bob.PublicKey.Y.Text(10), inputs.outputOwnerPublicKeys[1][1].Text(10))
			},
		},
		{
			name: "Invalid Public Key Length",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"1", "2"},
				InputSalts:        []string{"3", "4"},
				OutputCommitments: []string{"10", "20"},
				OutputSalts:       []string{"5", "6"},
				OutputOwners:      []string{"1234", "5678"}, // Invalid compressed public keys
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210037: Failed load owner public key. PD210072: Invalid compressed public key length",
		},
		{
			name: "Invalid Input Commitment",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"XYZ", "2"}, // Invalid hex
				InputSalts:        []string{"3", "4"},
				OutputCommitments: []string{"10", "20"},
				OutputSalts:       []string{"5", "6"},
				OutputOwners:      []string{sender, receiver},
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210084: Failed to parse input commitment",
		},
		{
			name: "Invalid Input Salt",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"1", "2"},
				InputSalts:        []string{"XYZ", "4"}, // Invalid hex
				OutputCommitments: []string{"10", "20"},
				OutputSalts:       []string{"5", "6"},
				OutputOwners:      []string{sender, receiver},
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210082: Failed to parse input salt",
		},
		{
			name: "Invalid Output Commitment",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"1", "2"},
				InputSalts:        []string{"3", "4"},
				OutputCommitments: []string{"XYZ", "20"}, // Invalid hex
				OutputSalts:       []string{"5", "6"},
				OutputOwners:      []string{sender, receiver},
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210047: Failed to parse output states.",
		},
		{
			name: "Invalid Output Salt",
			req: &pb.ProvingRequestCommon{
				InputCommitments:  []string{"1", "2"},
				InputSalts:        []string{"3", "4"},
				OutputCommitments: []string{"10", "20"},
				OutputSalts:       []string{"XYZ", "6"}, // Invalid hex
				OutputOwners:      []string{sender, receiver},
				TokenSecrets:      []byte(`{"tokenIds":["1","2"],"tokenUris":["uri1","uri2"]}`),
			},
			expectErr:   true,
			errContains: "PD210083: Failed to parse output salt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NonFungibleWitnessInputs{}
			err := f.Build(ctx, tc.req)

			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				if tc.validateFunc != nil {
					tc.validateFunc(t, &f)
				}
			}
		})
	}
}
func TestBuildFungibleWitnessInputs(t *testing.T) {
	ctx := context.Background()

	alice := common.NewTestKeypair()
	sender := alice.PublicKey.Compress().String()
	bob := common.NewTestKeypair()
	receiver := bob.PublicKey.Compress().String()

	tests := []struct {
		name         string
		req          *pb.ProvingRequestCommon
		expectErr    bool
		errContains  string
		validateFunc func(*testing.T, *FungibleWitnessInputs)
	}{
		{
			name: "Successful Fungible Circuit Input Build",
			req: &pb.ProvingRequestCommon{
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				OutputSalts:      []string{"5", "0"},
				OutputOwners:     []string{sender, receiver},
				TokenType:        pb.TokenType_fungible,
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			expectErr: false,
			validateFunc: func(t *testing.T, inputs *FungibleWitnessInputs) {
				assert.Equal(t, 2, len(inputs.outputOwnerPublicKeys))
				assert.Equal(t, alice.PublicKey.X.Text(10), inputs.outputOwnerPublicKeys[0][0].Text(10))
				assert.Equal(t, alice.PublicKey.Y.Text(10), inputs.outputOwnerPublicKeys[0][1].Text(10))
				assert.Equal(t, "0", inputs.outputOwnerPublicKeys[1][0].Text(10))
				assert.Equal(t, "0", inputs.outputOwnerPublicKeys[1][1].Text(10))
				assert.Equal(t, "0", inputs.outputValues[1].Text(10))
				assert.Equal(t, "0", inputs.outputCommitments[1].Text(10))
			},
		},
		{
			name: "Invalid Public Key Length",
			req: &pb.ProvingRequestCommon{
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				OutputSalts:      []string{"5", "0"},
				OutputOwners:     []string{"1234", "5678"},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			expectErr:   true,
			errContains: "PD210037: Failed load owner public key. PD210072: Invalid compressed public key length",
		},
		{
			name: "Invalid Output Salt Format",
			req: &pb.ProvingRequestCommon{
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				OutputSalts:      []string{"0x5", "0x1"},
				OutputOwners:     []string{sender, receiver},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			expectErr:   true,
			errContains: "PD210083: Failed to parse output salt",
		},
		{
			name: "Invalid Input Commitment",
			req: &pb.ProvingRequestCommon{
				InputCommitments: []string{"XYZ", "2"},
				InputSalts:       []string{"3", "4"},
				OutputSalts:      []string{"5", "0"},
				OutputOwners:     []string{sender, receiver},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			expectErr:   true,
			errContains: "PD210084: Failed to parse input commitment",
		},
		{
			name: "Invalid Input Salt",
			req: &pb.ProvingRequestCommon{
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"XYZ", "4"},
				OutputSalts:      []string{"5", "0"},
				OutputOwners:     []string{sender, receiver},
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			expectErr:   true,
			errContains: "PD210082: Failed to parse input salt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := FungibleWitnessInputs{}
			err := f.Build(ctx, tc.req)

			if tc.expectErr {
				require.Error(t, err, "expected error in test case %q", tc.name)
				assert.Contains(t, err.Error(), tc.errContains, "error message should contain %q", tc.errContains)
			} else {
				require.NoError(t, err, "unexpected error in test case %q", tc.name)
				if tc.validateFunc != nil {
					tc.validateFunc(t, &f)
				}
			}
		})
	}
}

func TestAssembleFungibleWitnessInputs(t *testing.T) {
	inputs := FungibleWitnessInputs{
		CommonWitnessInputs: CommonWitnessInputs{
			inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
			inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
		},
		inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
		outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Equal(t, key.PrivateKeyForZkp, result["inputOwnerPrivateKey"])
}

func TestAssembleDepositWitnessInputs(t *testing.T) {
	inputs := DepositWitnessInputs{
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments:      []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:            []*big.Int{big.NewInt(3), big.NewInt(4)},
				outputCommitments:     []*big.Int{big.NewInt(9), big.NewInt(10)},
				outputSalts:           []*big.Int{big.NewInt(11), big.NewInt(12)},
				outputOwnerPublicKeys: [][]*big.Int{},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.NotContains(t, result, "inputCommitments")
	assert.NotContains(t, result, "inputSalts")
	assert.NotContains(t, result, "inputValues")
	assert.NotContains(t, result, "inputOwnerPrivateKey")
	assert.Equal(t, inputs.outputCommitments, result["outputCommitments"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Equal(t, inputs.outputSalts, result["outputSalts"])
	assert.Equal(t, inputs.outputOwnerPublicKeys, result["outputOwnerPublicKeys"])
}

func TestAssembleNonFungibleWitnessInputs(t *testing.T) {
	inputs := NonFungibleWitnessInputs{
		CommonWitnessInputs: CommonWitnessInputs{
			inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
			inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
		},
		tokenIDs:  []*big.Int{big.NewInt(5), big.NewInt(6)},
		tokenURIs: []*big.Int{big.NewInt(7), big.NewInt(8)},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.tokenIDs, result["tokenIds"])
	assert.Equal(t, inputs.tokenURIs, result["tokenUris"])
	assert.Equal(t, key.PrivateKeyForZkp, result["inputOwnerPrivateKey"])
}

func TestAssembleFungibleEncWitnessInputs(t *testing.T) {
	inputs := FungibleEncWitnessInputs{
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Contains(t, result, "encryptionNonce")
	assert.Contains(t, result, "ecdhPrivateKey")
}

func TestAssembleFungibleNullifierWitnessInputs(t *testing.T) {
	ras := &pb.ProvingRequestExtras_Nullifiers{
		SmtProof: &pb.MerkleProofObject{
			Root: "123456",
			MerkleProofs: []*pb.MerkleProof{
				{
					Nodes: []string{"1", "2", "3"},
				},
				{
					Nodes: []string{"0", "0", "0"},
				},
			},
			Enabled: []bool{true, false},
		},
		Delegate: "0x1234567890123456789012345678901234567890",
	}
	inputs := FungibleNullifierWitnessInputs{
		Extras: ras,
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Contains(t, result, "nullifiers")
	assert.Contains(t, result, "root")
	assert.Contains(t, result, "merkleProof")
	assert.Contains(t, result, "enabled")
}

func TestAssembleFungibleNullifierKycWitnessInputs(t *testing.T) {
	ras := &pb.ProvingRequestExtras_NullifiersKyc{
		SmtUtxoProof: &pb.MerkleProofObject{
			Root: "123456",
			MerkleProofs: []*pb.MerkleProof{
				{
					Nodes: []string{"1", "2", "3"},
				},
				{
					Nodes: []string{"0", "0", "0"},
				},
			},
			Enabled: []bool{true, false},
		},
		SmtKycProof: &pb.MerkleProofObject{
			Root: "123456",
			MerkleProofs: []*pb.MerkleProof{
				{
					Nodes: []string{"1", "2", "3"},
				},
				{
					Nodes: []string{"0", "0", "0"},
				},
			},
			Enabled: []bool{true, false},
		},
		Delegate: "0x1234567890123456789012345678901234567890",
	}
	inputs := FungibleNullifierKycWitnessInputs{
		Extras: ras,
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()

	t.Run("valid inputs should succeed", func(t *testing.T) {
		result, err := inputs.Assemble(ctx, &key)
		assert.NoError(t, err)
		assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
		assert.Equal(t, inputs.inputSalts, result["inputSalts"])
		assert.Equal(t, inputs.inputValues, result["inputValues"])
		assert.Equal(t, inputs.outputValues, result["outputValues"])
		assert.Contains(t, result, "nullifiers")
		assert.Contains(t, result, "utxosRoot")
		assert.Contains(t, result, "utxosMerkleProof")
		assert.Contains(t, result, "enabled")
		assert.Contains(t, result, "identitiesRoot")
		assert.Contains(t, result, "identitiesMerkleProof")
	})

	t.Run("invalid salts should fail", func(t *testing.T) {
		invalidField, OK := big.NewInt(0).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
		require.True(t, OK)
		inputs.inputSalts = []*big.Int{big.NewInt(3), invalidField}
		_, err := inputs.Assemble(ctx, &key)
		assert.ErrorContains(t, err, "PD210079: Failed to calculate nullifier. inputs values not inside Finite Field")
	})

	t.Run("skip empty input commitments", func(t *testing.T) {
		inputs.inputCommitments = []*big.Int{big.NewInt(1), big.NewInt(0)}
		_, err := inputs.Assemble(ctx, &key)
		assert.NoError(t, err)
	})

	t.Run("fail to decode smt proof", func(t *testing.T) {
		inputs.inputCommitments = []*big.Int{big.NewInt(1), big.NewInt(2)}
		inputs.inputSalts = []*big.Int{big.NewInt(3), big.NewInt(4)}
		ras.SmtUtxoProof.MerkleProofs[0].Nodes[0] = "invalid"
		_, err := inputs.Assemble(ctx, &key)
		assert.ErrorContains(t, err, "PD210081: Failed to decode node in merkle proof in extras")
	})

	t.Run("fail to decode smt KYC proof", func(t *testing.T) {
		ras.SmtUtxoProof.MerkleProofs[0].Nodes[0] = "1"
		ras.SmtKycProof.MerkleProofs[0].Nodes[0] = "invalid"
		_, err := inputs.Assemble(ctx, &key)
		assert.ErrorContains(t, err, "PD210081: Failed to decode node in merkle proof in extras")
	})

	t.Run("fail to decode delegate", func(t *testing.T) {
		ras.SmtKycProof.MerkleProofs[0].Nodes[0] = "1"
		ras.Delegate = "invalid"
		_, err := inputs.Assemble(ctx, &key)
		assert.ErrorContains(t, err, "PD210132: Failed to decode delegate in extras. invalid")
	})
}

func TestPrepareInputsForNullifiers(t *testing.T) {
	ras := &pb.ProvingRequestExtras_Nullifiers{
		SmtProof: &pb.MerkleProofObject{
			Root: "123456",
			MerkleProofs: []*pb.MerkleProof{
				{
					Nodes: []string{"1", "2", "3"},
				},
				{
					Nodes: []string{"0", "0", "0"},
				},
			},
			Enabled: []bool{true, false},
		},
	}
	inputs := FungibleNullifierWitnessInputs{
		Extras: ras,
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	nullifiers, root, proofs, enabled, _, err := inputs.prepareInputsForNullifiers(ctx, ras, &key)
	assert.NoError(t, err)

	a, _ := new(big.Int).SetString("11227379794363607928937567982576521385233423371965312580610589067804815929531", 0)
	b, _ := new(big.Int).SetString("18511661912667214378913629034224397005995848268976364384592575521219418327022", 0)

	assert.Equal(t, []*big.Int{a, b}, nullifiers)
	assert.Equal(t, big.NewInt(0x123456), root)
	assert.Equal(t, [][]*big.Int{
		{big.NewInt(1), big.NewInt(2), big.NewInt(3)},
		{big.NewInt(0), big.NewInt(0), big.NewInt(0)},
	}, proofs)
	assert.Equal(t, []*big.Int{big.NewInt(1), big.NewInt(0)}, enabled)
}

func TestAssembleLockWitnessInputs(t *testing.T) {
	inputs := LockWitnessInputs{
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues: []*big.Int{big.NewInt(5), big.NewInt(6)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["commitments"])
	assert.Equal(t, inputs.inputValues, result["values"])
	assert.Equal(t, inputs.inputSalts, result["salts"])
	assert.Equal(t, key.PrivateKeyForZkp, result["ownerPrivateKey"])
}

func TestAssembleInputsLock(t *testing.T) {
	inputs := CommonWitnessInputs{
		inputCommitments: []*big.Int{big.NewInt(100)},
		inputSalts:       []*big.Int{big.NewInt(200)},
	}
	_, _, zkpKey := newKeypair()
	key := core.KeyEntry{
		PrivateKeyForZkp: zkpKey,
	}
	result := inputs.Assemble(&key)
	assert.Equal(t, "100", result["inputCommitments"].([]*big.Int)[0].Text(10))
}

func newKeypair() (*babyjub.PrivateKey, *babyjub.PublicKey, *big.Int) {
	// generate babyJubjub private key randomly
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	// generate public key from private key
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &babyJubjubPrivKey, babyJubjubPubKey, privKeyBigInt
}

func TestFungibleEncWitnessInputs_Assemble_ErrorCases(t *testing.T) {
	ctx := context.Background()
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}

	tests := []struct {
		name        string
		inputs      FungibleEncWitnessInputs
		expectErr   bool
		errContains string
	}{
		{
			name: "invalid encryption nonce",
			inputs: FungibleEncWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
				Enc: &pb.ProvingRequestExtras_Encryption{
					EncryptionNonce: "invalid_nonce",
				},
			},
			expectErr:   true,
			errContains: "PD210077: Failed to parse encryption nonce",
		},
		{
			name: "valid custom encryption nonce",
			inputs: FungibleEncWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
				Enc: &pb.ProvingRequestExtras_Encryption{
					EncryptionNonce: "12345",
				},
			},
			expectErr: false,
		},
		{
			name: "nil encryption extras (should generate random nonce)",
			inputs: FungibleEncWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
				Enc: nil,
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.inputs.Assemble(ctx, &key)

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				assert.Contains(t, result, "encryptionNonce")
				assert.Contains(t, result, "ecdhPrivateKey")
				assert.NotNil(t, result["encryptionNonce"])
				assert.NotNil(t, result["ecdhPrivateKey"])
			}
		})
	}
}
func TestPrepareInputsForNullifiers_ErrorCases(t *testing.T) {
	ctx := context.Background()
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}

	tests := []struct {
		name        string
		extras      *pb.ProvingRequestExtras_Nullifiers
		inputs      FungibleNullifierWitnessInputs
		expectErr   bool
		errContains string
	}{
		{
			name: "invalid root format",
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root:         "invalid_hex",
					MerkleProofs: []*pb.MerkleProof{},
					Enabled:      []bool{},
				},
			},
			inputs: FungibleNullifierWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
			},
			expectErr:   true,
			errContains: "PD210080: Failed to decode root value in extras",
		},
		{
			name: "invalid merkle proof node",
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root: "123456",
					MerkleProofs: []*pb.MerkleProof{
						{
							Nodes: []string{"invalid_hex"},
						},
					},
					Enabled: []bool{true},
				},
			},
			inputs: FungibleNullifierWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
			},
			expectErr:   true,
			errContains: "PD210081: Failed to decode node in merkle proof in extras",
		},
		{
			name: "invalid delegate address",
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root:         "123456",
					MerkleProofs: []*pb.MerkleProof{},
					Enabled:      []bool{},
				},
				Delegate: "invalid_address",
			},
			inputs: FungibleNullifierWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
			},
			expectErr:   true,
			errContains: "PD210132: Failed to decode delegate in extras. invalid_address",
		},
		{
			name: "valid delegate with 0x prefix",
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root:         "123456",
					MerkleProofs: []*pb.MerkleProof{},
					Enabled:      []bool{},
				},
				Delegate: "0x1234567890123456789012345678901234567890",
			},
			inputs: FungibleNullifierWitnessInputs{
				FungibleWitnessInputs: FungibleWitnessInputs{
					CommonWitnessInputs: CommonWitnessInputs{
						inputCommitments: []*big.Int{big.NewInt(1)},
						inputSalts:       []*big.Int{big.NewInt(3)},
					},
					inputValues: []*big.Int{big.NewInt(5)},
				},
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, err := tc.inputs.prepareInputsForNullifiers(ctx, tc.extras, &key)

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNonFungibleWitnessInputs_Validate_AdditionalCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         *pb.ProvingRequestCommon
		expectErr   bool
		errContains string
	}{
		{
			name: "empty token secrets",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     []byte(`{}`),
			},
			expectErr: false, // Empty is valid
		},
		{
			name: "null token secrets",
			req: &pb.ProvingRequestCommon{
				TokenType:        pb.TokenType_nunFungible,
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				TokenSecrets:     nil,
			},
			expectErr:   true,
			errContains: "PD210122",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NonFungibleWitnessInputs{}
			err := f.Validate(ctx, tc.req)

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFungibleNullifierWitnessInputs_ZeroValueHandling(t *testing.T) {
	ctx := context.Background()
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}

	ras := &pb.ProvingRequestExtras_Nullifiers{
		SmtProof: &pb.MerkleProofObject{
			Root: "123456",
			MerkleProofs: []*pb.MerkleProof{
				{
					Nodes: []string{"1", "2", "3"},
				},
				{
					Nodes: []string{"0", "0", "0"},
				},
			},
			Enabled: []bool{true, false},
		},
	}

	inputs := FungibleNullifierWitnessInputs{
		Extras: ras,
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(0), big.NewInt(2)}, // First commitment is 0 (filler)
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues: []*big.Int{big.NewInt(0), big.NewInt(6)}, // First value is 0 (filler)
		},
	}

	nullifiers, _, _, _, _, err := inputs.prepareInputsForNullifiers(ctx, ras, &key)
	require.NoError(t, err)

	// First nullifier should be 0 for zero commitment
	assert.Equal(t, big.NewInt(0), nullifiers[0], "nullifier for zero commitment should be zero")
	// Second nullifier should be calculated normally
	assert.NotEqual(t, big.NewInt(0), nullifiers[1], "nullifier for non-zero commitment should be calculated")
}

func TestLockWitnessInputs_EmptyInputs(t *testing.T) {
	inputs := LockWitnessInputs{
		FungibleWitnessInputs: FungibleWitnessInputs{
			CommonWitnessInputs: CommonWitnessInputs{
				inputCommitments: []*big.Int{},
				inputSalts:       []*big.Int{},
			},
			inputValues: []*big.Int{},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.Assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, []*big.Int{}, result["commitments"])
	assert.Equal(t, []*big.Int{}, result["values"])
	assert.Equal(t, []*big.Int{}, result["salts"])
	assert.Equal(t, key.PrivateKeyForZkp, result["ownerPrivateKey"])
}
