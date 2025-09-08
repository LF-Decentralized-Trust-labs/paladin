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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

// HexInt256 is any integer (signed or unsigned) up to 256 bits in size, serialized to the DB using a 65 sortable string (a 0/1 sign character, followed by 32 hex bytes)
type HexInt256 big.Int

func Int64ToInt256(v int64) *HexUint256 {
	return (*HexUint256)(new(big.Int).SetInt64(v))
}

// Parse a string
func ParseHexInt256(ctx context.Context, s string) (*HexInt256, error) {
	bi, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHexInteger, s)
	}
	return (*HexInt256)(bi), nil
}

func MustParseHexInt256(s string) *HexInt256 {
	hi, err := ParseHexInt256(context.Background(), s)
	if err != nil {
		panic(err)
	}
	return hi
}

// Natural string representation is HexString0xPrefix() if non-nil, or empty string if ""
func (hi *HexInt256) String() string {
	return hi.HexString0xPrefix()
}

// JSON representation is lower case hex, with 0x prefix
func (hi *HexInt256) MarshalJSON() ([]byte, error) {
	return json.Marshal(hi.HexString0xPrefix())
}

func (hi *HexInt256) setJSONString(text string) error {
	pID, err := ParseHexInt256(context.Background(), string(text))
	if err != nil {
		return err
	}
	*hi = *pID
	return nil
}

// Parses with/without 0x in any case
func (hi *HexInt256) UnmarshalJSON(b []byte) error {
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

func (hi *HexInt256) Int() *big.Int {
	return (*big.Int)(hi)
}

func (hi *HexInt256) NilOrZero() bool {
	return hi == nil || hi.Int().Sign() == 0
}

// Get string with 0x prefix - nil is all zeros
func (hi *HexInt256) HexString0xPrefix() string {
	absHi := new(big.Int).Abs(hi.Int())
	sign := ""
	if hi.Int().Sign() < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s0x%s", sign, absHi.Text(16))
}

// Get string (without 0x prefix) - nil is all zeros
func (hi *HexInt256) HexString() string {
	return hi.Int().Text(16)
}

func (hi *HexInt256) Value() (driver.Value, error) {
	if hi == nil {
		return nil, nil
	}
	return Int256To65CharDBSafeSortableString((*big.Int)(hi)), nil
}

func (hi *HexInt256) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		if len(v) != 65 {
			// This type was not used to serialize to the database
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesInvalidDBInt256, v)
		}
		b, err := hex.DecodeString(v[1:])
		if err != nil {
			return i18n.WrapError(context.Background(), err, pldmsgs.MsgTypesInvalidDBInt256, v)
		}
		bi := abi.ParseInt256TwosComplementBytes(b)
		*hi = HexInt256(*bi)
		return nil
	case int64:
		*hi = (HexInt256)(*big.NewInt(v))
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, hi)
	}
}

func Int256To65CharDBSafeSortableString(bi *big.Int) string {
	sign := bi.Sign()
	signPlusZeroPaddedInt256 := PadHexBigIntTwosComplement(bi, make([]byte, 65))
	if sign < 0 {
		signPlusZeroPaddedInt256[0] = '0'
	} else {
		// Zero or positive get a "1" in the first string position, which makes them
		signPlusZeroPaddedInt256[0] = '1'
	}
	return (string)(signPlusZeroPaddedInt256)
}

// PadHexBigIntTwosComplement returns the supplied buffer, with all the bytes to the left of
// the two's complement formatted string set to 0
func PadHexBigIntTwosComplement(bi *big.Int, buff []byte) []byte {
	twosComplement := abi.SerializeInt256TwosComplementBytes(bi)
	unPadded := hex.EncodeToString(twosComplement)
	boundary := len(buff) - len(unPadded)
	for i := 0; i < len(buff); i++ {
		if i >= boundary {
			buff[i] = unPadded[i-boundary]
		} else {
			buff[i] = 'f'
		}
	}
	return buff
}
