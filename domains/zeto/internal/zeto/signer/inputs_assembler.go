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
	"crypto/rand"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/crypto"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
)

func assembleInputs_anon(inputs *commonWitnessInputs, keyEntry *core.KeyEntry) map[string]interface{} {
	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs
}

func assembleInputs_anon_enc(ctx context.Context, inputs *commonWitnessInputs, extras *pb.ProvingRequestExtras_Encryption, keyEntry *core.KeyEntry) (map[string]any, error) {
	var nonce *big.Int
	if extras != nil && extras.EncryptionNonce != "" {
		n, ok := new(big.Int).SetString(extras.EncryptionNonce, 10)
		if !ok {
			return nil, i18n.NewError(ctx, msgs.MsgErrorParseEncNonce)
		}
		nonce = n
	} else {
		nonce = crypto.NewEncryptionNonce()
	}
	// TODO: right now we generate the ephemeral key pair and throw away the private key,
	// need more thought on if more management of the key is needed
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic(i18n.NewError(ctx, msgs.MsgErrorGenerateRandBytes, err))
	}
	ephemeralKey := key.NewKeyEntryFromPrivateKeyBytes([32]byte(randomBytes))

	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
		"encryptionNonce":       nonce,
		"ecdhPrivateKey":        ephemeralKey.PrivateKeyForZkp,
	}
	return witnessInputs, nil
}

func assembleInputs_anon_nullifier(ctx context.Context, inputs *commonWitnessInputs, extras *pb.ProvingRequestExtras_Nullifiers, keyEntry *core.KeyEntry) (map[string]any, error) {
	nullifiers, root, proofs, enabled, err := prepareInputsForNullifiers(ctx, inputs, extras, keyEntry)
	if err != nil {
		return nil, err
	}

	witnessInputs := map[string]interface{}{
		"nullifiers":            nullifiers,
		"root":                  root,
		"merkleProof":           proofs,
		"enabled":               enabled,
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs, nil
}

func assembleInputs_deposit(inputs *commonWitnessInputs) map[string]interface{} {
	witnessInputs := map[string]interface{}{
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs
}

func assembleInputs_withdraw(inputs *commonWitnessInputs, keyEntry *core.KeyEntry) map[string]interface{} {
	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs
}

func assembleInputs_withdraw_nullifier(ctx context.Context, inputs *commonWitnessInputs, extras *pb.ProvingRequestExtras_Nullifiers, keyEntry *core.KeyEntry) (map[string]interface{}, error) {
	nullifiers, root, proofs, enabled, err := prepareInputsForNullifiers(ctx, inputs, extras, keyEntry)
	if err != nil {
		return nil, err
	}

	witnessInputs := map[string]interface{}{
		"nullifiers":            nullifiers,
		"root":                  root,
		"merkleProof":           proofs,
		"enabled":               enabled,
		"inputCommitments":      inputs.inputCommitments,
		"inputValues":           inputs.inputValues,
		"inputSalts":            inputs.inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     inputs.outputCommitments,
		"outputValues":          inputs.outputValues,
		"outputSalts":           inputs.outputSalts,
		"outputOwnerPublicKeys": inputs.outputOwnerPublicKeys,
	}
	return witnessInputs, nil
}

func assembleInputs_lock(inputs *commonWitnessInputs, keyEntry *core.KeyEntry) map[string]interface{} {
	witnessInputs := map[string]interface{}{
		"commitments":     inputs.inputCommitments,
		"values":          inputs.inputValues,
		"salts":           inputs.inputSalts,
		"ownerPrivateKey": keyEntry.PrivateKeyForZkp,
	}
	return witnessInputs
}

func prepareInputsForNullifiers(ctx context.Context, inputs *commonWitnessInputs, extras *pb.ProvingRequestExtras_Nullifiers, keyEntry *core.KeyEntry) ([]*big.Int, *big.Int, [][]*big.Int, []*big.Int, error) {
	// calculate the nullifiers for the input UTXOs
	nullifiers := make([]*big.Int, len(inputs.inputCommitments))
	for i := 0; i < len(inputs.inputCommitments); i++ {
		// if the input commitment is 0, as a filler, the nullifier is 0
		if inputs.inputCommitments[i].Cmp(big.NewInt(0)) == 0 {
			nullifiers[i] = big.NewInt(0)
			continue
		}
		nullifier, err := CalculateNullifier(inputs.inputValues[i], inputs.inputSalts[i], keyEntry.PrivateKeyForZkp)
		if err != nil {
			return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorCalcNullifier, err)
		}
		nullifiers[i] = nullifier
	}
	root, ok := new(big.Int).SetString(extras.Root, 16)
	if !ok {
		return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeRootExtras)
	}
	var proofs [][]*big.Int
	for _, proof := range extras.MerkleProofs {
		var mp []*big.Int
		for _, node := range proof.Nodes {
			n, ok := new(big.Int).SetString(node, 16)
			if !ok {
				return nil, nil, nil, nil, i18n.NewError(ctx, msgs.MsgErrorDecodeMTPNodeExtras)
			}
			mp = append(mp, n)
		}
		proofs = append(proofs, mp)
	}
	enabled := make([]*big.Int, len(extras.Enabled))
	for i, e := range extras.Enabled {
		if e {
			enabled[i] = big.NewInt(1)
		} else {
			enabled[i] = big.NewInt(0)
		}
	}

	return nullifiers, root, proofs, enabled, nil
}
