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

package types

import (
	"context"
	"database/sql/driver"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

// Just like types.RawJSON, but with ability to SQL serialize to string as well
type RawJSON []byte

func (m RawJSON) String() string {
	b, _ := m.MarshalJSON()
	return (string)(b)
}

func (m RawJSON) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

func (m *RawJSON) UnmarshalJSON(data []byte) error {
	if m == nil {
		return i18n.NewError(context.Background(), msgs.MsgTypesUnmarshalNil)
	}
	*m = append((*m)[0:0], data...)
	return nil
}

func (m RawJSON) Value() driver.Value {
	// Ensure null goes to a null value in the DB (not the string "null")
	if m == nil || (string)(m) == "null" {
		return nil
	}
	return (string)(m)
}

func (m *RawJSON) Scan(src interface{}) error {
	switch s := src.(type) {
	case string:
		*m = ([]byte)(s)
		return nil
	case []byte:
		*m = s
		return nil
	case nil:
		*m = nil
		return nil
	default:
		return i18n.NewError(context.Background(), msgs.MsgTypesScanFail, src, m)
	}
}
