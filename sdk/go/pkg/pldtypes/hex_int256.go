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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/pldmsgs"
	"github.com/hyperledger-firefly/signer/pkg/abi"
)

// HexInt256 is a signed integer in the range -2^255 to 2^255-1, serialized to the DB using a
// 65 character sortable string (a 0/1 sign character, followed by 32 hex bytes of two's complement)
type HexInt256 big.Int

var (
	int256Min = new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 255))
	int256Max = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))
)

// checkInt256Range enforces the range of the type. The DB serialization is exactly 32 bytes of
// two's complement, so the same check covers both parsing a value and persisting one.
func checkInt256Range(ctx context.Context, bi *big.Int) error {
	if bi.Cmp(int256Min) < 0 || bi.Cmp(int256Max) > 0 {
		return i18n.NewError(ctx, pldmsgs.MsgTypesInt256OutOfRange, bi.Text(10))
	}
	return nil
}

func Int64ToInt256(v int64) *HexInt256 {
	return (*HexInt256)(new(big.Int).SetInt64(v))
}

// Parse a string
func ParseHexInt256(ctx context.Context, s string) (*HexInt256, error) {
	bi, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesInvalidHexInteger, s)
	}
	if err := checkInt256Range(ctx, bi); err != nil {
		return nil, err
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
	text, ok := jsonNumericText(b)
	if !ok {
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, string(b), hi)
	}
	return hi.setJSONString(text)
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
	s, err := Int256To65CharDBSafeSortableString(context.Background(), (*big.Int)(hi))
	if err != nil {
		return nil, err
	}
	return s, nil
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

func Int256To65CharDBSafeSortableString(ctx context.Context, bi *big.Int) (string, error) {
	sign := bi.Sign()
	signPlusZeroPaddedInt256, err := PadHexBigIntTwosComplement(ctx, bi, make([]byte, 65))
	if err != nil {
		return "", err
	}
	if sign < 0 {
		signPlusZeroPaddedInt256[0] = '0'
	} else {
		// Zero or positive get a "1" in the first string position, which makes them
		signPlusZeroPaddedInt256[0] = '1'
	}
	return (string)(signPlusZeroPaddedInt256), nil
}

// PadHexBigIntTwosComplement returns the supplied buffer, containing the two's complement
// formatted string sign-extended to the left - with 'f' for a negative integer, and '0' for
// zero or a positive one. The supplied integer is not modified.
//
// The integer must be in the range of an int256, and its encoding must fit within the supplied
// buffer. Both are errors, rather than the silently wrapped or truncated - and so incorrect -
// encoding that an out of range value would otherwise produce
func PadHexBigIntTwosComplement(ctx context.Context, bi *big.Int, buff []byte) ([]byte, error) {
	if err := checkInt256Range(ctx, bi); err != nil {
		return nil, err
	}
	twosComplement := abi.SerializeInt256TwosComplementBytes(bi)
	unPadded := hex.EncodeToString(twosComplement)
	boundary := len(buff) - len(unPadded)
	if boundary < 0 {
		return nil, i18n.NewError(ctx, pldmsgs.MsgTypesHexIntBufferTooSmall, len(buff), len(unPadded))
	}
	signExtend := byte('0')
	if bi.Sign() < 0 {
		signExtend = 'f'
	}
	for i := 0; i < len(buff); i++ {
		if i >= boundary {
			buff[i] = unPadded[i-boundary]
		} else {
			buff[i] = signExtend
		}
	}
	return buff, nil
}
