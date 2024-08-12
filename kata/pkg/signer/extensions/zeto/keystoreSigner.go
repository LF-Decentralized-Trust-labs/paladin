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

package extensions

import (
	"context"
	"errors"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/kaleido-io/paladin/kata/internal/cache"
	"github.com/kaleido-io/paladin/kata/internal/confutil"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/proto/zeto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/signer/keystore"
	"google.golang.org/protobuf/proto"
)

// zetoKeystoreSigner implements both the api.Keystore and KeyStoreSigner_snark
// interfaces for the Zeto domain.
type zetoKeystoreSigner struct {
	keyStore         api.KeyStore
	zkpProverConfig  *api.ZkpProverConfig
	circuitsCache    cache.Cache[string, witness.Calculator]
	provingKeysCache cache.Cache[string, []byte]
}

func NewZetoKeystoreSigner(ctx context.Context, config *api.StoreConfig) (*zetoKeystoreSigner, error) {
	// TODO: get the key store config from the Paladin config
	var keyStore api.KeyStore
	if config.FileSystem != nil {
		if ks, err := keystore.NewFilesystemStore(ctx, config.FileSystem); err != nil {
			return nil, err
		} else {
			keyStore = ks
		}
	} else if config.Static != nil {
		if ks, err := keystore.NewStaticKeyStore(ctx, config.Static); err != nil {
			return nil, err
		} else {
			keyStore = ks
		}
	} else {
		return nil, errors.New("key store config is required")
	}

	if config.ZkpProver == nil {
		return nil, errors.New("zkp prover config is required")
	} else if config.ZkpProver.CircuitsDir == "" {
		return nil, errors.New("zkp prover circuits directory config is required")
	} else if config.ZkpProver.ProvingKeysDir == "" {
		return nil, errors.New("zkp prover proving keys directory config is required")
	}

	cacheConfig := cache.Config{
		Capacity: confutil.P(100),
	}
	return &zetoKeystoreSigner{
		keyStore:         keyStore,
		zkpProverConfig:  config.ZkpProver,
		circuitsCache:    cache.NewCache[string, witness.Calculator](&cacheConfig, &cacheConfig),
		provingKeysCache: cache.NewCache[string, []byte](&cacheConfig, &cacheConfig),
	}, nil
}

func (ks *zetoKeystoreSigner) FindOrCreateLoadableKey(ctx context.Context, req *pb.ResolveKeyRequest, newKeyMaterial func() ([]byte, error)) (keyMaterial []byte, keyHandle string, err error) {
	return ks.keyStore.FindOrCreateLoadableKey(ctx, req, newKeyMaterial)
}

func (ks *zetoKeystoreSigner) LoadKeyMaterial(ctx context.Context, keyHandle string) ([]byte, error) {
	return ks.keyStore.LoadKeyMaterial(ctx, keyHandle)
}

func (ks *zetoKeystoreSigner) FindOrCreateKey_snark(ctx context.Context, req *pb.ResolveKeyRequest) (addr *babyjub.PublicKeyComp, keyHandle string, err error) {
	return nil, "", nil
}

func (ks *zetoKeystoreSigner) Prove_snark(ctx context.Context, keyHandle string, payload []byte) (*types.ZKProof, error) {
	bytes, err := ks.keyStore.LoadKeyMaterial(ctx, keyHandle)
	if err != nil {
		return nil, err
	}

	keyBytes := [32]byte{}
	copy(keyBytes[:], bytes)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)

	inputs := zeto.ProvingRequest{}
	// Unmarshal payload into inputs
	err = proto.Unmarshal(payload, &inputs)
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
	circuit, _ := ks.circuitsCache.Get(inputs.CircuitId)
	provingKey, _ := ks.provingKeysCache.Get(inputs.CircuitId)
	if circuit == nil || provingKey == nil {
		c, p, err := loadCircuit(inputs.CircuitId, ks.zkpProverConfig)
		if err != nil {
			return nil, err
		}
		ks.circuitsCache.Set(inputs.CircuitId, c)
		ks.provingKeysCache.Set(inputs.CircuitId, p)
		circuit = c
		provingKey = p
	}

	var wtns []byte
	switch inputs.CircuitId {
	case "anon":
		// no further input fields are needed
		wtns, err = calculateWitness_anon(inputs.Common, keyEntry, circuit)
	}

	if err != nil {
		return nil, err
	}

	proof, err := generateProof(wtns, provingKey)
	if err != nil {
		return nil, err
	}
	return proof, nil
}

func validateInputs(inputs *zeto.ProvingRequestCommon) error {
	if inputs.InputCommitments == nil || len(inputs.InputCommitments) == 0 {
		return errors.New("input commitments are required")
	}
	if inputs.InputValues == nil || len(inputs.InputValues) == 0 {
		return errors.New("input values are required")
	}
	if inputs.InputSalts == nil || len(inputs.InputSalts) == 0 {
		return errors.New("input salts are required")
	}
	if len(inputs.InputCommitments) != len(inputs.InputValues) || len(inputs.InputCommitments) != len(inputs.InputSalts) {
		return errors.New("input commitments, values, and salts must have the same length")
	}
	if inputs.OutputValues == nil || len(inputs.OutputValues) == 0 {
		return errors.New("output values are required")
	}
	if inputs.OutputOwners == nil || len(inputs.OutputOwners) == 0 {
		return errors.New("output owner keys are required")
	}
	if len(inputs.OutputValues) != len(inputs.OutputOwners) {
		return errors.New("output values and owner keys must have the same length")
	}
	return nil
}

func calculateWitness_anon(commonInputs *zeto.ProvingRequestCommon, keyEntry *core.KeyEntry, circuit witness.Calculator) ([]byte, error) {
	// construct the output UTXOs based on the values and owner public keys
	outputCommitments := make([]*big.Int, len(commonInputs.OutputValues))
	outputSalts := make([]*big.Int, len(commonInputs.OutputValues))
	outputOwnerPublicKeys := make([][]*big.Int, len(commonInputs.OutputValues))

	// TODO: how to tell the domain how to construct the UTXO?
	for i, value := range commonInputs.OutputValues {
		salt := utxo.NewSalt()
		outputSalts[i] = salt
		var ownerPubKeyComp babyjub.PublicKeyComp
		copy(ownerPubKeyComp[:], []byte(commonInputs.OutputOwners[i]))
		ownerPubKey, err := ownerPubKeyComp.Decompress()
		if err != nil {
			return nil, err
		}
		outputOwnerPublicKeys[i] = []*big.Int{ownerPubKey.X, ownerPubKey.Y}

		u := utxo.NewFungible(new(big.Int).SetUint64(value), ownerPubKey, salt)
		hash, err := u.GetHash()
		if err != nil {
			return nil, err
		}
		outputCommitments[i] = hash
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
		v, ok := new(big.Int).SetString(commonInputs.InputSalts[i], 16)
		if !ok {
			return nil, errors.New("failed to parse input salt")
		}
		inputSalts[i] = v
	}
	outputValues := make([]*big.Int, len(commonInputs.OutputValues))
	for i, v := range commonInputs.OutputValues {
		outputValues[i] = new(big.Int).SetUint64(v)
	}

	witnessInputs := map[string]interface{}{
		"inputCommitments":      inputCommitments,
		"inputValues":           inputValues,
		"inputSalts":            inputSalts,
		"inputOwnerPrivateKey":  keyEntry.PrivateKeyForZkp,
		"outputCommitments":     outputCommitments,
		"outputValues":          outputValues,
		"outputSalts":           outputSalts,
		"outputOwnerPublicKeys": outputOwnerPublicKeys,
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
