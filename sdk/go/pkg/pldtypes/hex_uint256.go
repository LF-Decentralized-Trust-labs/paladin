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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

// HexUint256 is any integer (signed or unsigned) up to 256 bits in size, serialized to the DB using a 65 sortable string (a 0/1 sign character, followed by 32 hex bytes)
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
	return (*HexUint256)(bi), nil
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
	var iVal interface{}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber() // It's not safe to use a JSON number decoder as it uses float64, so can (and does) lose precision
	err := decoder.Decode(&iVal)
	if err == nil {
		// Note JSON string decoding rules are NOT the same as DB decoding rules in Scan below
		switch v := iVal.(type) {
		case string:
			err = hi.setJSONString(v)
		case json.Number:
			err = hi.setJSONString(v.String())
		default:
			err = i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, iVal, hi)
		}
	}
	return err
}

func (hi *HexUint256) Int() *big.Int {
	return (*big.Int)(hi)
}

func (hi *HexUint256) NilOrZero() bool {
	return hi == nil || hi.Int().Sign() == 0
}

// Get string with 0x prefix - nil is all zeros
func (hi *HexUint256) HexString0xPrefix() string {
	absHi := new(big.Int).Abs(hi.Int())
	str := absHi.Text(16)
	if len(str)%2 != 0 {
		str = "0" + str
	}
	return fmt.Sprintf("0x%s", str)
}

// Get string (without 0x prefix) - nil is all zeros
func (hi *HexUint256) HexString() string {
	return hi.Int().Text(16)
}

func (hi *HexUint256) Value() (driver.Value, error) {
	if hi == nil {
		return nil, nil
	}
	return string(PadHexBigUint((*big.Int)(hi), make([]byte, 64))), nil
}

func (hi *HexUint256) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		bi, ok := new(big.Int).SetString(v, 16)
		if len(v) != 64 || !ok {
			// This type was not used to serialize to the database
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesInvalidDBUint256, v)
		}
		*hi = (HexUint256)(*bi)
		return nil
	case int64:
		*hi = (HexUint256)(*big.NewInt(v))
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, hi)
	}
}

// PadHexBigUint returns the supplied buffer, with all the bytes to the left of the integer set to '0'
func PadHexBigUint(bi *big.Int, buff []byte) []byte {
	unPadded := bi.Abs(bi).Text(16)
	boundary := len(buff) - len(unPadded)
	for i := 0; i < len(buff); i++ {
		if i >= boundary {
			buff[i] = unPadded[i-boundary]
		} else {
			buff[i] = '0'
		}
	}
	return buff
}
