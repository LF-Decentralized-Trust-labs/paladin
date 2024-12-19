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
	"math/big"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestAssembleInputsAnonEnc(t *testing.T) {
	inputs := commonWitnessInputs{}
	key := core.KeyEntry{}

	ctx := context.Background()
	privateInputs, err := assembleInputs_anon_enc(ctx, &inputs, nil, &key)
	assert.NoError(t, err)
	assert.Equal(t, 10, len(privateInputs))
}

func TestAssembleInputsAnonNullifier(t *testing.T) {
	inputs := commonWitnessInputs{
		inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(0)},
		inputValues:      []*big.Int{big.NewInt(3), big.NewInt(4), big.NewInt(5), big.NewInt(0)},
		inputSalts:       []*big.Int{big.NewInt(5), big.NewInt(6), big.NewInt(7), big.NewInt(0)},
	}
	privKey, pubKey, zkpKey := newKeypair()
	key := core.KeyEntry{
		PrivateKey:       privKey,
		PublicKey:        pubKey,
		PrivateKeyForZkp: zkpKey,
	}
	extras := proto.ProvingRequestExtras_Nullifiers{
		Root: "123",
	}
	ctx := context.Background()
	privateInputs, err := assembleInputs_anon_nullifier(ctx, &inputs, &extras, &key)
	assert.NoError(t, err)
	assert.Equal(t, 12, len(privateInputs))
	assert.Equal(t, "123", privateInputs["root"].(*big.Int).Text(16))
	assert.Len(t, privateInputs["nullifiers"], 4)
	assert.Equal(t, "0", privateInputs["nullifiers"].([]*big.Int)[3].Text(10))
}

func TestAssembleInputsAnonEnc_fail(t *testing.T) {
	inputs := commonWitnessInputs{}
	extras := proto.ProvingRequestExtras_Encryption{
		EncryptionNonce: "1234",
	}
	key := core.KeyEntry{}
	ctx := context.Background()
	privateInputs, err := assembleInputs_anon_enc(ctx, &inputs, &extras, &key)
	assert.NoError(t, err)
	assert.Equal(t, privateInputs["encryptionNonce"], new(big.Int).SetInt64(1234))

	extras.EncryptionNonce = "bad number"
	_, err = assembleInputs_anon_enc(ctx, &inputs, &extras, &key)
	assert.EqualError(t, err, "PD210077: Failed to parse encryption nonce")
}

func TestAssembleInputsAnonNullifier_fail(t *testing.T) {
	inputs := commonWitnessInputs{}
	extras := proto.ProvingRequestExtras_Nullifiers{
		Root: "123456",
		MerkleProofs: []*proto.MerkleProof{
			{
				Nodes: []string{"1", "2", "3"},
			},
			{
				Nodes: []string{"0", "0", "0"},
			},
		},
		Enabled: []bool{true, false},
	}
	key := core.KeyEntry{}
	ctx := context.Background()
	privateInputs, err := assembleInputs_anon_nullifier(ctx, &inputs, &extras, &key)
	assert.NoError(t, err)
	assert.Equal(t, "123456", privateInputs["root"].(*big.Int).Text(16))
	assert.Equal(t, "1", privateInputs["enabled"].([]*big.Int)[0].Text(10))
	assert.Equal(t, "0", privateInputs["enabled"].([]*big.Int)[1].Text(10))

	extras.Root = "bad number"
	_, err = assembleInputs_anon_nullifier(ctx, &inputs, &extras, &key)
	assert.EqualError(t, err, "PD210080: Failed to decode root value in extras")

	extras.Root = "123456"
	extras.MerkleProofs[0].Nodes = []string{"bad number"}
	_, err = assembleInputs_anon_nullifier(ctx, &inputs, &extras, &key)
	assert.EqualError(t, err, "PD210081: Failed to decode node in merkle proof in extras")

	inputs = commonWitnessInputs{
		inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
		inputValues:      []*big.Int{big.NewInt(3), big.NewInt(4)},
		inputSalts:       []*big.Int{big.NewInt(5), big.NewInt(6)},
	}
	privKey, pubKey, _ := newKeypair()
	tooBig, ok := new(big.Int).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	assert.True(t, ok)
	key = core.KeyEntry{
		PrivateKey:       privKey,
		PublicKey:        pubKey,
		PrivateKeyForZkp: tooBig,
	}
	_, err = assembleInputs_anon_nullifier(ctx, &inputs, &extras, &key)
	assert.EqualError(t, err, "PD210079: Failed to calculate nullifier. inputs values not inside Finite Field")
}

func TestAssembleInputsDeposit(t *testing.T) {
	inputs := commonWitnessInputs{outputCommitments: []*big.Int{big.NewInt(100)}}
	result := assembleInputs_deposit(&inputs)
	assert.Equal(t, "100", result["outputCommitments"].([]*big.Int)[0].Text(10))
}

func TestAssembleInputsWithdrawNullifier(t *testing.T) {
	inputs := commonWitnessInputs{
		inputCommitments:  []*big.Int{big.NewInt(100)},
		inputValues:       []*big.Int{big.NewInt(100)},
		inputSalts:        []*big.Int{big.NewInt(200)},
		outputCommitments: []*big.Int{big.NewInt(100)},
		outputValues:      []*big.Int{big.NewInt(200)},
		outputSalts:       []*big.Int{big.NewInt(300)},
	}
	privKey, pubKey, zkpKey := newKeypair()
	key := core.KeyEntry{
		PrivateKey:       privKey,
		PublicKey:        pubKey,
		PrivateKeyForZkp: zkpKey,
	}
	extras := proto.ProvingRequestExtras_Nullifiers{
		Root: "123",
		MerkleProofs: []*proto.MerkleProof{
			{
				Nodes: []string{"1", "2", "3"},
			},
			{
				Nodes: []string{"0", "0", "0"},
			},
		},
		Enabled: []bool{true, false},
	}
	ctx := context.Background()
	privateInputs, err := assembleInputs_withdraw_nullifier(ctx, &inputs, &extras, &key)
	assert.NoError(t, err)
	assert.Equal(t, "123", privateInputs["root"].(*big.Int).Text(16))
	assert.Len(t, privateInputs["nullifiers"], 1)
	assert.NotEqual(t, "0", privateInputs["nullifiers"].([]*big.Int)[0].Text(10))
}
