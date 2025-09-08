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

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"

	"github.com/stretchr/testify/require"
)

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

func NewTestProver(t *testing.T) signerapi.InMemorySigner {
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

	testProofGenerator := func(ctx context.Context, witness []byte, provingKey []byte) (*types.ZKProof, error) {
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

	return prover
}
