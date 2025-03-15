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

package signer

import (
	"context"
	"testing"

	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
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
			f := nonFungibleWitnessInputs{}
			err := f.validate(ctx, tc.req)

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
			f := fungibleWitnessInputs{}
			err := f.validate(ctx, tc.req)

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

	alice := NewTestKeypair()
	sender := alice.PublicKey.Compress().String()
	bob := NewTestKeypair()
	receiver := bob.PublicKey.Compress().String()

	tests := []struct {
		name         string
		req          *pb.ProvingRequestCommon
		expectErr    bool
		errContains  string
		validateFunc func(*testing.T, *nonFungibleWitnessInputs)
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
			validateFunc: func(t *testing.T, inputs *nonFungibleWitnessInputs) {
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
			f := nonFungibleWitnessInputs{}
			err := f.build(ctx, tc.req)

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

	alice := NewTestKeypair()
	sender := alice.PublicKey.Compress().String()
	bob := NewTestKeypair()
	receiver := bob.PublicKey.Compress().String()

	tests := []struct {
		name         string
		req          *pb.ProvingRequestCommon
		expectErr    bool
		errContains  string
		validateFunc func(*testing.T, *fungibleWitnessInputs)
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
			validateFunc: func(t *testing.T, inputs *fungibleWitnessInputs) {
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
			f := fungibleWitnessInputs{}
			err := f.build(ctx, tc.req)

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
