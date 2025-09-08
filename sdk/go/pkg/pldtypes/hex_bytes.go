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
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

// HexBytes is byte slice that is formatted in JSON with an 0x prefix, and stored in the DB as hex
type HexBytes []byte

// Parse a string
func ParseHexBytes(ctx context.Context, s string) (HexBytes, error) {
	h, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHex, err)
	}
	return h, nil
}

func MustParseHexBytes(s string) HexBytes {
	h, err := ParseHexBytes(context.Background(), s)
	if err != nil {
		panic(err)
	}
	return h
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (id HexBytes) String() string {
	if id == nil {
		return ""
	}
	return id.HexString0xPrefix()
}

func (id HexBytes) Equals(id2 HexBytes) bool {
	return bytes.Equal(id, id2)
}

// JSON representation is lower case hex, with 0x prefix
func (id HexBytes) MarshalText() ([]byte, error) {
	return ([]byte)(id.HexString0xPrefix()), nil
}

// Parses with/without 0x in any case
func (id *HexBytes) UnmarshalText(text []byte) error {
	pID, err := ParseHexBytes(context.Background(), string(text))
	if err != nil {
		return err
	}
	*id = pID
	return nil
}

// Get string with 0x prefix - nil is all zeros
func (id HexBytes) HexString0xPrefix() string {
	if id == nil {
		return (&HexBytes{}).HexString0xPrefix()
	}
	return fmt.Sprintf("0x%s", hex.EncodeToString(id[:]))
}

// Get string (without 0x prefix) - nil is all zeros
func (id HexBytes) HexString() string {
	if id == nil {
		return (&HexBytes{}).HexString()
	}
	return hex.EncodeToString(id[:])
}

func (id HexBytes) Value() (driver.Value, error) {
	if id == nil {
		return nil, nil
	}
	return id.HexString(), nil // no 0x prefix
}

func (id *HexBytes) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		b, err := ParseHexBytes(context.Background(), v)
		if err != nil {
			return err
		}
		*id = b
		return nil
	case []byte:
		*id = HexBytes(v)
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, id)
	}
}
