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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo"
	"github.com/hyperledger-labs/zeto/go-sdk/pkg/utxo/core"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/kaleido-io/paladin/domains/zeto/internal/msgs"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner"
	"github.com/kaleido-io/paladin/toolkit/pkg/i18n"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type ZetoCoinState struct {
	ID              tktypes.HexUint256 `json:"id"`
	Created         tktypes.Timestamp  `json:"created"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            ZetoCoin           `json:"data"`
}

type ZetoCoin struct {
	Salt   *tktypes.HexUint256 `json:"salt"`
	Owner  tktypes.HexBytes    `json:"owner"`
	Amount *tktypes.HexUint256 `json:"amount"`
	hash   *tktypes.HexUint256
}

var ZetoCoinABI = &abi.Parameter{
	Name:         "ZetoCoin",
	Indexed:      true,
	Type:         "tuple",
	InternalType: "struct ZetoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "uint256"},
		{Name: "owner", Type: "bytes32", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
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

func (z *ZetoCoin) Hash(ctx context.Context) (*tktypes.HexUint256, error) {
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
		z.hash = (*tktypes.HexUint256)(commitment)
	}
	return z.hash, nil
}

// ZetoNFTState represents the overall state of an NFT.
type ZetoNFTState struct {
	ID              tktypes.HexUint256 `json:"id"`
	Created         tktypes.Timestamp  `json:"created"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            ZetoNFToken        `json:"data"`
}

// ZetoNFToken holds the NFT token details.
type ZetoNFToken struct {
	Salt      *tktypes.HexUint256 `json:"salt"`
	URI       string              `json:"uri"`
	Owner     tktypes.HexBytes    `json:"owner"`
	TokenID   *tktypes.HexUint256 `json:"tokenID"`
	utxoToken core.UTXO           // Calculated from TokenID, URI, etc.
}

// NewZetoNFToken creates a new ZetoNFToken from the given parameters.
func NewZetoNFToken(tokenID *tktypes.HexUint256, uri string, publicKey *babyjub.PublicKey, salt *big.Int) *ZetoNFToken {
	return &ZetoNFToken{
		Salt:    (*tktypes.HexUint256)(salt),
		URI:     uri,
		Owner:   tktypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(publicKey)),
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
func (z *ZetoNFToken) Hash(ctx context.Context) (*tktypes.HexUint256, error) {
	if z.utxoToken == nil {
		if err := z.setUTXO(); err != nil {
			return nil, i18n.NewError(ctx, msgs.MsgInvalidUTXO, err)
		}
	}
	hash, err := z.utxoToken.GetHash()
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorHashState, err)
	}
	return (*tktypes.HexUint256)(hash), nil
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
