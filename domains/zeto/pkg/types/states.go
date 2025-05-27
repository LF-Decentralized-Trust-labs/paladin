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

package types

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo/core"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
)

var ZetoCoinABI = &abi.Parameter{
	Name:         "ZetoCoin",
	Indexed:      true,
	Type:         "tuple",
	InternalType: "struct ZetoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "owner", Type: "bytes32", Indexed: true},
		{Name: "amount", Type: "uint256"},
		{Name: "locked", Type: "bool", Indexed: true},
	},
}

type ZetoCoinState struct {
	ID              pldtypes.HexUint256 `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            ZetoCoin            `json:"data"`
}

type ZetoCoin struct {
	Salt   *pldtypes.HexUint256 `json:"salt"`
	Owner  pldtypes.HexBytes    `json:"owner"`
	Amount *pldtypes.HexUint256 `json:"amount"`
	Locked bool                 `json:"locked"`
	hash   *pldtypes.HexUint256
}

func (z *ZetoCoin) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.hash == nil {
		ownerKey, err := zetosigner.DecodeBabyJubJubPublicKey(z.Owner.HexString())
		if err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgErrorDecodeBJJKey, err)
		}
		commitment, err := poseidon.Hash([]*big.Int{
			z.Amount.Int(),
			z.Salt.Int(),
			ownerKey.X,
			ownerKey.Y,
		})
		if err != nil {
			return nil, err
		}
		z.hash = (*pldtypes.HexUint256)(commitment)
	}
	return z.hash, nil
}

var ZetoNFTokenABI = &abi.Parameter{
	Name:         "ZetoNFToken",
	Indexed:      true,
	Type:         "tuple",
	InternalType: "struct ZetoNFToken",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "uri", Type: "string"},
		{Name: "owner", Type: "bytes32", Indexed: true},
		{Name: "tokenID", Type: "uint256", Indexed: true},
	},
}

type TransactionData struct {
	Salt *pldtypes.HexUint256 `json:"salt"`
	Data pldtypes.HexBytes    `json:"data"`
	hash *pldtypes.HexUint256
}

var TransactionDataABI = &abi.Parameter{
	Name:         "TransactionData",
	Type:         "tuple",
	InternalType: "struct TransactionData",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "data", Type: "bytes"},
	},
}

func (z *TransactionData) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.hash == nil {
		hash := sha256.New()
		hash.Write(z.Salt.Int().Bytes())
		hash.Write(z.Data)
		hashBytes := pldtypes.HexBytes(hash.Sum(nil))
		hashInt, err := pldtypes.ParseHexUint256(ctx, hashBytes.String())
		if err != nil {
			return nil, err
		}
		z.hash = hashInt
	}
	return z.hash, nil
}

// ZetoNFTState represents the overall state of an NFT.
type ZetoNFTState struct {
	ID              pldtypes.HexUint256 `json:"id"`
	Created         pldtypes.Timestamp  `json:"created"`
	ContractAddress pldtypes.EthAddress `json:"contractAddress"`
	Data            ZetoNFToken         `json:"data"`
}

// ZetoNFToken holds the NFT token details.
type ZetoNFToken struct {
	Salt      *pldtypes.HexUint256 `json:"salt"`
	URI       string               `json:"uri"`
	Owner     pldtypes.HexBytes    `json:"owner"`
	TokenID   *pldtypes.HexUint256 `json:"tokenID"`
	utxoToken core.UTXO            // Calculated from TokenID, URI, etc.
}

// NewZetoNFToken creates a new ZetoNFToken from the given parameters.
func NewZetoNFToken(tokenID *pldtypes.HexUint256, uri string, publicKey *babyjub.PublicKey, salt *big.Int) *ZetoNFToken {
	return &ZetoNFToken{
		Salt:    (*pldtypes.HexUint256)(salt),
		URI:     uri,
		Owner:   pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(publicKey)),
		TokenID: tokenID,
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface for ZetoNFToken.
func (z *ZetoNFToken) UnmarshalJSON(data []byte) error {
	type alias ZetoNFToken // alias to avoid infinite recursion during unmarshaling

	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*z = ZetoNFToken(tmp)
	return nil
}

// Hash calculates the hash of the token using its UTXO representation.
func (z *ZetoNFToken) Hash(ctx context.Context) (*pldtypes.HexUint256, error) {
	if z.utxoToken == nil {
		if err := z.setUTXO(); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidUTXO, err)
		}
	}
	hash, err := z.utxoToken.GetHash()
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashState, err)
	}
	return (*pldtypes.HexUint256)(hash), nil
}

// setUTXO validates required fields and calculates the UTXO token.
func (z *ZetoNFToken) setUTXO() error {
	if err := z.validate(); err != nil {
		return err
	}

	// Decode the public key from the Owner field.
	publicKey, err := zetosigner.DecodeBabyJubJubPublicKey(z.Owner.HexString())
	if err != nil {
		return err
	}

	z.utxoToken = utxo.NewNonFungible(z.TokenID.Int(), z.URI, publicKey, z.Salt.Int())
	return nil
}

// validate ensures all required fields are present.
func (z *ZetoNFToken) validate() error {
	if z.TokenID == nil {
		return fmt.Errorf("tokenID is missing")
	}
	if z.URI == "" {
		return fmt.Errorf("uri is empty")
	}
	if z.Owner == nil {
		return fmt.Errorf("owner is missing")
	}
	if z.Salt == nil {
		return fmt.Errorf("salt is missing")
	}
	return nil
}

var MerkleTreeRootABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct MerkleTreeRoot",
	Components: abi.ParameterArray{
		{Name: "smtName", Type: "string", Indexed: true},
		{Name: "rootIndex", Type: "bytes32"},
	},
}

type MerkleTreeRoot struct {
	SmtName   string           `json:"smtName"`
	RootIndex pldtypes.Bytes32 `json:"rootIndex"`
}

func (m *MerkleTreeRoot) Hash() (string, error) {
	h := sha256.New()
	h.Write([]byte(m.SmtName))
	h.Write(m.RootIndex.Bytes())
	return pldtypes.Bytes32(h.Sum(nil)).HexString(), nil
}

var MerkleTreeNodeABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct MerkleTreeNode",
	Components: abi.ParameterArray{
		{Name: "refKey", Type: "bytes32", Indexed: true},
		{Name: "index", Type: "bytes32"},
		{Name: "type", Type: "bytes1"},
		{Name: "leftChild", Type: "bytes32"},
		{Name: "rightChild", Type: "bytes32"},
	},
}

type MerkleTreeNode struct {
	RefKey     pldtypes.Bytes32  `json:"refKey"`
	Index      pldtypes.Bytes32  `json:"index"`
	Type       pldtypes.HexBytes `json:"type"`
	LeftChild  pldtypes.Bytes32  `json:"leftChild"`
	RightChild pldtypes.Bytes32  `json:"rightChild"`
}

func (m *MerkleTreeNode) Hash() (string, error) {
	h := sha256.New()
	h.Write(m.RefKey.Bytes())
	h.Write(m.Index.Bytes())
	h.Write([]byte(m.Type))
	h.Write(m.LeftChild.Bytes())
	h.Write(m.RightChild.Bytes())
	return pldtypes.Bytes32(h.Sum(nil)).HexString(), nil
}

func GetStateSchemas() ([]string, error) {
	coinJSON, _ := json.Marshal(ZetoCoinABI)
	nftJSON, _ := json.Marshal(ZetoNFTokenABI)
	smtRootJSON, _ := json.Marshal(MerkleTreeRootABI)
	smtNodeJSON, _ := json.Marshal(MerkleTreeNodeABI)
	infoJSON, _ := json.Marshal(TransactionDataABI)

	return []string{
		string(coinJSON),
		string(nftJSON),
		string(smtRootJSON),
		string(smtNodeJSON),
		string(infoJSON),
	}, nil
}
