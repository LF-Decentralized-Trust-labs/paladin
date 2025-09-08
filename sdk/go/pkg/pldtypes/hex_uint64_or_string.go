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
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

// HexUint64OrString is used for things like block numbers, where you can provide a number or a string like "latest".
type HexUint64OrString string

// Parses with/without 0x in any case
func (his *HexUint64OrString) UnmarshalJSON(b []byte) error {
	var iVal interface{}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber() // It's not safe
	err := decoder.Decode(&iVal)
	if err == nil {
		switch v := iVal.(type) {
		case string:
			*his = HexUint64OrString(v)
		case json.Number:
			*his = HexUint64OrString(v.String())
		default:
			err = i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, iVal, his)
		}
	}
	return err
}

func (his HexUint64OrString) MarshalJSON() ([]byte, error) {
	return json.Marshal(his.String())
}

func (his HexUint64OrString) String() string {
	intVal, intErr := ParseHexUint64(context.Background(), string(his))
	if intErr == nil {
		// If it parses as a number - we use the normalized 0x hex value as our value
		return intVal.String()
	}
	return string(his)
}
