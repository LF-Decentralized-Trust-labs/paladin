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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"gopkg.in/yaml.v3"
)

// Just like pldtypes.RawJSON, but with ability to SQL serialize to string as well
type RawJSON []byte

func JSONString(s any) RawJSON {
	b, _ := json.Marshal(s)
	return b
}

func (m RawJSON) String() string {
	b, _ := m.MarshalJSON()
	return (string)(b)
}

func (m RawJSON) Bytes() []byte {
	return m
}

func (m RawJSON) StringValue() string {
	if m == nil {
		return ""
	}
	var v any
	_ = json.Unmarshal(m, &v)
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		var n json.Number
		_ = json.Unmarshal(m, &n)
		return n.String()
	default:
		return m.String()
	}
}

func (m RawJSON) Pretty() string {
	b, err := m.MarshalJSON()
	var val interface{}
	if err == nil {
		err = json.Unmarshal(b, &val)
	}
	if err == nil {
		b, err = json.MarshalIndent(val, "", "  ")
	}
	if err != nil {
		b, _ = json.Marshal(err.Error())
	}
	return (string)(b)
}

func (m RawJSON) YAML() string {
	b, err := m.MarshalJSON()
	var val interface{}
	if err == nil {
		err = json.Unmarshal(b, &val)
	}
	if err == nil {
		b, err = yaml.Marshal(val)
	}
	if err != nil {
		b, _ = json.Marshal(err.Error())
	}
	return (string)(b)
}

func (m RawJSON) MarshalJSON() ([]byte, error) {
	return m.BytesOrNull(), nil
}

func (m RawJSON) BytesOrNull() []byte {
	if len(m) == 0 {
		return []byte("null")
	}
	return m
}

func (m *RawJSON) UnmarshalJSON(data []byte) error {
	if m == nil {
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesUnmarshalNil)
	}
	*m = append((*m)[0:0], data...)
	return nil
}

func (m RawJSON) IsNil() bool {
	return m == nil || (string)(m) == "null"
}

func (m RawJSON) Value() driver.Value {
	// Ensure null goes to a null value in the DB (not the string "null")
	if m.IsNil() {
		return nil
	}
	return (string)(m)
}

func (m RawJSON) ToMap() (jm map[string]any) {
	_ = json.Unmarshal(m, &jm)
	if jm == nil {
		jm = map[string]any{}
	}
	return jm
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
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, m)
	}
}
