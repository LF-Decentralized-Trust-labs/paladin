// Copyright © 2025 Kaleido, Inc.
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

	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
)

const MaxPLDBigIntHexLength = 65

// PLDBigInt is a wrapper on a Go big.Int that standardizes JSON and DB serialization
type PLDBigInt big.Int

func (i PLDBigInt) MarshalText() ([]byte, error) {
	// Represent as base 10 string in Marshalled JSON
	// This could become configurable to other options, such as:
	// - Hex formatted string
	// - Number up to max float64, then string if larger
	return []byte((*big.Int)(&i).Text(10)), nil
}

func (i *PLDBigInt) UnmarshalJSON(b []byte) error {
	var val interface{}
	if err := json.Unmarshal(b, &val); err != nil {
		return i18n.WrapError(context.Background(), err, pldmsgs.MsgBigIntParseFailed, b)
	}
	switch val := val.(type) {
	case string:
		if _, ok := i.Int().SetString(val, 0); !ok {
			return i18n.NewError(context.Background(), pldmsgs.MsgBigIntParseFailed, b)
		}
		return nil
	case float64:
		i.Int().SetInt64(int64(val))
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgBigIntParseFailed, b)
	}
}

func NewPLDBigInt(x int64) *PLDBigInt {
	return (*PLDBigInt)(big.NewInt(x))
}

func (i PLDBigInt) Value() (driver.Value, error) {
	// Represent as base 16 string in database, to allow a 64 character limit
	hexValue := (*big.Int)(&i).Text(16)
	if len(hexValue) > MaxPLDBigIntHexLength {
		return nil, i18n.NewError(context.Background(), pldmsgs.MsgBigIntTooLarge, len(hexValue), MaxPLDBigIntHexLength)
	}
	// Pad left after the sign digit to the max len to allow alphabetical sorting to work correctly
	targetLen := MaxPLDBigIntHexLength - 1
	src := []byte(hexValue)
	buff := make([]byte, 0, MaxPLDBigIntHexLength)
	if len(src) > 0 && src[0] == '-' {
		buff = append(buff, '-')
		src = src[1:]
		targetLen++
	}
	padLen := targetLen - len(hexValue)
	for i := 0; i < padLen; i++ {
		buff = append(buff, '0')
	}
	for i := 0; i < len(src); i++ {
		buff = append(buff, src[i])
	}
	return string(buff), nil
}

func (i *PLDBigInt) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		return nil
	case string:
		if src == "" {
			return nil
		}
		// Scan is different to JSON deserialization - always read as HEX (without any 0x prefix)
		if _, ok := i.Int().SetString(src, 16); !ok {
			return i18n.NewError(context.Background(), pldmsgs.MsgTypeRestoreFailed, src, i)
		}
		return nil
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypeRestoreFailed, src, i)
	}
}

func (i *PLDBigInt) Int() *big.Int {
	return (*big.Int)(i)
}

func (i *PLDBigInt) Int64() int64 {
	if i == nil {
		return 0
	}
	return (*big.Int)(i).Int64()
}

func (i *PLDBigInt) Uint64() uint64 {
	if i == nil || !(*big.Int)(i).IsUint64() {
		return 0
	}
	return (*big.Int)(i).Uint64()
}

func (i *PLDBigInt) Equals(i2 *PLDBigInt) bool {
	switch {
	case i == nil && i2 == nil:
		return true
	case i == nil || i2 == nil:
		return false
	default:
		return (*big.Int)(i).Cmp((*big.Int)(i2)) == 0
	}
}

func (i *PLDBigInt) String() string {
	return (*big.Int)(i).String()
}
