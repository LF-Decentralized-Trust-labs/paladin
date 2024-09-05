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
	"errors"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/kaleido-io/paladin/core/internal/cache"
	pb "github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/api"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"google.golang.org/protobuf/proto"
)

// snarkProver encapsulates the logic for generating SNARK proofs
type snarkProver struct {
	zkpProverConfig  api.SnarkProverConfig
	circuitsCache    cache.Cache[string, witness.Calculator]
	provingKeysCache cache.Cache[string, []byte]
	circuitLoader    func(circuitID string, config api.SnarkProverConfig) (witness.Calculator, []byte, error)
	proofGenerator   func(witness []byte, provingKey []byte) (*types.ZKProof, error)
}

func Register(ctx context.Context, config api.SnarkProverConfig, registry map[string]api.InMemorySigner) error {
	// skip registration is no ZKP prover config is provided
	if config.CircuitsDir == "" || config.ProvingKeysDir == "" {
		log.L(ctx).Info("zkp prover not configured, skip registering as an in-memory signer")
		return nil
	}

	signer, err := newSnarkProver(config)
	if err != nil {
		return err
	}
	registry[algorithms.ZKP_BABYJUBJUB_PLAINBYTES] = signer
	return nil
}

func newSnarkProver(config api.SnarkProverConfig) (*snarkProver, error) {
	cacheConfig := cache.Config{
		Capacity: confutil.P(5),
	}
	return &snarkProver{
		zkpProverConfig:  config,
		circuitsCache:    cache.NewCache[string, witness.Calculator](&cacheConfig, &cacheConfig),
		provingKeysCache: cache.NewCache[string, []byte](&cacheConfig, &cacheConfig),
		circuitLoader:    loadCircuit,
		proofGenerator:   generateProof,
	}, nil
}

func (sp *snarkProver) Sign(ctx context.Context, privateKey []byte, req *pb.SignRequest) (*pb.SignResponse, error) {
	keyBytes := [32]byte{}
	copy(keyBytes[:], privateKey)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)

	inputs := pb.ProvingRequest{}
	// Unmarshal payload into inputs
	err := proto.Unmarshal(req.Payload, &inputs)
	if err != nil {
		return nil, err
	}
	// Perform proof generation
	if inputs.CircuitId == "" {
		return nil, errors.New("circuit ID is required")
	}
	if err := validateInputs(inputs.Common); err != nil {
		return nil, err
	}

	// Perform proof generation
	circuit, _ := sp.circuitsCache.Get(inputs.CircuitId)
	provingKey, _ := sp.provingKeysCache.Get(inputs.CircuitId)
	if circuit == nil || provingKey == nil {
		c, p, err := sp.circuitLoader(inputs.CircuitId, sp.zkpProverConfig)
		if err != nil {
			return nil, err
		}
		sp.circuitsCache.Set(inputs.CircuitId, c)
		sp.provingKeysCache.Set(inputs.CircuitId, p)
		circuit = c
		provingKey = p
	}

	wtns, err := calculateWitness(inputs.CircuitId, inputs.Common, keyEntry, circuit)
	if err != nil {
		return nil, err
	}

	proof, err := sp.proofGenerator(wtns, provingKey)
	if err != nil {
		return nil, err
	}

	proofBytes, err := serializeProof(proof)
	if err != nil {
		return nil, err
	}

	return &pb.SignResponse{
		Payload: proofBytes,
	}, nil
}

func validateInputs(inputs *pb.ProvingRequestCommon) error {
	if len(inputs.InputCommitments) == 0 {
		return errors.New("input commitments are required")
	}
	if len(inputs.InputValues) == 0 {
		return errors.New("input values are required")
	}
	if len(inputs.InputSalts) == 0 {
		return errors.New("input salts are required")
	}
	if len(inputs.InputCommitments) != len(inputs.InputValues) || len(inputs.InputCommitments) != len(inputs.InputSalts) {
		return errors.New("input commitments, values, and salts must have the same length")
	}
	if len(inputs.OutputValues) == 0 {
		return errors.New("output values are required")
	}
	if len(inputs.OutputOwners) == 0 {
		return errors.New("output owner keys are required")
	}
	if len(inputs.OutputValues) != len(inputs.OutputOwners) {
		return errors.New("output values and owner keys must have the same length")
	}
	return nil
}

func serializeProof(proof *types.ZKProof) ([]byte, error) {
	snark := pb.SnarkProof{}
	snark.A = proof.Proof.A
	snark.B = make([]*pb.B_Item, 0, len(proof.Proof.B))
	for _, p := range proof.Proof.B {
		bItems := pb.B_Item{}
		bItems.Items = append(bItems.Items, p...)
		snark.B = append(snark.B, &bItems)
	}
	snark.C = proof.Proof.C

	return proto.Marshal(&snark)
}

func calculateWitness(circuitId string, commonInputs *pb.ProvingRequestCommon, keyEntry *core.KeyEntry, circuit witness.Calculator) ([]byte, error) {
	inputs, err := buildCircuitInputs(commonInputs)
	if err != nil {
		return nil, err
	}

	var witnessInputs map[string]interface{}
	switch circuitId {
	case "anon":
		witnessInputs = assembleInputs_anon(inputs, keyEntry)
	}

	wtns, err := circuit.CalculateWTNSBin(witnessInputs, true)
	if err != nil {
		return nil, err
	}

	return wtns, nil
}

func generateProof(wtns, provingKey []byte) (*types.ZKProof, error) {
	proof, err := prover.Groth16Prover(provingKey, wtns)
	if err != nil {
		return nil, err
	}
	return proof, nil
}
