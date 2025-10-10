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
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockPubKey = func() *babyjub.PublicKey {
	x, _ := new(big.Int).SetString("20324599009286821207881465153085764126595806822268060878040393292028608397602", 0)
	y, _ := new(big.Int).SetString("6667720951847887467326343771312468792334056297732558024347070059459187374673", 0)
	return &babyjub.PublicKey{
		X: x,
		Y: y,
	}
}

// validOwner returns a valid owner value by encoding the public key (mockPubKey)
func validOwner() pldtypes.HexBytes {
	encoded := zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())
	return pldtypes.MustParseHexBytes(encoded)
}

func TestZetoCoin_Hash(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		coin        *ZetoCoin
		wantErr     bool
		errContains string
	}{
		{
			name: "valid coin",
			coin: &ZetoCoin{
				Owner:  validOwner(),
				Amount: (*pldtypes.HexUint256)(big.NewInt(500)),
				Salt:   (*pldtypes.HexUint256)(big.NewInt(123)),
				hash:   nil, // no cached hash yet
			},
			wantErr: false,
		},
		{
			name: "invalid coin (nil owner)",
			coin: &ZetoCoin{
				Owner:  nil,
				Amount: (*pldtypes.HexUint256)(big.NewInt(500)),
				Salt:   (*pldtypes.HexUint256)(big.NewInt(123)),
			},
			wantErr:     true,
			errContains: "PD210001: Failed to decode babyjubjub key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := tc.coin.Hash(ctx)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				require.NotNil(t, hash)
				// Verify caching: a second call returns the same value.
				hash2, err2 := tc.coin.Hash(ctx)
				require.NoError(t, err2)
				assert.Equal(t, hash.String(), hash2.String(), "cached hash should be identical")
			}
		})
	}
}

func TestABIParameters(t *testing.T) {
	abiTests := []struct {
		name               string
		param              *abi.Parameter
		expectedName       string
		expectedType       string
		expectedComponents int
	}{
		{
			name:               "ZetoCoin ABI",
			param:              ZetoCoinABI,
			expectedName:       "ZetoCoin",
			expectedType:       "tuple",
			expectedComponents: 4,
		},
		{
			name:               "ZetoNFToken ABI",
			param:              ZetoNFTokenABI,
			expectedName:       "ZetoNFToken",
			expectedType:       "tuple",
			expectedComponents: 4,
		},
	}

	for _, tc := range abiTests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedName, tc.param.Name)
			assert.Equal(t, tc.expectedType, tc.param.Type)
			assert.Len(t, tc.param.Components, tc.expectedComponents)
		})
	}
}

func TestNewZetoNFToken(t *testing.T) {
	tokenID := (*pldtypes.HexUint256)(big.NewInt(456))
	uri := "https://example.com"
	saltVal := big.NewInt(123)
	tests := []struct {
		name             string
		tokenID          *pldtypes.HexUint256
		uri              string
		pubKey           *babyjub.PublicKey
		salt             *big.Int
		expectedURI      string
		expectedTokenID  string
		expectedOwnerHex string
		expectedSalt     string
	}{
		{
			name:             "valid token",
			tokenID:          tokenID,
			uri:              uri,
			pubKey:           mockPubKey(),
			salt:             saltVal,
			expectedURI:      uri,
			expectedTokenID:  tokenID.Int().String(), // "456"
			expectedOwnerHex: zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()),
			expectedSalt:     saltVal.String(), // "123"
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nft := NewZetoNFToken(tc.tokenID, tc.uri, tc.pubKey, tc.salt)
			require.NotNil(t, nft, "NewZetoNFToken should return a valid token")
			assert.Equal(t, tc.expectedURI, nft.URI, "URI should match")
			assert.Equal(t, tc.tokenID, nft.TokenID, "TokenID should match")
			expectedOwner := pldtypes.MustParseHexBytes(tc.expectedOwnerHex)
			assert.Equal(t, expectedOwner, nft.Owner, "Owner should be set from encoded public key")
			expectedSalt := (*pldtypes.HexUint256)(tc.salt)
			assert.Equal(t, expectedSalt, nft.Salt, "Salt should match")
			assert.Nil(t, nft.utxoToken, "utxoToken should not be initialized")
		})
	}
}

func TestZetoNFToken_UnmarshalJSON(t *testing.T) {
	// Use mockPubKey for a valid owner.
	ownerHex := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())).String()
	tests := []struct {
		name         string
		jsonStr      string
		expectedURI  string
		expectedSalt string
		expectedTID  string
		expectedOwn  string
		expectErr    bool
	}{
		{
			name: "valid JSON",
			jsonStr: fmt.Sprintf(`{
				"salt": "123",
				"uri": "https://example.com",
				"owner": "%s",
				"tokenID": "456"
			}`, ownerHex),
			expectedURI:  "https://example.com",
			expectedSalt: "123",
			expectedTID:  "456",
			expectedOwn:  ownerHex,
			expectErr:    false,
		},
		{
			name: "valid JSON",
			jsonStr: `{
				salt": "123"
				"tokenID": "456"
			}`,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var token ZetoNFToken
			err := json.Unmarshal([]byte(tc.jsonStr), &token)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				hash, err := token.Hash(context.Background())
				require.NoError(t, err)
				assert.Equal(t, tc.expectedURI, token.URI, "URI should match")
				assert.Equal(t, tc.expectedSalt, token.Salt.Int().String(), "Salt should match")
				assert.Equal(t, tc.expectedTID, token.TokenID.Int().String(), "TokenID should match")
				expectedOwner := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()))
				assert.Equal(t, expectedOwner, token.Owner, "Owner should match")
				assert.Equal(t, "0x11e84f5f703728d1f231655c59597678524e3a14ce684d07a0b653bd51ccd650", hash.String(), "Hash should match")
				// UnmarshalJSON calls setUTXO (ignoring errors), so utxoToken should be non-nil.
				assert.NotNil(t, token.utxoToken, "utxoToken should be set")
			}
		})
	}
}

// For testing Hash we use a custom utxoToken that returns a fixed hash.
type testUTXO struct {
	hashVal *big.Int
}

func (tu testUTXO) GetHash() (*big.Int, error) {
	return tu.hashVal, nil
}

func TestZetoNFToken_Hash(t *testing.T) {
	ctx := context.Background()
	tokenID := (*pldtypes.HexUint256)(big.NewInt(456))
	uri := "https://example.com"
	tests := []struct {
		name         string
		token        *ZetoNFToken
		expectErr    bool
		expectedHash string // expected hash as string (if no error)
	}{
		{
			name: "success",
			token: &ZetoNFToken{
				Salt:    (*pldtypes.HexUint256)(big.NewInt(123)),
				URI:     uri,
				Owner:   pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())),
				TokenID: tokenID,
				// Set utxoToken to our testUTXO returning hash value 789.
				utxoToken: testUTXO{hashVal: big.NewInt(789)},
			},
			expectErr:    false,
			expectedHash: "789",
		},
		{
			name: "error - missing TokenID",
			token: &ZetoNFToken{
				Salt:      (*pldtypes.HexUint256)(big.NewInt(123)),
				URI:       uri,
				Owner:     pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey())),
				TokenID:   nil,
				utxoToken: nil,
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := tc.token.Hash(ctx)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, hash)
				assert.Equal(t, tc.expectedHash, hash.Int().String(), "hash should match expected value")
			}
		})
	}
}

func TestZetoNFToken_validate(t *testing.T) {
	tokenID := (*pldtypes.HexUint256)(big.NewInt(456))
	salt := (*pldtypes.HexUint256)(big.NewInt(123))
	uri := "https://example.com"
	owner := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()))
	tests := []struct {
		name      string
		token     *ZetoNFToken
		expectErr bool
	}{
		{
			name: "complete token",
			token: &ZetoNFToken{
				Salt:    salt,
				URI:     uri,
				Owner:   owner,
				TokenID: tokenID,
			},
			expectErr: false,
		},
		{
			name: "missing TokenID",
			token: &ZetoNFToken{
				Salt:  salt,
				URI:   uri,
				Owner: owner,
			},
			expectErr: true,
		},
		{
			name: "empty URI",
			token: &ZetoNFToken{
				Salt:    salt,
				URI:     "",
				Owner:   owner,
				TokenID: tokenID,
			},
			expectErr: true,
		},
		{
			name: "missing Owner",
			token: &ZetoNFToken{
				Salt:    salt,
				URI:     uri,
				Owner:   nil,
				TokenID: tokenID,
			},
			expectErr: true,
		},
		{
			name: "missing Salt",
			token: &ZetoNFToken{
				URI:     uri,
				Owner:   owner,
				TokenID: tokenID,
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.token.validate()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestZetoNFToken_setUTXO(t *testing.T) {
	tokenID := (*pldtypes.HexUint256)(big.NewInt(456))
	salt := (*pldtypes.HexUint256)(big.NewInt(123))
	uri := "https://example.com"
	owner := pldtypes.MustParseHexBytes(zetosigner.EncodeBabyJubJubPublicKey(mockPubKey()))
	tests := []struct {
		name      string
		token     *ZetoNFToken
		expectErr bool
	}{
		{
			name: "valid token",
			token: &ZetoNFToken{
				Salt:    salt,
				URI:     uri,
				Owner:   owner,
				TokenID: tokenID,
			},
			expectErr: false,
		},
		{
			name: "invalid token (missing TokenID)",
			token: &ZetoNFToken{
				Salt:  salt,
				URI:   uri,
				Owner: owner,
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.token.setUTXO()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tc.token.utxoToken, "utxoToken should be set")
			}
		})
	}
}
