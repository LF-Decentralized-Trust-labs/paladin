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

package secp256k1

import (
	"context"
	"testing"

	k1 "github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
	registry := make(map[string]api.InMemorySigner)
	Register(registry)
	assert.Equal(t, 1, len(registry))
}

func TestNewSigner(t *testing.T) {
	keypair, err := k1.GenerateSecp256k1KeyPair()
	assert.NoError(t, err)
	signer := &sepc256k1Signer{}
	res, err := signer.Sign(context.Background(), keypair.PrivateKeyBytes(), &proto.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
		Payload:   ([]byte)("something to sign"),
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)
}
