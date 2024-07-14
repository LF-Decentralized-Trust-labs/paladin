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

package statestore

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"golang.org/x/crypto/sha3"
)

// HashID is a 32 byte value, optimized for DB storage using a compound key
// of two UUIDs (which is faster than either bytea(16) or char(32) it seems)
type HashID struct {
	L uuid.UUID `gorm:"type:uuid;"`
	H uuid.UUID `gorm:"type:uuid;"`
}

func NewHashID(bytes [32]byte) *HashID {
	h := &HashID{}
	copy(h.L[:], bytes[0:16])
	copy(h.H[:], bytes[16:32])
	return h
}

// No checking in this function on length
func NewHashIDSlice32(bytes []byte) *HashID {
	h := &HashID{}
	copy(h.L[:], bytes[0:16])
	copy(h.H[:], bytes[16:32])
	return h
}

func HashIDKeccak(b []byte) *HashID {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(b)
	var h32 [32]byte
	_ = hash.Sum(h32[0:0])
	return NewHashID(h32)
}

// Parse a string
func ParseHashID(ctx context.Context, s string) (*HashID, error) {
	h, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgStateInvalidHex, err)
	}
	if len(h) != 32 {
		return nil, i18n.NewError(ctx, msgs.MsgStateInvalidLength, 32, len(h))
	}
	return NewHashIDSlice32(h), nil
}

func MustParseHashID(s string) *HashID {
	h, err := ParseHashID(context.Background(), s)
	if err != nil {
		panic(err)
	}
	return h
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (id *HashID) String() string {
	if id == nil {
		return ""
	}
	return id.HexString0xPrefix()
}

// JSON representation is lower case hex, with 0x prefix
func (id HashID) MarshalText() ([]byte, error) {
	return ([]byte)(id.HexString0xPrefix()), nil
}

// Parses with/without 0x in any case
func (id *HashID) UnmarshalText(text []byte) error {
	pID, err := ParseHashID(context.Background(), string(text))
	if err != nil {
		return err
	}
	*id = *pID
	return nil
}

// Get string with 0x prefix - nil is all zeros
func (id *HashID) HexString0xPrefix() string {
	if id == nil {
		return (&HashID{}).HexString0xPrefix()
	}
	return fmt.Sprintf("0x%s%s", hex.EncodeToString(id.L[:]), hex.EncodeToString(id.H[:]))
}

// Get string (without 0x prefix) - nil is all zeros
func (id *HashID) HexString() string {
	if id == nil {
		return (&HashID{}).HexString()
	}
	return fmt.Sprintf("%s%s", hex.EncodeToString(id.L[:]), hex.EncodeToString(id.H[:]))
}

// Get bytes - or nil
func (id *HashID) Bytes() []byte {
	if id == nil {
		return nil
	}
	var b32 [32]byte
	copy(b32[0:16], id.L[:])
	copy(b32[16:32], id.H[:])
	return b32[:]
}

// Get bytes32 (or zero value)
func (id *HashID) Bytes32() [32]byte {
	var b32 [32]byte
	if id != nil {
		copy(b32[0:16], id.L[:])
		copy(b32[16:32], id.H[:])
	}
	return b32
}

// Returns true for either nil, or all-zeros value
func (id *HashID) IsZero() bool {
	return id == nil || *id == HashID{}
}
