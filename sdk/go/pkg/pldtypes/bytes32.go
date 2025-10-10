// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldtypes

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

// Bytes32 is a 32 byte value, with DB storage serialization
type Bytes32 [32]byte

// No checking in this function on length
func NewBytes32FromSlice(bytes []byte) Bytes32 {
	h := Bytes32{}
	copy(h[:], bytes[:])
	return h
}

func Bytes32Keccak(b []byte) Bytes32 {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(b)
	var h32 Bytes32
	_ = hash.Sum(h32[0:0])
	return h32
}

// Parse a string
func ParseBytes32Ctx(ctx context.Context, s string) (Bytes32, error) {
	h, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return Bytes32{}, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHex, err)
	}
	if len(h) != 32 {
		return Bytes32{}, i18n.NewError(ctx, pldmsgs.MsgTypesValueInvalidHexBytes32, len(h))
	}
	return NewBytes32FromSlice(h), nil
}

func ParseBytes32(s string) (Bytes32, error) {
	pB32, err := ParseBytes32Ctx(context.Background(), s)
	if err != nil {
		return Bytes32{}, err
	}
	return pB32, nil
}

func MustParseBytes32(s string) Bytes32 {
	h, err := ParseBytes32(s)
	if err != nil {
		panic(err)
	}
	return h
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (id Bytes32) String() string {
	return id.HexString0xPrefix()
}

func (id *Bytes32) Equals(id2 *Bytes32) bool {
	if id == nil && id2 == nil {
		return true
	}
	if id == nil || id2 == nil {
		return false
	}
	return *id == *id2
}

// Return the first 16 bytes as a UUID
func (id Bytes32) UUIDFirst16() (u uuid.UUID) {
	copy(u[:], id[0:16])
	return u
}

func Bytes32UUIDFirst16(u uuid.UUID) Bytes32 {
	var v Bytes32
	copy(v[0:16], u[:])
	return v
}

// JSON representation is lower case hex, with 0x prefix
func (id Bytes32) MarshalText() ([]byte, error) {
	return ([]byte)(id.HexString0xPrefix()), nil
}

// Parses with/without 0x in any case
func (id *Bytes32) UnmarshalText(text []byte) error {
	pID, err := ParseBytes32Ctx(context.Background(), string(text))
	if err != nil {
		return err
	}
	*id = pID
	return nil
}

// Get string with 0x prefix - nil is all zeros
func (id Bytes32) HexString0xPrefix() string {
	return fmt.Sprintf("0x%s", hex.EncodeToString(id[:]))
}

// Get string (without 0x prefix) - nil is all zeros
func (id Bytes32) HexString() string {
	return hex.EncodeToString(id[:])
}

// Get bytes - or nil
func (id Bytes32) Bytes() []byte {
	return id[:]
}

// Returns true for either nil, or all-zeros value
func (id Bytes32) IsZero() bool {
	return id == Bytes32{}
}

func (id Bytes32) Value() (driver.Value, error) {
	return id.HexString(), nil // no 0x prefix
}

func (id *Bytes32) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		b, err := ParseBytes32Ctx(context.Background(), v)
		if err != nil {
			return err
		}
		*id = b
		return nil
	case []byte:
		if len(v) == 32 {
			*id = NewBytes32FromSlice(v)
		} else if len(v) == 64 || len(v) == 66 {
			b, err := ParseBytes32Ctx(context.Background(), string(v))
			if err != nil {
				return err
			}
			*id = b
		} else {
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesValueInvalidHexBytes32, len(v))
		}
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, id)
	}
}
