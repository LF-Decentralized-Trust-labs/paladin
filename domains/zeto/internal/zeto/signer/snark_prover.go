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
	"slices"
	"strings"
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/common"
	wtns "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/signer/witness"
	pb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/cache"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/core"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/key-manager/key"
	"github.com/iden3/go-rapidsnark/prover"
	"github.com/iden3/go-rapidsnark/types"
	"github.com/iden3/go-rapidsnark/witness/v2"
	"google.golang.org/protobuf/proto"
)

var defaultSnarkProverConfig = zetosignerapi.SnarkProverConfig{
	MaxProverPerCircuit: confutil.P(10),
}
var getWitnessInputs func(tokeType pb.TokenType, circuit *zetosignerapi.Circuit, extras interface{}) (witnessInputs, error) = newWitnessInputs

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
	if err := checkVerifierType(ctx, verifierType); err != nil {
		return "", err
	}
	pk, err := common.NewBabyJubJubPrivateKey(privateKey)
	if err != nil {
		return "", i18n.NewError(ctx, msgs.MsgErrorDecodePrivateKey, err)
	}
	pubKey := pk.Public()
	if verifierType == zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X {
		compressedPubkey := common.EncodeBabyJubJubPublicKey(pubKey)
		return compressedPubkey, nil
	} else {
		pubKeyComp := pubKey.Compress()
		uncompressedPubkey, err := pubKeyComp.Decompress()
		if err != nil {
			return "", i18n.NewError(ctx, msgs.MsgErrorDecodePublicKeyFromHex, err)
		}
		return strings.Join([]string{"0x" + uncompressedPubkey.X.Text(16), "0x" + uncompressedPubkey.Y.Text(16)}, ","), nil
	}
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
	if inputs.Circuit.Name == "" {
		return nil, i18n.NewError(ctx, msgs.MsgErrorMissingCircuitID)
	}

	circuit := getCircuit(inputs)

	// obtain a slot for the proof generation for this specific circuit
	// check whether this is a controlling channel
	sp.circuitsWorkerIndexChanRWLock.RLock()
	circuitId := circuit.Name
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
	witnessCalculator, _ := sp.circuitsCache.Get(workerID)
	provingKey, _ := sp.provingKeysCache.Get(workerID)
	sp.proverCacheRWLock.RUnlock() // release the lock, happy path, 1 lock is good enough
	if witnessCalculator == nil || provingKey == nil {
		sp.proverCacheRWLock.Lock()
		// obtain the W&R lock and check again
		witnessCalculator, _ = sp.circuitsCache.Get(workerID)
		provingKey, _ = sp.provingKeysCache.Get(workerID)
		if witnessCalculator == nil || provingKey == nil {
			// the generated WASM instance can only generate one proof at a time, circuitsWorkerIndexChan is used to ensure only 1 proof request
			// is served per WASM instance at any given time
			c, p, err := sp.circuitLoader(ctx, circuitId, sp.zkpProverConfig)
			if err != nil {
				return nil, err
			}
			sp.circuitsCache.Set(workerID, c)
			sp.provingKeysCache.Set(workerID, p)
			witnessCalculator = c
			provingKey = p
		}
		sp.proverCacheRWLock.Unlock()
	}

	wtns, err := calculateWitness(ctx, circuit, inputs.Common, extras, keyEntry, witnessCalculator)
	if err != nil {
		return nil, err
	}

	proof, err := sp.proofGenerator(ctx, wtns, provingKey)
	if err != nil {
		return nil, err
	}

	proofBytes, err := serializeProofResponse(circuit, proof)
	if err != nil {
		return nil, err
	}

	return proofBytes, nil
}

func getCircuit(inputs *pb.ProvingRequest) *zetosignerapi.Circuit {
	circuitId := inputs.Circuit.Name
	if len(inputs.Common.InputCommitments) > 2 {
		circuitId = getBatchCircuit(circuitId)
	}
	ret := zetosignerapi.NewCircuitFromProto(inputs.Circuit)
	ret.Name = circuitId
	return ret
}

func serializeProofResponse(circuit *zetosignerapi.Circuit, proof *types.ZKProof) ([]byte, error) {
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
	if circuit.Type == zetosignerapi.Transfer {
		if circuit.UsesEncryption {
			if !IsBatchCircuit(circuit.Name) {
				publicInputs["ecdhPublicKey"] = strings.Join(proof.PubSignals[0:2], ",")
				publicInputs["encryptedValues"] = strings.Join(proof.PubSignals[2:10], ",")
				publicInputs["encryptionNonce"] = proof.PubSignals[14]
			} else {
				publicInputs["ecdhPublicKey"] = strings.Join(proof.PubSignals[0:2], ",")
				publicInputs["encryptedValues"] = strings.Join(proof.PubSignals[2:42], ",")
				publicInputs["encryptionNonce"] = proof.PubSignals[62]
			}
		} else if circuit.UsesNullifiers {
			if !IsBatchCircuit(circuit.Name) {
				publicInputs["nullifiers"] = strings.Join(proof.PubSignals[0:2], ",")
				publicInputs["root"] = proof.PubSignals[2]
			} else {
				publicInputs["nullifiers"] = strings.Join(proof.PubSignals[0:10], ",")
				publicInputs["root"] = proof.PubSignals[10]
			}
		}
	} else if circuit.Type == zetosignerapi.Withdraw {
		if circuit.UsesNullifiers {
			if !IsBatchCircuit(circuit.Name) {
				publicInputs["nullifiers"] = strings.Join(proof.PubSignals[1:3], ",")
				publicInputs["root"] = proof.PubSignals[3]
			} else {
				publicInputs["nullifiers"] = strings.Join(proof.PubSignals[1:11], ",")
				publicInputs["root"] = proof.PubSignals[11]
			}
		}
	}

	res := pb.ProvingResponse{
		Proof:        &snark,
		PublicInputs: publicInputs,
	}

	return proto.Marshal(&res)
}

func calculateWitness(ctx context.Context, circuit *zetosignerapi.Circuit, commonInputs *pb.ProvingRequestCommon, extras interface{}, keyEntry *core.KeyEntry, witnessCalculator witness.Calculator) ([]byte, error) {
	inputs, err := getWitnessInputs(commonInputs.TokenType, circuit, extras)
	if err != nil {
		return nil, err
	}

	// Validate the inputs
	if err := inputs.Validate(ctx, commonInputs); err != nil {
		return nil, err
	}

	// Build the common witness inputs
	if err := inputs.Build(ctx, commonInputs); err != nil {
		return nil, err
	}

	// Assemble the circuit-specific witness inputs
	witnessInputs, err := inputs.Assemble(ctx, keyEntry)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorAssembleInputs, err)
	}

	// Calculate the witness binary
	wtns, err := witnessCalculator.CalculateWTNSBin(witnessInputs, true)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorCalcWitness, err)
	}

	return wtns, nil
}

func newWitnessInputs(tokenType pb.TokenType, circuit *zetosignerapi.Circuit, extras interface{}) (witnessInputs, error) {
	switch circuit.Type {
	case zetosignerapi.Deposit:
		return &wtns.DepositWitnessInputs{}, nil
	case zetosignerapi.Withdraw:
		if circuit.UsesNullifiers {
			nullifierExtras, ok := extras.(*pb.ProvingRequestExtras_Nullifiers)
			if !ok {
				return nil, fmt.Errorf("unexpected extras type for anon nullifier circuit")
			}
			return &wtns.WithdrawNullifierWitnessInputs{
				FungibleNullifierWitnessInputs: wtns.FungibleNullifierWitnessInputs{
					Extras: nullifierExtras,
				},
			}, nil
		}
		return &wtns.WithdrawWitnessInputs{}, nil
	case zetosignerapi.Transfer:
		if tokenType == pb.TokenType_fungible {
			if circuit.UsesEncryption {
				encExtras, ok := extras.(*pb.ProvingRequestExtras_Encryption)
				if !ok {
					return nil, fmt.Errorf("unexpected extras type for encryption circuit")
				}
				return &wtns.FungibleEncWitnessInputs{Enc: encExtras}, nil
			} else if circuit.UsesNullifiers {
				if circuit.UsesKyc {
					nullifierKycExtras, ok := extras.(*pb.ProvingRequestExtras_NullifiersKyc)
					if !ok {
						return nil, fmt.Errorf("unexpected extras type for anon nullifier kyc circuit")
					}
					return &wtns.FungibleNullifierKycWitnessInputs{
						Extras: nullifierKycExtras,
					}, nil
				}
				nullifierExtras, ok := extras.(*pb.ProvingRequestExtras_Nullifiers)
				if !ok {
					return nil, fmt.Errorf("unexpected extras type for anon nullifier circuit")
				}
				return &wtns.FungibleNullifierWitnessInputs{
					Extras: nullifierExtras,
				}, nil
			} else {
				return &wtns.FungibleWitnessInputs{}, nil
			}
		} else {
			return &wtns.NonFungibleWitnessInputs{}, nil
		}
	case zetosignerapi.TransferLocked:
		return &wtns.FungibleWitnessInputs{}, nil
	}

	return nil, fmt.Errorf("unsupported circuit type %s", circuit.Type)
}

func generateProof(ctx context.Context, wtns, provingKey []byte) (*types.ZKProof, error) {
	proof, err := prover.Groth16Prover(provingKey, wtns)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorGenerateProof, err)
	}
	return proof, nil
}

func checkVerifierType(ctx context.Context, verifierType string) error {
	supportedVerifierTypes := []string{
		zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X,
		zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_UNCOMPRESSED_0X,
	}
	if slices.Contains(supportedVerifierTypes, verifierType) {
		return nil
	}
	return i18n.NewError(ctx, msgs.MsgErrorVerifierTypeMismatch, verifierType, strings.Join(supportedVerifierTypes, ", "))
}
