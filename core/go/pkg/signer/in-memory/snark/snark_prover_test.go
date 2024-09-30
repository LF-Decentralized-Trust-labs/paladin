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

package snark

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	pb "github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/common"
	"github.com/kaleido-io/paladin/core/pkg/signer/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

type User struct {
	PrivateKey       *babyjub.PrivateKey
	PublicKey        *babyjub.PublicKey
	PrivateKeyBigInt *big.Int
}

func NewKeypair() *User {
	babyJubjubPrivKey := babyjub.NewRandPrivKey()
	babyJubjubPubKey := babyJubjubPrivKey.Public()
	// convert the private key to big.Int for use inside circuits
	privKeyBigInt := babyjub.SkToBigInt(&babyJubjubPrivKey)

	return &User{
		PrivateKey:       &babyJubjubPrivKey,
		PublicKey:        babyJubjubPubKey,
		PrivateKeyBigInt: privKeyBigInt,
	}
}

func TestRegister(t *testing.T) {
	registry := make(map[string]signerapi.InMemorySigner)
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	err := Register(context.Background(), config, registry)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(registry))
}

func TestNewProver(t *testing.T) {
	config := signerapi.SnarkProverConfig{
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
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	testCircuitLoader := func(circuitID string, config signerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
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

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
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
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	res, err := prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.NoError(t, err)
	assert.Equal(t, 36, len(res.Payload))
}

func TestConcurrentSnarkProofGeneration(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:         "test",
		ProvingKeysDir:      "test",
		MaxProverPerCircuit: confutil.P(50), // equal to the default cache size, so all provers can be cached at once
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	circuitLoadedTotal := 0
	circuitLoadedTotalMutex := &sync.Mutex{}

	peakProverCount := 0
	totalProvingRequestCount := 0
	peakProverCountMutex := &sync.Mutex{}

	testCircuitLoader := func(circuitID string, config signerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		circuitLoadedTotalMutex.Lock()
		defer circuitLoadedTotalMutex.Unlock()
		circuitLoadedTotal++
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	testProofGenerator := func(witness []byte, provingKey []byte) (*types.ZKProof, error) {
		peakProverCountMutex.Lock()
		peakProverCount++
		assert.LessOrEqual(t, peakProverCount, 50) // ensure the peak prover count is smaller than the default max
		peakProverCountMutex.Unlock()
		defer func() {
			peakProverCountMutex.Lock()
			peakProverCount--
			totalProvingRequestCount++
			peakProverCountMutex.Unlock()
		}()
		time.Sleep(100 * time.Millisecond) // simulate delay

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

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
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
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)
	expectReqCount := 500
	reqChan := make(chan struct{}, expectReqCount)

	for i := 0; i < expectReqCount; i++ {
		go func() {
			res, err := prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
				KeyHandle: "key1",
				Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
				Payload:   payload,
			})
			assert.NoError(t, err)
			assert.Equal(t, 36, len(res.Payload))
			reqChan <- struct{}{}
		}()
	}
	resCount := 0
	for {
		<-reqChan
		resCount++
		if resCount == expectReqCount {
			assert.Equal(t, expectReqCount, totalProvingRequestCount) // check all proving requests has been processed
			assert.Equal(t, 50, circuitLoadedTotal)                   // check cache works as expected, loaded circuit 50 times for 500 proving requests
			break
		}
	}
}

func TestSnarkProveError(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	alice := NewKeypair()

	// construct a bad payload by using the inner object
	req := pb.ProvingRequestCommon{
		InputCommitments: []string{"input1", "input2"},
		InputValues:      []uint64{30, 40},
		InputSalts:       []string{"salt1", "salt2"},
		InputOwner:       "alice/key0",
		OutputValues:     []uint64{32, 38},
		OutputOwners:     []string{"bob", "alice"},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "cannot parse invalid wire-format data")
}

func TestSnarkProveErrorCircuit(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	alice := NewKeypair()

	// leave the circuit ID empty
	req := pb.ProvingRequest{
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputValues:      []uint64{30, 40},
			InputSalts:       []string{"salt1", "salt2"},
			InputOwner:       "alice/key0",
			OutputValues:     []uint64{32, 38},
			OutputSalts:      []string{"salt1", "salt2"},
			OutputOwners:     []string{"bob", "alice"},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "circuit ID is required")
}

func TestSnarkProveErrorInputs(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	alice := NewKeypair()

	req := pb.ProvingRequest{
		CircuitId: "anon",
		Common:    &pb.ProvingRequestCommon{},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "input commitments are required")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "input values are required")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputValues:      []uint64{30, 40},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "input salts are required")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputValues:      []uint64{30, 40},
			InputSalts:       []string{"salt1", "salt2"},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "output values are required")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputValues:      []uint64{30, 40},
			InputSalts:       []string{"salt1", "salt2"},
			OutputValues:     []uint64{32, 38},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "output owner keys are required")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputValues:      []uint64{30, 40},
			InputSalts:       []string{"salt1", "salt2"},
			OutputValues:     []uint64{32, 38},
			OutputOwners:     []string{"bob", "alice"},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "anon.wasm: no such file or directory")
}

func TestSnarkProveErrorLoadcircuits(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	testCircuitLoader := func(circuitID string, config signerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return nil, nil, fmt.Errorf("bang!")
	}
	prover.circuitLoader = testCircuitLoader

	alice := NewKeypair()
	bob := NewKeypair()

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
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

	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.EqualError(t, err, "bang!")
}

func TestSnarkProveErrorGenerateProof(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	testCircuitLoader := func(circuitID string, config signerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	alice := NewKeypair()

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
	input2, _ := poseidon.Hash([]*big.Int{inputValues[1], salt2, alice.PublicKey.X, alice.PublicKey.Y})
	inputCommitments := []string{input1.Text(16), input2.Text(16)}

	inputValueInts := []uint64{inputValues[0].Uint64(), inputValues[1].Uint64()}
	inputSalts := []string{salt1.Text(16), salt2.Text(16)}
	outputValueInts := []uint64{outputValues[0].Uint64(), outputValues[1].Uint64()}

	req := pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputValues:     outputValueInts,
			OutputOwners:     []string{"badKey1", "badKey2"},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)

	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "witness is empty")
}

func TestSnarkProveErrorGenerateProof2(t *testing.T) {
	config := signerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	assert.NoError(t, err)

	testCircuitLoader := func(circuitID string, config signerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	alice := NewKeypair()
	bob := NewKeypair()

	inputValues := []*big.Int{big.NewInt(30), big.NewInt(40)}
	outputValues := []*big.Int{big.NewInt(32), big.NewInt(38)}

	salt1 := crypto.NewSalt()
	input1, _ := poseidon.Hash([]*big.Int{inputValues[0], salt1, alice.PublicKey.X, alice.PublicKey.Y})
	salt2 := crypto.NewSalt()
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
			InputCommitments: []string{"input1", "input2"},
			InputValues:      inputValueInts,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputValues:     outputValueInts,
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err := proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "failed to parse input commitment")

	req = pb.ProvingRequest{
		CircuitId: "anon",
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputValues:      inputValueInts,
			InputSalts:       []string{"salt1", "salt2"},
			InputOwner:       "alice/key0",
			OutputValues:     outputValueInts,
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
		},
	}
	payload, err = proto.Marshal(&req)
	assert.NoError(t, err)
	_, err = prover.Sign(context.Background(), alice.PrivateKey[:], &pb.SignRequest{
		KeyHandle: "key1",
		Algorithm: algorithms.ZKP_BABYJUBJUB_PLAINBYTES,
		Payload:   payload,
	})
	assert.ErrorContains(t, err, "failed to parse input salt")
}

func TestValidateInputs(t *testing.T) {
	inputs1 := &pb.ProvingRequestCommon{
		InputCommitments: []string{"input1", "input2"},
		InputValues:      []uint64{30},
		InputSalts:       []string{"salt1", "salt2"},
	}
	err := validateInputs(inputs1)
	assert.ErrorContains(t, err, "input commitments, values, and salts must have the same length")

	inputs2 := &pb.ProvingRequestCommon{
		InputCommitments: []string{"input1", "input2"},
		InputValues:      []uint64{30, 40},
		InputSalts:       []string{"salt1"},
	}
	err = validateInputs(inputs2)
	assert.ErrorContains(t, err, "input commitments, values, and salts must have the same length")

	inputs3 := &pb.ProvingRequestCommon{
		InputCommitments: []string{"input1", "input2"},
		InputValues:      []uint64{30, 40},
		InputSalts:       []string{"salt1", "salt2"},
		OutputValues:     []uint64{32, 38},
		OutputOwners:     []string{"bob"},
	}
	err = validateInputs(inputs3)
	assert.ErrorContains(t, err, "output values and owner keys must have the same length")
}

func TestSerializeProofResponse(t *testing.T) {
	snark := types.ZKProof{
		Proof: &types.ProofData{
			A: []string{"a"},
			B: [][]string{
				{"b1.1", "b1.2"},
				{"b2.1", "b2.2"},
			},
			C: []string{"c"},
		},
		PubSignals: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"},
	}
	bytes, err := serializeProofResponse("anon_enc", &snark)
	assert.NoError(t, err)
	assert.Equal(t, 86, len(bytes))

	bytes, err = serializeProofResponse("anon_nullifier", &snark)
	assert.NoError(t, err)
	assert.Equal(t, 66, len(bytes))
}
