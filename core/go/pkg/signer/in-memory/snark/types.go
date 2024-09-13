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
	"errors"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	pb "github.com/kaleido-io/paladin/core/pkg/proto"
	"github.com/kaleido-io/paladin/core/pkg/signer/common"
)

type commonWitnessInputs struct {
	inputCommitments      []*big.Int
	inputValues           []*big.Int
	inputSalts            []*big.Int
	outputCommitments     []*big.Int
	outputValues          []*big.Int
	outputSalts           []*big.Int
	outputOwnerPublicKeys [][]*big.Int
}

func buildCircuitInputs(commonInputs *pb.ProvingRequestCommon) (*commonWitnessInputs, error) {
	// construct the output UTXOs based on the values and owner public keys
	outputCommitments := make([]*big.Int, len(commonInputs.OutputValues))
	outputSalts := make([]*big.Int, len(commonInputs.OutputValues))
	outputOwnerPublicKeys := make([][]*big.Int, len(commonInputs.OutputValues))
	outputValues := make([]*big.Int, len(commonInputs.OutputValues))

	for i := 0; i < len(commonInputs.OutputSalts); i++ {
		salt, ok := new(big.Int).SetString(commonInputs.OutputSalts[i], 16)
		if !ok {
			return nil, errors.New("failed to parse output salt")
		}
		outputSalts[i] = salt

		if salt.Cmp(big.NewInt(0)) == 0 {
			outputOwnerPublicKeys[i] = []*big.Int{big.NewInt(0), big.NewInt(0)}
			outputValues[i] = big.NewInt(0)
			outputCommitments[i] = big.NewInt(0)
		} else {
			ownerPubKey, err := common.DecodePublicKey(commonInputs.OutputOwners[i])
			if err != nil {
				return nil, err
			}
			outputOwnerPublicKeys[i] = []*big.Int{ownerPubKey.X, ownerPubKey.Y}
			value := commonInputs.OutputValues[i]
			outputValues[i] = new(big.Int).SetUint64(value)
			u := utxo.NewFungible(new(big.Int).SetUint64(value), ownerPubKey, salt)
			hash, err := u.GetHash()
			if err != nil {
				return nil, err
			}
			outputCommitments[i] = hash
		}
	}

	inputCommitments := make([]*big.Int, len(commonInputs.InputCommitments))
	inputValues := make([]*big.Int, len(commonInputs.InputValues))
	inputSalts := make([]*big.Int, len(commonInputs.InputSalts))
	for i, c := range commonInputs.InputCommitments {
		commitment, ok := new(big.Int).SetString(c, 16)
		if !ok {
			return nil, errors.New("failed to parse input commitment")
		}
		inputCommitments[i] = commitment
		inputValues[i] = new(big.Int).SetUint64(commonInputs.InputValues[i])
		salt, ok := new(big.Int).SetString(commonInputs.InputSalts[i], 16)
		if !ok {
			return nil, errors.New("failed to parse input salt")
		}
		inputSalts[i] = salt
	}
	return &commonWitnessInputs{
		inputCommitments:      inputCommitments,
		inputValues:           inputValues,
		inputSalts:            inputSalts,
		outputCommitments:     outputCommitments,
		outputValues:          outputValues,
		outputSalts:           outputSalts,
		outputOwnerPublicKeys: outputOwnerPublicKeys,
	}, nil
}
