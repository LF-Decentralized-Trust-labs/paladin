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
	"encoding/hex"
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

// jsonResultToBigInt takes the types that are supported from Unmarshal into interface{}
// and where possible converts into a big integer, including string parsing using the
// default prefix handling (hex with 0x, octal, decimal etc.)
func jsonResultToBigInt(ctx context.Context, jsonResult interface{}) (*big.Int, error) {
	switch v := jsonResult.(type) {
	case string:
		bi, ok := new(big.Int).SetString(v, 0)
		if !ok {
			return nil, i18n.NewError(ctx, msgs.MsgFiltersValueIntStringParseFail, v)
		}
		return bi, nil
	case float64:
		return new(big.Int).SetInt64((int64)(v)), nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueInvalidForBigInt, jsonResult)
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

// PadHexBigIntTwosCompliment returns the supplied buffer, with all the bytes to the left of
// the two's compliment formatted string set to 0
func PadHexBigIntTwosCompliment(bi *big.Int, buff []byte) []byte {
	twosCompliment := abi.SerializeInt256TwosComplementBytes(bi)
	unPadded := hex.EncodeToString(twosCompliment)
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
