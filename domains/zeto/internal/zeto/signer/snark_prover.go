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
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	pb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"google.golang.org/protobuf/proto"
)

var defaultSnarkProverConfig = zetosignerapi.SnarkProverConfig{
	MaxProverPerCircuit: confutil.P(10),
}

// snarkProver encapsulates the logic for generating SNARK proofs
type snarkProver struct {
	zkpProverConfig               *zetosignerapi.SnarkProverConfig
	circuitsCache                 cache.Cache[string, witness.Calculator]
	provingKeysCache              cache.Cache[string, []byte]
	proverCacheRWLock             sync.RWMutex
	workerPerCircuit              int
	circuitsWorkerIndexChanRWLock sync.RWMutex
	circuitsWorkerIndexChan       map[string]chan *int
	circuitLoader                 func(ctx context.Context, circuitID string, config *zetosignerapi.SnarkProverConfig) (witness.Calculator, []byte, error)
	proofGenerator                func(ctx context.Context, witness []byte, provingKey []byte) (*types.ZKProof, error)
}

func NewSnarkProver(conf *zetosignerapi.SnarkProverConfig) (signerapi.InMemorySigner, error) {
	return newSnarkProver(conf)
}

func newSnarkProver(conf *zetosignerapi.SnarkProverConfig) (*snarkProver, error) {
	cacheConfig := pldconf.CacheConfig{
		Capacity: confutil.P(50),
	}
	return &snarkProver{
		zkpProverConfig:         conf,
		circuitsCache:           cache.NewCache[string, witness.Calculator](&cacheConfig, &cacheConfig),
		provingKeysCache:        cache.NewCache[string, []byte](&cacheConfig, &cacheConfig),
		circuitLoader:           loadCircuit,
		proofGenerator:          generateProof,
		workerPerCircuit:        confutil.Int(conf.MaxProverPerCircuit, *defaultSnarkProverConfig.MaxProverPerCircuit),
		circuitsWorkerIndexChan: make(map[string]chan *int),
	}, nil
}

func (sp *snarkProver) GetVerifier(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
	if !zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP.MatchString(algorithm) {
		return "", i18n.NewError(ctx, msgs.MsgErrorSignAlgoMismatch, algorithm, zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP)
	}
	if verifierType != zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X {
		return "", i18n.NewError(ctx, msgs.MsgErrorVerifierTypeMismatch, algorithm, zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	}
	pk, err := NewBabyJubJubPrivateKey(privateKey)
	if err != nil {
		return "", err
	}
	return EncodeBabyJubJubPublicKey(pk.Public()), nil
}

func (sp *snarkProver) GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error) {
	if !zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP.MatchString(algorithm) {
		return -1, i18n.NewError(ctx, msgs.MsgErrorSignAlgoMismatch, algorithm, zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP)
	}
	return 32, nil
}

func (sp *snarkProver) Sign(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) ([]byte, error) {
	if !zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP.MatchString(algorithm) {
		return nil, i18n.NewError(ctx, msgs.MsgErrorSignAlgoMismatch, algorithm, zetosignerapi.ALGO_DOMAIN_ZETO_SNARK_BJJ_REGEXP)
	}
	if payloadType != zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK {
		return nil, i18n.NewError(ctx, msgs.MsgErrorPayloadTypeMismatch, payloadType, zetosignerapi.PAYLOAD_DOMAIN_ZETO_SNARK)
	}

	keyBytes := [32]byte{}
	copy(keyBytes[:], privateKey)
	keyEntry := key.NewKeyEntryFromPrivateKeyBytes(keyBytes)

	inputs, extras, err := decodeProvingRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	// Perform proof generation
	if inputs.CircuitId == "" {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMissingCircuitID)
	}
	if err := validateInputs(ctx, inputs.Common); err != nil {
		return nil, err
	}

	circuitId := getCircuitId(inputs)

	// obtain a slot for the proof generation for this specific circuit
	// check whether this is a controlling channel
	sp.circuitsWorkerIndexChanRWLock.RLock()
	ccChan, chanelFound := sp.circuitsWorkerIndexChan[circuitId]
	sp.circuitsWorkerIndexChanRWLock.RUnlock()
	if !chanelFound {
		// if not found, obtain the W&R lock and check again before initializing
		sp.circuitsWorkerIndexChanRWLock.Lock()
		ccChan, chanelFound = sp.circuitsWorkerIndexChan[circuitId]
		if !chanelFound {
			ccChan = make(chan *int, sp.workerPerCircuit) // init token channel
			sp.circuitsWorkerIndexChan[circuitId] = ccChan
			for i := 0; i < sp.workerPerCircuit; i++ {
				ccChan <- confutil.P(i) // add all tokens
			}
		}
		sp.circuitsWorkerIndexChanRWLock.Unlock()
	}

	var workerIndex *int
	select {
	case workerIndex = <-ccChan: // wait till there is a worker available
		defer func() {
			// put the worker index back into the queue upon function exit
			ccChan <- workerIndex
		}()
	case <-ctx.Done():
		return nil, errors.New("context cancelled")
	}

	workerID := fmt.Sprintf("%s-%d", circuitId, workerIndex)
	// Perform proof generation
	// Read lock to check the cache
	sp.proverCacheRWLock.RLock()
	circuit, _ := sp.circuitsCache.Get(workerID)
	provingKey, _ := sp.provingKeysCache.Get(workerID)
	sp.proverCacheRWLock.RUnlock() // release the lock, happy path, 1 lock is good enough
	if circuit == nil || provingKey == nil {
		sp.proverCacheRWLock.Lock()
		// obtain the W&R lock and check again
		circuit, _ = sp.circuitsCache.Get(workerID)
		provingKey, _ = sp.provingKeysCache.Get(workerID)
		if circuit == nil || provingKey == nil {
			// the generated WASM instance can only generate one proof at a time, circuitsWorkerIndexChan is used to ensure only 1 proof request
			// is served per WASM instance at any given time
			c, p, err := sp.circuitLoader(ctx, circuitId, sp.zkpProverConfig)
			if err != nil {
				return nil, err
			}
			sp.circuitsCache.Set(workerID, c)
			sp.provingKeysCache.Set(workerID, p)
			circuit = c
			provingKey = p
		}
		sp.proverCacheRWLock.Unlock()
	}
	wtns, err := calculateWitness(ctx, circuitId, inputs.Common, extras, keyEntry, circuit)
	if err != nil {
		return nil, err
	}

	proof, err := sp.proofGenerator(ctx, wtns, provingKey)
	if err != nil {
		return nil, err
	}

	proofBytes, err := serializeProofResponse(circuitId, proof)
	if err != nil {
		return nil, err
	}

	return proofBytes, nil
}

func getCircuitId(inputs *pb.ProvingRequest) string {
	circuitId := inputs.CircuitId
	if len(inputs.Common.InputCommitments) > 2 {
		circuitId += "_batch"
	}
	return circuitId
}

func validateInputs(ctx context.Context, inputs *pb.ProvingRequestCommon) error {
	if len(inputs.InputCommitments) != len(inputs.InputValues) || len(inputs.InputCommitments) != len(inputs.InputSalts) {
		return i18n.NewError(ctx, msgs.MsgErrorInputsDiffLength)
	}
	if len(inputs.OutputValues) != len(inputs.OutputOwners) {
		return i18n.NewError(ctx, msgs.MsgErrorOutputsDiffLength)
	}
	return nil
}

func serializeProofResponse(circuitId string, proof *types.ZKProof) ([]byte, error) {
	snark := pb.SnarkProof{}
	snark.A = proof.Proof.A
	snark.B = make([]*pb.B_Item, 0, len(proof.Proof.B))
	for _, p := range proof.Proof.B {
		bItems := pb.B_Item{}
		bItems.Items = append(bItems.Items, p...)
		snark.B = append(snark.B, &bItems)
	}
	snark.C = proof.Proof.C

	publicInputs := make(map[string]string)
	switch circuitId {
	case constants.CIRCUIT_ANON_ENC:
		publicInputs["ecdhPublicKey"] = strings.Join(proof.PubSignals[0:2], ",")
		publicInputs["encryptedValues"] = strings.Join(proof.PubSignals[2:10], ",")
		publicInputs["encryptionNonce"] = proof.PubSignals[14]
	case constants.CIRCUIT_ANON_ENC_BATCH:
		publicInputs["ecdhPublicKey"] = strings.Join(proof.PubSignals[0:2], ",")
		publicInputs["encryptedValues"] = strings.Join(proof.PubSignals[2:42], ",")
		publicInputs["encryptionNonce"] = proof.PubSignals[62]
	case constants.CIRCUIT_ANON_NULLIFIER:
		publicInputs["nullifiers"] = strings.Join(proof.PubSignals[:2], ",")
		publicInputs["root"] = proof.PubSignals[2]
	case constants.CIRCUIT_ANON_NULLIFIER_BATCH:
		publicInputs["nullifiers"] = strings.Join(proof.PubSignals[:10], ",")
		publicInputs["root"] = proof.PubSignals[10]
	case constants.CIRCUIT_WITHDRAW_NULLIFIER:
		publicInputs["nullifiers"] = strings.Join(proof.PubSignals[1:3], ",")
		publicInputs["root"] = proof.PubSignals[3]
	case constants.CIRCUIT_WITHDRAW_NULLIFIER_BATCH:
		publicInputs["nullifiers"] = strings.Join(proof.PubSignals[1:11], ",")
		publicInputs["root"] = proof.PubSignals[11]
	}

	res := pb.ProvingResponse{
		Proof:        &snark,
		PublicInputs: publicInputs,
	}

	return proto.Marshal(&res)
}

func calculateWitness(ctx context.Context, circuitId string, commonInputs *pb.ProvingRequestCommon, extras interface{}, keyEntry *core.KeyEntry, circuit witness.Calculator) ([]byte, error) {
	inputs, err := buildCircuitInputs(ctx, commonInputs)
	if err != nil {
		return nil, err
	}

	var witnessInputs map[string]any
	switch circuitId {
	case constants.CIRCUIT_ANON, constants.CIRCUIT_ANON_BATCH:
		witnessInputs = assembleInputs_anon(inputs, keyEntry)
	case constants.CIRCUIT_ANON_ENC, constants.CIRCUIT_ANON_ENC_BATCH:
		witnessInputs, err = assembleInputs_anon_enc(ctx, inputs, extras.(*pb.ProvingRequestExtras_Encryption), keyEntry)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorAssembleInputs, err)
		}
	case constants.CIRCUIT_ANON_NULLIFIER, constants.CIRCUIT_ANON_NULLIFIER_BATCH:
		witnessInputs, err = assembleInputs_anon_nullifier(ctx, inputs, extras.(*pb.ProvingRequestExtras_Nullifiers), keyEntry)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorAssembleInputs, err)
		}
	case constants.CIRCUIT_DEPOSIT:
		witnessInputs = assembleInputs_deposit(inputs)
	case constants.CIRCUIT_WITHDRAW, constants.CIRCUIT_WITHDRAW_BATCH:
		witnessInputs = assembleInputs_withdraw(inputs, keyEntry)
	case constants.CIRCUIT_WITHDRAW_NULLIFIER, constants.CIRCUIT_WITHDRAW_NULLIFIER_BATCH:
		witnessInputs, err = assembleInputs_withdraw_nullifier(ctx, inputs, extras.(*pb.ProvingRequestExtras_Nullifiers), keyEntry)
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorAssembleInputs, err)
		}
	case constants.CIRCUIT_LOCK, constants.CIRCUIT_LOCK_BATCH:
		witnessInputs = assembleInputs_lock(inputs, keyEntry)
	}

	wtns, err := circuit.CalculateWTNSBin(witnessInputs, true)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorCalcWitness, err)
	}

	return wtns, nil
}

func generateProof(ctx context.Context, wtns, provingKey []byte) (*types.ZKProof, error) {
	proof, err := prover.Groth16Prover(provingKey, wtns)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateProof, err)
	}
	return proof, nil
}
