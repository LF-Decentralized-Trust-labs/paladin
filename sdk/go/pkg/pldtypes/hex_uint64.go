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
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

const MAX_SAFE_INT64 = 0x7fffffffffffffff

// HexUint64 is an unsigned integer that is serialized in JSON as hex
type HexUint64 uint64

// Parse a string
func ParseHexUint64(ctx context.Context, s string) (HexUint64, error) {
	bi, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return 0, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHexInteger, s)
	}
	if !bi.IsUint64() {
		return 0, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidUint64, s)
	}
	return HexUint64(bi.Uint64()), nil
}

func MustParseHexUint64(s string) HexUint64 {
	hi, err := ParseHexUint64(context.Background(), s)
	if err != nil {
		panic(err)
	}
	return hi
}

func (hi HexUint64) Uint64() uint64 {
	return uint64(hi)
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (hi HexUint64) String() string {
	return hi.HexString0xPrefix()
}

// JSON representation is lower case hex, with 0x prefix
func (hi HexUint64) MarshalJSON() ([]byte, error) {
	return json.Marshal(hi.HexString0xPrefix())
}

func (hi *HexUint64) setString(text string) error {
	pID, err := ParseHexUint64(context.Background(), string(text))
	if err != nil {
		return err
	}
	*hi = pID
	return nil
}

// Parses with/without 0x in any case
func (hi *HexUint64) UnmarshalJSON(b []byte) error {
	var iVal interface{}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber() // It's not safe to use a JSON number decoder as it uses float64, so can (and does) lose precision
	err := decoder.Decode(&iVal)
	if err == nil {
		err = hi.Scan(iVal)
	}
	return err
}

// Get string with 0x prefix - nil is all zeros
func (hi HexUint64) HexString0xPrefix() string {
	return fmt.Sprintf("0x%s", strconv.FormatUint(uint64(hi), 16))
}

// Get string (without 0x prefix) - nil is all zeros
func (hi HexUint64) HexString() string {
	return strconv.FormatUint(uint64(hi), 16)
}

func (hi HexUint64) Value() (driver.Value, error) {
	// Check not too large for DB - which does not have unsigned numerics
	if hi > MAX_SAFE_INT64 {
		return nil, i18n.NewError(context.Background(), pldmsgs.MsgTypesInvalidDBInt64, strconv.FormatUint(uint64(hi), 10))
	}
	return int64(hi), nil
}

func (id *HexUint64) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return id.setString(v)
	case json.Number:
		return id.setString(v.String())
	case int64:
		*id = HexUint64(v)
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, id)
	}
}
