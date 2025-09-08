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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	corepb "github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/proto"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/sparse-merkle-tree/core"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/internal/zeto/smt"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/iden3/go-iden3-crypto/babyjub"
)

type StateSchemas struct {
	CoinSchema           *prototk.StateSchema
	NftSchema            *prototk.StateSchema
	DataSchema           *prototk.StateSchema
	MerkleTreeRootSchema *prototk.StateSchema
	MerkleTreeNodeSchema *prototk.StateSchema
}

type MerkleTreeType int

const (
	StatesTree MerkleTreeType = iota
	LockedStatesTree
	KycStatesTree
)

type MerkleTreeSpec struct {
	Name       string
	Levels     int
	Type       MerkleTreeType
	Storage    smt.StatesStorage
	Tree       core.SparseMerkleTree
	EmptyProof *proto.MerkleProof
}

func NewMerkleTreeSpec(ctx context.Context, name string, treeType MerkleTreeType, callbacks plugintk.DomainCallbacks, merkleTreeRootSchemaId, merkleTreeNodeSchemaId string, stateQueryContext string) (*MerkleTreeSpec, error) {
	var tree core.SparseMerkleTree
	var levels int
	switch treeType {
	case StatesTree:
		levels = smt.SMT_HEIGHT_UTXO
	case LockedStatesTree:
		levels = smt.SMT_HEIGHT_UTXO
	case KycStatesTree:
		levels = smt.SMT_HEIGHT_KYC
	default:
		return nil, i18n.NewError(ctx, msgs.MsgUnknownSmtType, treeType)
	}
	storage := smt.NewStatesStorage(callbacks, name, stateQueryContext, merkleTreeRootSchemaId, merkleTreeNodeSchemaId)
	tree, err := smt.NewSmt(storage, levels)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorNewSmt, name, err)
	}
	emptyProof := &smt.Empty_Proof_Utxos
	if treeType == KycStatesTree {
		emptyProof = &smt.Empty_Proof_kyc
	}
	return &MerkleTreeSpec{
		Name:       name,
		Levels:     levels,
		Type:       treeType,
		Storage:    storage,
		Tree:       tree,
		EmptyProof: emptyProof,
	}, nil
}

const modulus = "21888242871839275222246405745257275088548364400416034343698204186575808495617"

func IsNullifiersToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_NULLIFIER || tokenName == constants.TOKEN_NF_ANON_NULLIFIER || tokenName == constants.TOKEN_ANON_NULLIFIER_KYC
}

func IsKycToken(tokenName string) bool {
	return tokenName == constants.TOKEN_ANON_NULLIFIER_KYC
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

func HexUint256To32ByteHexString(v *pldtypes.HexUint256) string {
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

func EncodeTransactionData(ctx context.Context, transaction *prototk.TransactionSpecification, infoStates []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
	var err error
	stateIDs := make([]pldtypes.Bytes32, len(infoStates))
	for i, state := range infoStates {
		stateIDs[i], err = pldtypes.ParseBytes32Ctx(ctx, state.Id)
		if err != nil {
			return nil, err
		}
	}

	transactionID, err := pldtypes.ParseBytes32Ctx(ctx, transaction.TransactionId)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorParseTxId, err)
	}
	dataValues := &types.ZetoTransactionData_V0{
		TransactionID: transactionID,
		InfoStates:    stateIDs,
	}

	var dataJSON []byte
	var dataABI []byte
	var data []byte
	dataJSON, err = json.Marshal(dataValues)
	if err == nil {
		dataABI, err = types.ZetoTransactionDataABI_V0.EncodeABIDataJSONCtx(ctx, dataJSON)
		if err == nil {
			data = append(data, types.ZetoTransactionDataID_V0...)
			data = append(data, dataABI...)
		}
	}

	return data, err
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
