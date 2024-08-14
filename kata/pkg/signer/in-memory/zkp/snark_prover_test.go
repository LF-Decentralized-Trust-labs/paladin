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

package zkp

import (
	"context"
	"math/big"
	"testing"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/common"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestRegister(t *testing.T) {
	registry := make(map[string]api.InMemorySigner)
	config := api.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	err := Register(context.Background(), config, registry)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(registry))
}

func TestNewProver(t *testing.T) {
	config := api.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)
	assert.NotNil(t, prover.circuitsCache)
	assert.NotNil(t, prover.provingKeysCache)
}

type testWitnessCalculator struct{}

func (t *testWitnessCalculator) CalculateWTNSBin(inputs map[string]interface{}, _ bool) ([]byte, error) {
	return []byte{}, nil
}
func (t *testWitnessCalculator) CalculateWitness(inputs map[string]interface{}, sanityCheck bool) ([]*big.Int, error) {
	return []*big.Int{}, nil
}
func (t *testWitnessCalculator) CalculateBinWitness(inputs map[string]interface{}, sanityCheck bool) ([]byte, error) {
	return []byte{}, nil
}

func TestSnarkProve(t *testing.T) {
	config := api.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	testCircuitLoader := func(circuitID string, config api.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	testProofGenerator := func(witness []byte, provingKey []byte) (*types.ZKProof, error) {
		return &types.ZKProof{
			Proof: &types.ProofData{
				A:        []string{"a"},
				B:        [][]string{{"b1.1", "b1.2"}, {"b2.1", "b2.2"}},
				C:        []string{"c"},
				Protocol: "super snark",
			},
		}, nil
	}
	prover.proofGenerator = testProofGenerator

	alice := NewKeypair()
	bob := NewKeypair()

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := utxo.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := utxo.NewSalt()
	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
	inputCommitments := []string{input1.Text(16), input2.Text(16)}

	inputValueInts := []uint64{inputValues[0].Uint64(), inputValues[1].Uint64()}
	inputSalts := []string{salt1.Text(16), salt2.Text(16)}
	outputValueInts := []uint64{outputValues[0].Uint64(), outputValues[1].Uint64()}

	alicePubKey := common.EncodePublicKey(alice.PublicKey)
	bobPubKey := common.EncodePublicKey(bob.PublicKey)

	req := pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputValues:     outputValueInts,
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	res, err := prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: api.Algorithm_ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.NoError(t, err)
	assert.Equal(t, 38, len(res.Payload))
}
