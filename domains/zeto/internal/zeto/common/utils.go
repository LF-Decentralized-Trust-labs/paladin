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

package common

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"

	corepb "github.com/kaleido-io/paladin/domains/zeto/pkg/proto"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

const modulus = "21888242871839275222246405745257275088548364400416034343698204186575808495617"

func IsNullifiersCircuit(circuitId string) bool {
	return IsFungibleNullifiersCircuit(circuitId) || IsNonFungibleNullifiersCircuit(circuitId)
}

func IsFungibleNullifiersCircuit(circuitId string) bool {
	nullifierCircuits := []string{
		constants.CIRCUIT_ANON_NULLIFIER,
		constants.CIRCUIT_ANON_NULLIFIER_BATCH,
		constants.CIRCUIT_WITHDRAW_NULLIFIER,
		constants.CIRCUIT_WITHDRAW_NULLIFIER_BATCH,
	}
	return slices.Contains(nullifierCircuits, circuitId)
}

func IsNonFungibleNullifiersCircuit(circuitId string) bool {
	return constants.CIRCUIT_NF_ANON_NULLIFIER == circuitId
}

func IsEncryptionCircuit(circuitId string) bool {
	encryptionCircuits := []string{
		constants.CIRCUIT_ANON_ENC,
		constants.CIRCUIT_ANON_ENC_BATCH,
	}
	for _, c := range encryptionCircuits {
		if circuitId == c {
			return true
		}
	}
	return false
}

func IsBatchCircuit(sizeOfEndorsableStates int) bool {
	return sizeOfEndorsableStates > 2
}

func IsNonFungibleCircuit(circuitId string) bool {
	return circuitId == constants.CIRCUIT_NF_ANON || circuitId == constants.CIRCUIT_NF_ANON_NULLIFIER
}

func IsNullifiersToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_NULLIFIER || tokenName == constants.TOKEN_NF_ANON_NULLIFIER
}

func IsNonFungibleToken(tokenName string) bool {
	return tokenName == constants.TOKEN_NF_ANON || tokenName == constants.TOKEN_NF_ANON_NULLIFIER
}

func IsEncryptionToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_ENC
}

// the Zeto implementations support two input/output sizes for the circuits: 2 and 10,
// if the input or output size is larger than 2, then the batch circuit is used with
// input/output size 10
func GetInputSize(sizeOfEndorsableStates int) int {
	if sizeOfEndorsableStates <= 2 {
		return 2
	}
	return 10
}

func HexUint256To32ByteHexString(v *tktypes.HexUint256) string {
	paddedBytes := IntTo32ByteSlice(v.Int())
	return hex.EncodeToString(paddedBytes)
}

func IntTo32ByteSlice(bigInt *big.Int) (res []byte) {
	return bigInt.FillBytes(make([]byte, 32))
}

func IntTo32ByteHexString(bigInt *big.Int) string {
	paddedBytes := bigInt.FillBytes(make([]byte, 32))
	return hex.EncodeToString(paddedBytes)
}

func LoadBabyJubKey(payload []byte) (*babyjub.PublicKey, error) {
	var keyCompressed babyjub.PublicKeyComp
	if err := keyCompressed.UnmarshalText(payload); err != nil {
		return nil, err
	}
	return keyCompressed.Decompress()
}
func EncodeTransactionData(ctx context.Context, transaction *prototk.TransactionSpecification, transactionData ethtypes.HexBytes0xPrefix) (tktypes.HexBytes, error) {
	txID, err := tktypes.ParseHexBytes(ctx, transaction.TransactionId)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseTxId, err)
	}
	var data []byte
	data = append(data, transactionData...)
	data = append(data, txID...)
	return data, nil
}

func EncodeProof(proof *corepb.SnarkProof) map[string]interface{} {
	// Convert the proof json to the format that the Solidity verifier expects
	return map[string]interface{}{
		"pA": []string{proof.A[0], proof.A[1]},
		"pB": [][]string{
			{proof.B[0].Items[1], proof.B[0].Items[0]},
			{proof.B[1].Items[1], proof.B[1].Items[0]},
		},
		"pC": []string{proof.C[0], proof.C[1]},
	}
}

// Generate a random 256-bit integer
func CryptoRandBN254() (*big.Int, error) {
	// The BN254 field modulus.
	fieldModulus, ok := new(big.Int).SetString(modulus, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse field modulus")
	}

	// Generate a random number in [0, fieldModulus).
	tokenValue, err := rand.Int(rand.Reader, fieldModulus)
	if err != nil {
		return nil, err
	}
	return tokenValue, nil
}
