/*
 * Copyright © 2025 Kaleido, Inc.
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

	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDecodeProvingRequest_Fail(t *testing.T) {
	common := pb.ProvingRequestCommon{}
	circuit := pb.Circuit{
		Name:           "anon_enc",
		UsesEncryption: true,
	}
	req := &pb.ProvingRequest{
		Circuit: &circuit,
		Common:  &common,
		Extras:  []byte("invalid"),
	}
	bytes, err := proto.Marshal(req)
	require.NoError(t, err)

	ctx := context.Background()
	signReq := &prototk.SignWithKeyRequest{
		Payload: bytes,
	}
	_, _, err = decodeProvingRequest(ctx, signReq.Payload)
	assert.ErrorContains(t, err, "PD210076: Failed to unmarshal proving request extras for circuit anon_enc")

	circuit = pb.Circuit{
		Name:           "anon_nullifier",
		UsesNullifiers: true,
	}
	req.Circuit = &circuit
	bytes, err = proto.Marshal(req)
	assert.NoError(t, err)

	signReq = &prototk.SignWithKeyRequest{
		Payload: bytes,
	}
	_, _, err = decodeProvingRequest(ctx, signReq.Payload)
	assert.ErrorContains(t, err, "PD210076: Failed to unmarshal proving request extras for circuit anon_nullifier")
	_, _, err = decodeProvingRequest(ctx, bytes)
	assert.ErrorContains(t, err, "cannot parse invalid wire-format data")
}

func TestInvalidDecodeProvingRequest(t *testing.T) {
	_, _, err := decodeProvingRequest(context.Background(), []byte("invalid"))
	assert.Error(t, err)
}

func TestDecodeProvingRequest(t *testing.T) {
	tests := []struct {
		name        string
		circuit     *zetosignerapi.Circuit
		extras      interface{}
		expectError bool
		expectValue interface{}
	}{
		{
			name:    "AnonEnc No Extras",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
		},
		{
			name:    "AnonEnc With Extras",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			extras: &pb.ProvingRequestExtras_Encryption{
				EncryptionNonce: "123456",
			},
			expectValue: "123456",
		},
		{
			name:    "AnonNullifier With Extras",
			circuit: &zetosignerapi.Circuit{Name: "anon_nullifier", UsesNullifiers: true},
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root: "123456",
					MerkleProofs: []*pb.MerkleProof{
						{
							Nodes: []string{"1", "2", "3"},
						},
					},
					Enabled: []bool{true},
				},
			},
			expectValue: "123456",
		},
		{
			name:        "AnonNullifier Invalid Extras",
			circuit:     &zetosignerapi.Circuit{Name: "anon_nullifier", UsesNullifiers: true},
			extras:      []byte("invalid"),
			expectError: true,
		},
		{
			name:    "AnonNullifier valid Extras",
			circuit: &zetosignerapi.Circuit{Name: "anon_nullifier", UsesNullifiers: true},
			extras: &pb.ProvingRequestExtras_Nullifiers{
				SmtProof: &pb.MerkleProofObject{
					Root: "123456",
					MerkleProofs: []*pb.MerkleProof{
						{
							Nodes: []string{"1", "2", "3"},
						},
					},
					Enabled: []bool{true},
				},
			},
			expectValue: "123456",
		},
		{
			name:        "NfAnonNullifier Invalid Extras",
			circuit:     &zetosignerapi.Circuit{Name: "nf_anon_nullifier", UsesNullifiers: true},
			extras:      []byte("invalid"),
			expectError: true,
		},
		{
			name:        "Invalid Extras",
			circuit:     &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			extras:      []byte("invalid"),
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common := pb.ProvingRequestCommon{}
			req := &pb.ProvingRequest{
				Circuit: tt.circuit.ToProto(),
				Common:  &common,
			}

			if tt.extras != nil {
				var err error
				switch extras := tt.extras.(type) {
				case proto.Message:
					req.Extras, err = proto.Marshal(extras)
					require.NoError(t, err)
				case []byte:
					req.Extras = extras
				}
			}

			bytes, err := proto.Marshal(req)
			require.NoError(t, err)

			ctx := context.Background()
			_, extras, err := decodeProvingRequest(ctx, bytes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectValue != nil {
					switch v := extras.(type) {
					case *pb.ProvingRequestExtras_Encryption:
						assert.Equal(t, tt.expectValue, v.EncryptionNonce)
					case *pb.ProvingRequestExtras_Nullifiers:
						assert.Equal(t, tt.expectValue, v.SmtProof.Root)
					}
				} else {
					assert.Empty(t, extras)
				}
			}
		})
	}

}
