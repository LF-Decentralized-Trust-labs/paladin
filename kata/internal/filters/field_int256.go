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

package filters

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"math/big"

	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type Int256Field string

func (sf Int256Field) SQLColumn() string {
	return (string)(sf)
}

func (sf Int256Field) SupportsLIKE() bool {
	return false
}

func (sf Int256Field) SQLValue(ctx context.Context, jsonValue types.RawJSON) (driver.Value, error) {
	if jsonValue.IsNil() {
		return nil, nil
	}
	var jsonResult interface{}
	err := json.Unmarshal(jsonValue, &jsonResult)
	if err != nil {
		return nil, err
	}
	bi, err := jsonResultToBigInt(ctx, jsonResult)
	if err != nil {
		return "", err
	}
	return Int256ToFilterString(ctx, bi), nil
}

func Int256ToFilterString(ctx context.Context, bi *big.Int) string {
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
