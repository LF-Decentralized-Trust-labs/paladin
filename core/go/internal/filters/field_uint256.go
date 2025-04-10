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
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/sdk/go/pkg/pldtypes"
)

type Uint256Field string

func (sf Uint256Field) SQLColumn() string {
	return (string)(sf)
}

func (sf Uint256Field) SupportsLIKE() bool {
	return false
}

func (sf Uint256Field) SQLValue(ctx context.Context, jsonValue pldtypes.RawJSON) (driver.Value, error) {
	if jsonValue.IsNil() {
		return nil, nil
	}
	bi, err := ethtypes.UnmarshalBigInt(ctx, jsonValue)
	if err != nil {
		return "", err
	}
	return Uint256ToFilterString(ctx, bi), nil
}

func Uint256ToFilterString(ctx context.Context, bi *big.Int) string {
	zeroPaddedUint256 := pldtypes.PadHexBigUint(bi, make([]byte, 64))
	return (string)(zeroPaddedUint256)
}
