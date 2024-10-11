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

package zetosigner

import (
	"testing"

	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDecodeProvingRequest_AnonEnc(t *testing.T) {
	common := pb.ProvingRequestCommon{}
	req := &pb.ProvingRequest{
		CircuitId: constants.CIRCUIT_ANON_ENC,
		Common:    &common,
	}
	bytes, err := proto.Marshal(req)
	require.NoError(t, err)

	_, extras, err := decodeProvingRequest(bytes)
	require.NoError(t, err)
	assert.Empty(t, extras)

	encExtras := &pb.ProvingRequestExtras_Encryption{
		EncryptionNonce: "123456",
	}
	req.Extras, err = proto.Marshal(encExtras)
	require.NoError(t, err)

	bytes, err = proto.Marshal(req)
	require.NoError(t, err)
	_, extras, err = decodeProvingRequest(bytes)
	require.NoError(t, err)
	assert.Equal(t, "123456", extras.(*pb.ProvingRequestExtras_Encryption).EncryptionNonce)
}

func TestDecodeProvingRequest_AnonNullifier(t *testing.T) {
	common := pb.ProvingRequestCommon{}
	req := &pb.ProvingRequest{
		CircuitId: constants.CIRCUIT_ANON_NULLIFIER,
		Common:    &common,
	}
	encExtras := &pb.ProvingRequestExtras_Nullifiers{
		Root: "123456",
		MerkleProofs: []*pb.MerkleProof{
			{
				Nodes: []string{"1", "2", "3"},
			},
		},
		Enabled: []bool{true},
	}
	var err error
	req.Extras, err = proto.Marshal(encExtras)
	assert.NoError(t, err)

	bytes, err := proto.Marshal(req)
	assert.NoError(t, err)

	signReq := &signerapi.SignRequest{
		Payload: bytes,
	}

	bytes, err = proto.Marshal(req)
	assert.NoError(t, err)
	signReq.Payload = bytes
	_, extras, err := decodeProvingRequest(signReq.Payload)
	assert.NoError(t, err)
	assert.Equal(t, "123456", extras.(*pb.ProvingRequestExtras_Nullifiers).Root)
}

func TestDecodeProvingRequest_Fail(t *testing.T) {
	common := pb.ProvingRequestCommon{}
	req := &pb.ProvingRequest{
		CircuitId: constants.CIRCUIT_ANON_ENC,
		Common:    &common,
		Extras:    []byte("invalid"),
	}
	bytes, err := proto.Marshal(req)
	require.NoError(t, err)

	signReq := &signerapi.SignRequest{
		Payload: bytes,
	}
	_, _, err = decodeProvingRequest(signReq.Payload)
	assert.ErrorContains(t, err, "failed to unmarshal proving request extras for circuit anon_enc")

	req.CircuitId = constants.CIRCUIT_ANON_NULLIFIER
	bytes, err = proto.Marshal(req)
	assert.NoError(t, err)

	signReq = &signerapi.SignRequest{
		Payload: bytes,
	}
	_, _, err = decodeProvingRequest(signReq.Payload)
	assert.ErrorContains(t, err, "failed to unmarshal proving request extras for circuit anon_nullifier")
	_, _, err = decodeProvingRequest(bytes)
	assert.ErrorContains(t, err, "cannot parse invalid wire-format data")
}
