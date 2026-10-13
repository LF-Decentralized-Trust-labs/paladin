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
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
)

// HexUint256 is an unsigned integer of at most 256 bits, serialized to the DB as a 64 character
// string of zero-padded hex, which sorts naturally as an unsigned value
type HexUint256 big.Int

func Uint64ToUint256(v uint64) *HexUint256 {
	return (*HexUint256)(new(big.Int).SetUint64(v))
}

// Parse a string
func ParseHexUint256(ctx context.Context, s string) (*HexUint256, error) {
	bi, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHexInteger, s)
	}
	if err := checkUint256Range(ctx, bi); err != nil {
		return nil, err
	}
	return (*HexUint256)(bi), nil
}

// checkUint256Range enforces the range of the type. The DB serialization is exactly 256
// unsigned bits, so the same check covers both parsing a value and persisting one.
func checkUint256Range(ctx context.Context, bi *big.Int) error {
	switch {
	case bi.Sign() < 0:
		return i18n.NewError(ctx, pldmsgs.MsgTypesUint256Negative, bi.Text(10))
	case bi.BitLen() > 256:
		return i18n.NewError(ctx, pldmsgs.MsgTypesUint256TooLarge, bi.Text(10))
	default:
		return nil
	}
}

func MustParseHexUint256(s string) *HexUint256 {
	hi, err := ParseHexUint256(context.Background(), s)
	if err != nil {
		panic(err)
	}
	return hi
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (hi *HexUint256) String() string {
	return hi.HexString0xPrefix()
}

// JSON representation is lower case hex, with 0x prefix
func (hi *HexUint256) MarshalJSON() ([]byte, error) {
	return json.Marshal(hi.HexString0xPrefix())
}

func (hi *HexUint256) setJSONString(text string) error {
	pID, err := ParseHexUint256(context.Background(), string(text))
	if err != nil {
		return err
	}
	*hi = *pID
	return nil
}

// Parses with/without 0x in any case
func (hi *HexUint256) UnmarshalJSON(b []byte) error {
	text, ok := jsonNumericText(b)
	if !ok {
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, string(b), hi)
	}
	return hi.setJSONString(text)
}

func (hi *HexUint256) Int() *big.Int {
	return (*big.Int)(hi)
}

func (hi *HexUint256) NilOrZero() bool {
	return hi == nil || hi.Int().Sign() == 0
}

// Get string with 0x prefix - nil is all zeros
func (hi *HexUint256) HexString0xPrefix() string {
	i := hi.Int()
	// Avoid allocating a new big.Int for Abs in the common non-negative case
	str := i.Text(16)
	sign := ""
	if i.Sign() < 0 {
		// A negative can only have been supplied by conversion, as no constructor of this
		// type accepts one. Report it as what it is rather than as its absolute value
		sign = "-"
		str = new(big.Int).Abs(i).Text(16)
	}
	if len(str)%2 != 0 {
		return sign + "0x0" + str
	}
	return sign + "0x" + str
}

// Get string (without 0x prefix) - nil is all zeros
func (hi *HexUint256) HexString() string {
	return hi.Int().Text(16)
}

func (hi *HexUint256) Value() (driver.Value, error) {
	if hi == nil {
		return nil, nil
	}
	buff, err := PadHexBigUint(context.Background(), (*big.Int)(hi), make([]byte, 64))
	if err != nil {
		return nil, err
	}
	return string(buff), nil
}

func (hi *HexUint256) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		bi, ok := new(big.Int).SetString(v, 16)
		// Note SetString accepts a leading sign, where Value() writes 64 hex digits and no
		// sign. A '-' would otherwise scan into a negative, outside the range of this type
		if len(v) != 64 || v[0] == '-' || v[0] == '+' || !ok {
			// This type was not used to serialize to the database
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesInvalidDBUint256, v)
		}
		*hi = (HexUint256)(*bi)
		return nil
	case int64:
		if v < 0 {
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesUint256Negative, strconv.FormatInt(v, 10))
		}
		*hi = (HexUint256)(*big.NewInt(v))
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, hi)
	}
}

// PadHexBigUint returns the supplied buffer, with all the bytes to the left of the integer set to '0'.
// The supplied integer is not modified.
//
// The integer must be in the range of a uint256, and its encoding must fit within the supplied
// buffer. Both are errors, rather than the silently truncated - and so incorrect - encoding that
// a value too large for the buffer would otherwise produce
func PadHexBigUint(ctx context.Context, bi *big.Int, buff []byte) ([]byte, error) {
	if err := checkUint256Range(ctx, bi); err != nil {
		return nil, err
	}
	unPadded := bi.Text(16)
	boundary := len(buff) - len(unPadded)
	if boundary < 0 {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesHexIntBufferTooSmall, len(buff), len(unPadded))
	}
	for i := 0; i < len(buff); i++ {
		if i >= boundary {
			buff[i] = unPadded[i-boundary]
		} else {
			buff[i] = '0'
		}
	}
	return buff, nil
}
