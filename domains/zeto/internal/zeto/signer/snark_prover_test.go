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
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	wtns "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/witness"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNewProver(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)
	assert.NotNil(t, prover.circuitsCache)
	assert.NotNil(t, prover.provingKeysCache)
}

func TestSnarkProve(t *testing.T) {
	prover := NewTestProver(t)

	alice := common.NewTestKeypair()
	bob := common.NewTestKeypair()

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

	alicePubKey := common.EncodeBabyJubJubPublicKey(alice.PublicKey)
	bobPubKey := common.EncodeBabyJubJubPublicKey(bob.PublicKey)

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)

	proof, err := prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	require.NoError(t, err)
	assert.Equal(t, 36, len(proof))
}

func TestConcurrentSnarkProofGeneration(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:         "test",
		ProvingKeysDir:      "test",
		MaxProverPerCircuit: confutil.P(50), // equal to the default cache size, so all provers can be cached at once
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	circuitLoadedTotal := 0
	circuitLoadedTotalMutex := &sync.Mutex{}

	peakProverCount := 0
	totalProvingRequestCount := 0
	peakProverCountMutex := &sync.Mutex{}

	testCircuitLoader := func(ctx context.Context, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		circuitLoadedTotalMutex.Lock()
		defer circuitLoadedTotalMutex.Unlock()
		circuitLoadedTotal++
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	testProofGenerator := func(ctx context.Context, witness []byte, provingKey []byte) (*types.ZKProof, error) {
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

	alice := common.NewTestKeypair()
	bob := common.NewTestKeypair()

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

	alicePubKey := common.EncodeBabyJubJubPublicKey(alice.PublicKey)
	bobPubKey := common.EncodeBabyJubJubPublicKey(bob.PublicKey)

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)
	expectReqCount := 500
	reqChan := make(chan struct{}, expectReqCount)

	for i := 0; i < expectReqCount; i++ {
		go func() {
			proof, err := prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
			require.NoError(t, err)
			assert.Equal(t, 36, len(proof))
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

func TestSnarkProveErrorCircuit(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	alice := common.NewTestKeypair()

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  []uint64{30, 40},
		OutputValues: []uint64{32, 38},
	})
	require.NoError(t, err)

	// leave the circuit ID empty
	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Type:           string(zetosignerapi.Withdraw),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputSalts:       []string{"salt1", "salt2"},
			InputOwner:       "alice/key0",
			OutputSalts:      []string{"salt1", "salt2"},
			OutputOwners:     []string{"bob", "alice"},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)

	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.ErrorContains(t, err, "circuit ID is required")
}

func TestSnarkProveErrorInputs(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	alice := common.NewTestKeypair()

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  []uint64{30, 40},
		OutputValues: []uint64{32, 38},
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputSalts:       []string{"salt1", "salt2"},
			OutputOwners:     []string{"bob", "alice"},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)
	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.ErrorContains(t, err, "anon.wasm: no such file or directory")
}

func TestSnarkProveErrorLoadcircuits(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	testCircuitLoader := func(ctx context.Context, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return nil, nil, fmt.Errorf("bang!")
	}
	prover.circuitLoader = testCircuitLoader

	alice := common.NewTestKeypair()
	bob := common.NewTestKeypair()

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

	alicePubKey := common.EncodeBabyJubJubPublicKey(alice.PublicKey)
	bobPubKey := common.EncodeBabyJubJubPublicKey(bob.PublicKey)

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)

	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.EqualError(t, err, "bang!")
}

func TestSnarkProveErrorGenerateProof(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	testCircuitLoader := func(ctx context.Context, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	alice := common.NewTestKeypair()

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

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputOwners:     []string{"badKey1", "badKey2"},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)

	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.ErrorContains(t, err, "witness is empty")
}

func TestSnarkProveErrorGenerateProof2(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	testCircuitLoader := func(ctx context.Context, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error) {
		return &testWitnessCalculator{}, []byte("proving key"), nil
	}
	prover.circuitLoader = testCircuitLoader

	alice := common.NewTestKeypair()
	bob := common.NewTestKeypair()

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

	alicePubKey := common.EncodeBabyJubJubPublicKey(alice.PublicKey)
	bobPubKey := common.EncodeBabyJubJubPublicKey(bob.PublicKey)

	tokenSecrets, err := json.Marshal(&pb.TokenSecrets_Fungible{
		InputValues:  inputValueInts,
		OutputValues: outputValueInts,
	})
	require.NoError(t, err)

	req := pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
			InputSalts:       inputSalts,
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err := proto.Marshal(&req)
	require.NoError(t, err)
	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.ErrorContains(t, err, "PD210084: Failed to parse input commitment")

	req = pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: false,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: inputCommitments,
			InputSalts:       []string{"salt1", "salt2"},
			InputOwner:       "alice/key0",
			OutputSalts:      []string{crypto.NewSalt().Text(16), crypto.NewSalt().Text(16)},
			OutputOwners:     []string{bobPubKey, alicePubKey},
			TokenSecrets:     tokenSecrets,
			TokenType:        pb.TokenType_fungible,
		},
	}
	payload, err = proto.Marshal(&req)
	require.NoError(t, err)
	_, err = prover.Sign(context.Background(), zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, alice.PrivateKey[:], payload)
	assert.ErrorContains(t, err, "PD210082: Failed to parse input salt")
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
		PubSignals: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"},
	}
	circuit := &zetosignerapi.Circuit{
		Name:           "anon_enc",
		Type:           zetosignerapi.Transfer,
		UsesNullifiers: false,
		UsesEncryption: true,
	}
	bytes, err := serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 118, len(bytes))

	circuit = &zetosignerapi.Circuit{
		Name:           "anon_nullifier",
		Type:           zetosignerapi.Transfer,
		UsesNullifiers: true,
		UsesEncryption: false,
	}
	bytes, err = serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 66, len(bytes))

	snark.PubSignals = []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3"}
	circuit = &zetosignerapi.Circuit{
		Name:           "anon_enc_batch",
		Type:           zetosignerapi.Transfer,
		UsesNullifiers: false,
		UsesEncryption: true,
	}
	bytes, err = serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 202, len(bytes))

	snark.PubSignals = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
	circuit = &zetosignerapi.Circuit{
		Name:           "anon_nullifier_batch",
		Type:           zetosignerapi.Transfer,
		UsesNullifiers: true,
		UsesEncryption: false,
	}
	bytes, err = serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 84, len(bytes))

	circuit = &zetosignerapi.Circuit{
		Name:           "withdraw_nullifier",
		Type:           zetosignerapi.Withdraw,
		UsesNullifiers: true,
		UsesEncryption: false,
	}
	bytes, err = serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 66, len(bytes))

	snark.PubSignals = []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"1", "2", "3"}
	circuit = &zetosignerapi.Circuit{
		Name:           "withdraw_nullifier_batch",
		Type:           zetosignerapi.Withdraw,
		UsesNullifiers: true,
		UsesEncryption: false,
	}
	bytes, err = serializeProofResponse(circuit, &snark)
	assert.NoError(t, err)
	assert.Equal(t, 85, len(bytes))
}

func TestZKPProverInvalidAlgos(t *testing.T) {
	ctx := context.Background()
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	_, err = prover.GetVerifier(ctx, "domain:zeto:unsupported", "", nil)
	assert.Regexp(t, "algorithm", err)

	_, err = prover.GetVerifier(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), "not_hex", nil)
	assert.Regexp(t, "verifier", err)

	_, err = prover.GetVerifier(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X, nil)
	assert.Regexp(t, "Invalid key", err)

	_, err = prover.Sign(ctx, "domain:zeto:unsupported", "", nil, nil)
	assert.Regexp(t, "algorithm", err)

	_, err = prover.Sign(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), "domain:zeto:unsupported", nil, nil)
	assert.Regexp(t, "payloadType", err)

	_, err = prover.GetMinimumKeyLen(ctx, "domain:zeto:unsupported")
	assert.Regexp(t, "algorithm", err)

	keyLen, err := prover.GetMinimumKeyLen(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"))
	require.NoError(t, err)
	assert.Equal(t, 32, keyLen)
}

func TestGetVerifier(t *testing.T) {
	ctx := context.Background()
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	keyBytes := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	publicKey, err := prover.GetVerifier(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_UNCOMPRESSED_0X, keyBytes)
	require.NoError(t, err)
	assert.Equal(t, publicKey, "0x1fb27be1c28984de1ff3e0592ee7454fa17d5f20561be3a97b6bc48c2b7e7e2,0x2279cb2bb680d8d118d742fa669150845659c76b90b7c3d7f8a470976beed219")
}

func TestGetCircuitId(t *testing.T) {
	inputs := &pb.ProvingRequest{
		Circuit: &pb.Circuit{
			Name:           "anon_enc",
			Type:           string(zetosignerapi.Transfer),
			UsesNullifiers: true,
			UsesEncryption: false,
		},
		Common: &pb.ProvingRequestCommon{
			InputCommitments: []string{"input1", "input2"},
		},
	}
	circuit := getCircuit(inputs)
	assert.Equal(t, "anon_enc", circuit.Name)

	inputs.Common.InputCommitments = []string{"input1", "input2", "input3"}
	circuit = getCircuit(inputs)
	assert.Equal(t, "anon_enc_batch", circuit.Name)
}

func TestGenerateProof(t *testing.T) {
	ctx := context.Background()

	t.Run("Error in proof generation", func(t *testing.T) {
		wtns := []byte("invalid witness")
		provingKey := []byte("invalid proving key")

		_, err := generateProof(ctx, wtns, provingKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PD210101")
	})
}

func TestNewWitnessInputs(t *testing.T) {
	tests := []struct {
		name        string
		tokenType   pb.TokenType
		circuit     *zetosignerapi.Circuit
		extras      interface{}
		expectType  interface{}
		expectErr   bool
		errContains string
	}{
		{
			name:       "Valid non-fungible witness inputs",
			tokenType:  pb.TokenType_nunFungible,
			circuit:    &zetosignerapi.Circuit{Name: "nf_anon", Type: zetosignerapi.Transfer},
			extras:     nil,
			expectType: &wtns.NonFungibleWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid non-fungible nullifier witness inputs",
			tokenType:  pb.TokenType_nunFungible,
			circuit:    &zetosignerapi.Circuit{Name: "nf_anon_nullifier", Type: zetosignerapi.Transfer, UsesNullifiers: true},
			extras:     nil,
			expectType: &wtns.NonFungibleWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid fungible encryption witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "anon_enc", Type: zetosignerapi.Transfer, UsesEncryption: true},
			extras:     &pb.ProvingRequestExtras_Encryption{},
			expectType: &wtns.FungibleEncWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid fungible encryption batch witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "anon_enc", Type: zetosignerapi.Transfer, UsesEncryption: true},
			extras:     &pb.ProvingRequestExtras_Encryption{},
			expectType: &wtns.FungibleEncWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid fungible nullifier witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "anon_nullifier", Type: zetosignerapi.Transfer, UsesNullifiers: true},
			extras:     &pb.ProvingRequestExtras_Nullifiers{},
			expectType: &wtns.FungibleNullifierWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid fungible nullifier batch witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "anon_nullifier", Type: zetosignerapi.Transfer, UsesNullifiers: true},
			extras:     &pb.ProvingRequestExtras_Nullifiers{},
			expectType: &wtns.FungibleNullifierWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid withdraw nullifier witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "withdraw_nullifier", Type: zetosignerapi.Withdraw, UsesNullifiers: true},
			extras:     &pb.ProvingRequestExtras_Nullifiers{},
			expectType: &wtns.WithdrawNullifierWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid withdraw nullifier batch witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "withdraw_nullifier", Type: zetosignerapi.Withdraw, UsesNullifiers: true},
			extras:     &pb.ProvingRequestExtras_Nullifiers{},
			expectType: &wtns.WithdrawNullifierWitnessInputs{},
			expectErr:  false,
		},
		{
			name:       "Valid withdraw deposit witness inputs",
			tokenType:  pb.TokenType_fungible,
			circuit:    &zetosignerapi.Circuit{Name: "deposit", Type: zetosignerapi.Deposit},
			expectType: &wtns.DepositWitnessInputs{},
			expectErr:  false,
		},
		{
			name:        "Invalid extras type for encryption circuit",
			tokenType:   pb.TokenType_fungible,
			circuit:     &zetosignerapi.Circuit{Name: "anon_enc", Type: zetosignerapi.Transfer, UsesEncryption: true},
			extras:      &pb.ProvingRequestExtras_Nullifiers{},
			expectType:  nil,
			expectErr:   true,
			errContains: "unexpected extras type for encryption circuit",
		},
		{
			name:        "Invalid extras type for nullifier circuit",
			tokenType:   pb.TokenType_fungible,
			circuit:     &zetosignerapi.Circuit{Name: "anon_nullifier", Type: zetosignerapi.Transfer, UsesNullifiers: true},
			extras:      &pb.ProvingRequestExtras_Encryption{},
			expectType:  nil,
			expectErr:   true,
			errContains: "unexpected extras type for anon nullifier circuit",
		},
		{
			name:        "Default fungible witness inputs",
			circuit:     &zetosignerapi.Circuit{Name: "unknown_circuit"},
			extras:      nil,
			expectType:  &wtns.FungibleWitnessInputs{},
			expectErr:   true,
			errContains: "unsupported circuit type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inputs, err := newWitnessInputs(tc.tokenType, tc.circuit, tc.extras)

			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tc.expectType, inputs)
			}
		})
	}
}
func TestCalculateWitness(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		circuit       *zetosignerapi.Circuit
		commonInputs  *pb.ProvingRequestCommon
		extras        interface{}
		keyEntry      *core.KeyEntry
		calculator    *testWitnessMock
		expectedError string
	}{
		{
			name:    "Successful witness calculation",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs: &pb.ProvingRequestCommon{
				InputCommitments: []string{"1", "2"},
				InputSalts:       []string{"3", "4"},
				OutputSalts:      []string{"5", "0"},
				OutputOwners:     []string{"3", "3"},
				TokenType:        pb.TokenType_fungible,
				TokenSecrets:     []byte(`{"inputValues":[10,20],"outputValues":[30,0]}`),
			},
			extras:     &pb.ProvingRequestExtras_Encryption{},
			keyEntry:   &core.KeyEntry{},
			calculator: &testWitnessMock{},
		},
		{
			name:          "Error in newWitnessInputs",
			circuit:       &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs:  &pb.ProvingRequestCommon{},
			extras:        &pb.ProvingRequestExtras_Nullifiers{},
			keyEntry:      &core.KeyEntry{},
			calculator:    &testWitnessMock{},
			expectedError: "unexpected extras type for encryption circuit",
		},
		{
			name:    "Error in validate inputs",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs: &pb.ProvingRequestCommon{
				InputCommitments: []string{"input1"},
				InputSalts:       []string{"salt1", "salt2"},
			},
			extras:        &pb.ProvingRequestExtras_Encryption{},
			keyEntry:      &core.KeyEntry{},
			calculator:    &testWitnessMock{validateError: true},
			expectedError: "validate error",
		},
		{
			name:    "Error in build inputs",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs: &pb.ProvingRequestCommon{
				InputCommitments: []string{"input1", "input2"},
				InputSalts:       []string{"salt1", "salt2"},
			},
			extras:        &pb.ProvingRequestExtras_Encryption{},
			keyEntry:      &core.KeyEntry{},
			calculator:    &testWitnessMock{buildError: true},
			expectedError: "build error",
		},
		{
			name:    "Error in assemble inputs",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs: &pb.ProvingRequestCommon{
				InputCommitments: []string{"input1", "input2"},
				InputSalts:       []string{"salt1", "salt2"},
			},
			extras:        &pb.ProvingRequestExtras_Encryption{},
			keyEntry:      &core.KeyEntry{},
			calculator:    &testWitnessMock{assembleError: true},
			expectedError: "assemble error",
		},
		{
			name:    "Error in CalculateWTNSBin",
			circuit: &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true},
			commonInputs: &pb.ProvingRequestCommon{
				InputCommitments: []string{"input1", "input2"},
				InputSalts:       []string{"salt1", "salt2"},
			},
			extras:        &pb.ProvingRequestExtras_Encryption{},
			keyEntry:      &core.KeyEntry{},
			calculator:    &testWitnessMock{calculateWTNSError: true},
			expectedError: "calculate WTNSBin error",
		},
	}

	tmpGetWitnessInputs := getWitnessInputs
	defer func() { getWitnessInputs = tmpGetWitnessInputs }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			getWitnessInputs = func(_ pb.TokenType, _ *zetosignerapi.Circuit, _ interface{}) (witnessInputs, error) {
				if tt.name == "Error in newWitnessInputs" {
					return nil, fmt.Errorf("unexpected extras type for encryption circuit")
				}
				return tt.calculator, nil
			}

			wtns, err := calculateWitness(ctx, tt.circuit, tt.commonInputs, tt.extras, tt.keyEntry, tt.calculator)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, wtns)
			}
		})
	}
}

func TestNewSnarkProver(t *testing.T) {
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := NewSnarkProver(config)
	require.NoError(t, err)
	assert.NotNil(t, prover)
}

func TestSnarkProverSign(t *testing.T) {
	ctx := context.Background()
	config := &zetosignerapi.SnarkProverConfig{
		CircuitsDir:    "test",
		ProvingKeysDir: "test",
	}
	prover, err := newSnarkProver(config)
	require.NoError(t, err)

	t.Run("Invalid algorithm", func(t *testing.T) {
		_, err := prover.Sign(ctx, "invalid_algorithm", zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, nil, nil)
		assert.ErrorContains(t, err, "PD210088")
	})

	t.Run("Invalid payload type", func(t *testing.T) {
		_, err := prover.Sign(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), "invalid_payload_type", nil, nil)
		assert.ErrorContains(t, err, "PD210090")
	})

	t.Run("Missing circuit ID", func(t *testing.T) {
		payload, err := proto.Marshal(&pb.ProvingRequest{})
		require.NoError(t, err)
		_, err = prover.Sign(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, nil, payload)
		assert.ErrorContains(t, err, "PD210124")
	})

	t.Run("Context cancelled", func(t *testing.T) {
		circuit := &zetosignerapi.Circuit{Name: "anon_enc", UsesEncryption: true}
		payload, err := proto.Marshal(&pb.ProvingRequest{Circuit: circuit.ToProto()})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(ctx)
		cancel()

		_, err = prover.Sign(ctx, zetosignerapi.AlgoDomainZetoSnarkBJJ("zeto"), zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK, nil, payload)
		assert.ErrorContains(t, err, "PD210124")
	})
}

var _ witness.Calculator = &testWitnessMock{}
var _ witnessInputs = &testWitnessMock{}

type testWitnessMock struct {
	buildError               bool
	validateError            bool
	assembleError            bool
	calculateWTNSError       bool
	calculateWitnessError    bool
	calculateBinWitnessError bool
}

func (twc *testWitnessMock) CalculateWitness(inputs map[string]interface{}, sanityCheck bool) ([]*big.Int, error) {
	if twc.calculateWitnessError {
		return nil, fmt.Errorf("calculate witness error")
	}
	return []*big.Int{}, nil
}

func (twc *testWitnessMock) CalculateBinWitness(inputs map[string]interface{}, sanityCheck bool) ([]byte, error) {
	if twc.calculateBinWitnessError {
		return nil, fmt.Errorf("calculate BinWitness error")
	}
	return []byte{}, nil
}
func (twc *testWitnessMock) CalculateWTNSBin(inputs map[string]interface{}, sanityCheck bool) ([]byte, error) {
	if twc.calculateWTNSError {
		return nil, fmt.Errorf("calculate WTNSBin error")
	}
	return []byte("witness"), nil
}

func (twc *testWitnessMock) Validate(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {
	if twc.validateError {
		return fmt.Errorf("validate error")
	}
	return nil
}

func (twc *testWitnessMock) Build(ctx context.Context, commonInputs *pb.ProvingRequestCommon) error {
	if twc.buildError {
		return fmt.Errorf("build error")
	}
	return nil
}

func (twc *testWitnessMock) Assemble(ctx context.Context, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	if twc.assembleError {
		return nil, fmt.Errorf("assemble error")
	}
	return map[string]interface{}{"key": "value"}, nil
}
