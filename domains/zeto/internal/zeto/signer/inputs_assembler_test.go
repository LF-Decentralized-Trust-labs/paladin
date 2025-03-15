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
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestAssembleFungibleWitnessInputs(t *testing.T) {
	inputs := fungibleWitnessInputs{
		commonWitnessInputs: commonWitnessInputs{
			inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
			inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
		},
		inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
		outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Equal(t, key.PrivateKeyForZkp, result["inputOwnerPrivateKey"])
}

func TestAssembleDepositWitnessInputs(t *testing.T) {
	inputs := depositWitnessInputs{
		fungibleWitnessInputs: fungibleWitnessInputs{
			commonWitnessInputs: commonWitnessInputs{
				inputCommitments:      []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:            []*big.Int{big.NewInt(3), big.NewInt(4)},
				outputCommitments:     []*big.Int{big.NewInt(9), big.NewInt(10)},
				outputSalts:           []*big.Int{big.NewInt(11), big.NewInt(12)},
				outputOwnerPublicKeys: [][]*big.Int{},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.NotContains(t, result, "inputCommitments")
	assert.NotContains(t, result, "inputSalts")
	assert.NotContains(t, result, "inputValues")
	assert.NotContains(t, result, "inputOwnerPrivateKey")
	assert.Equal(t, inputs.outputCommitments, result["outputCommitments"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Equal(t, inputs.outputSalts, result["outputSalts"])
	assert.Equal(t, inputs.outputOwnerPublicKeys, result["outputOwnerPublicKeys"])
}

func TestAssembleNonFungibleWitnessInputs(t *testing.T) {
	inputs := nonFungibleWitnessInputs{
		commonWitnessInputs: commonWitnessInputs{
			inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
			inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
		},
		tokenIDs:  []*big.Int{big.NewInt(5), big.NewInt(6)},
		tokenURIs: []*big.Int{big.NewInt(7), big.NewInt(8)},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.tokenIDs, result["tokenIds"])
	assert.Equal(t, inputs.tokenURIs, result["tokenUris"])
	assert.Equal(t, key.PrivateKeyForZkp, result["inputOwnerPrivateKey"])
}

func TestAssembleFungibleEncWitnessInputs(t *testing.T) {
	inputs := fungibleEncWitnessInputs{
		fungibleWitnessInputs: fungibleWitnessInputs{
			commonWitnessInputs: commonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Contains(t, result, "encryptionNonce")
	assert.Contains(t, result, "ecdhPrivateKey")
}

func TestAssembleFungibleNullifierWitnessInputs(t *testing.T) {
	ras := &pb.ProvingRequestExtras_Nullifiers{
		Root: "123456",
		MerkleProofs: []*pb.MerkleProof{
			{
				Nodes: []string{"1", "2", "3"},
			},
			{
				Nodes: []string{"0", "0", "0"},
			},
		},
		Enabled: []bool{true, false},
	}
	inputs := fungibleNullifierWitnessInputs{
		nul: ras,
		fungibleWitnessInputs: fungibleWitnessInputs{

			commonWitnessInputs: commonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["inputCommitments"])
	assert.Equal(t, inputs.inputSalts, result["inputSalts"])
	assert.Equal(t, inputs.inputValues, result["inputValues"])
	assert.Equal(t, inputs.outputValues, result["outputValues"])
	assert.Contains(t, result, "nullifiers")
	assert.Contains(t, result, "root")
	assert.Contains(t, result, "merkleProof")
	assert.Contains(t, result, "enabled")
}
func TestPrepareInputsForNullifiers(t *testing.T) {
	ras := &pb.ProvingRequestExtras_Nullifiers{
		Root: "123456",
		MerkleProofs: []*pb.MerkleProof{
			{
				Nodes: []string{"1", "2", "3"},
			},
			{
				Nodes: []string{"0", "0", "0"},
			},
		},
		Enabled: []bool{true, false},
	}
	inputs := fungibleNullifierWitnessInputs{
		nul: ras,
		fungibleWitnessInputs: fungibleWitnessInputs{
			commonWitnessInputs: commonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues:  []*big.Int{big.NewInt(5), big.NewInt(6)},
			outputValues: []*big.Int{big.NewInt(7), big.NewInt(8)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	nullifiers, root, proofs, enabled, err := inputs.prepareInputsForNullifiers(ctx, ras, &key)
	assert.NoError(t, err)

	a, _ := new(big.Int).SetString("11227379794363607928937567982576521385233423371965312580610589067804815929531", 0)
	b, _ := new(big.Int).SetString("18511661912667214378913629034224397005995848268976364384592575521219418327022", 0)

	assert.Equal(t, []*big.Int{a, b}, nullifiers)
	assert.Equal(t, big.NewInt(0x123456), root)
	assert.Equal(t, [][]*big.Int{
		{big.NewInt(1), big.NewInt(2), big.NewInt(3)},
		{big.NewInt(0), big.NewInt(0), big.NewInt(0)},
	}, proofs)
	assert.Equal(t, []*big.Int{big.NewInt(1), big.NewInt(0)}, enabled)
}
func TestAssembleLockWitnessInputs(t *testing.T) {
	inputs := lockWitnessInputs{
		fungibleWitnessInputs: fungibleWitnessInputs{
			commonWitnessInputs: commonWitnessInputs{
				inputCommitments: []*big.Int{big.NewInt(1), big.NewInt(2)},
				inputSalts:       []*big.Int{big.NewInt(3), big.NewInt(4)},
			},
			inputValues: []*big.Int{big.NewInt(5), big.NewInt(6)},
		},
	}
	key := core.KeyEntry{
		PrivateKeyForZkp: big.NewInt(123456789),
	}
	ctx := context.Background()
	result, err := inputs.assemble(ctx, &key)
	assert.NoError(t, err)
	assert.Equal(t, inputs.inputCommitments, result["commitments"])
	assert.Equal(t, inputs.inputValues, result["values"])
	assert.Equal(t, inputs.inputSalts, result["salts"])
	assert.Equal(t, key.PrivateKeyForZkp, result["ownerPrivateKey"])
}
