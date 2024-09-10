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
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type Bytes32Field string

func (sf Bytes32Field) SQLColumn() string {
	return (string)(sf)
}

func (sf Bytes32Field) SupportsLIKE() bool {
	return false
}

func (sf Bytes32Field) SQLValue(ctx context.Context, jsonValue tktypes.RawJSON) (driver.Value, error) {
	if jsonValue.IsNil() {
		return nil, nil
	}
	var untyped interface{}
	err := json.Unmarshal(jsonValue, &untyped)
	if err != nil {
		return nil, err
	}
	switch v := untyped.(type) {
	case string:
		b32 := make([]byte, 32)
		bytesRead, err := hex.Decode(b32, ([]byte)(strings.TrimPrefix(v, "0x")))
		if err != nil || bytesRead != 32 {
			return nil, i18n.WrapError(ctx, err, msgs.MsgFiltersValueInvalidHexBytes32, bytesRead)
		}
		return hex.EncodeToString(b32), nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueInvalidForString, string(jsonValue))
	}
}
