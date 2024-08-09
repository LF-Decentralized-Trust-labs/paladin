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
	"encoding/json"
	"reflect"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

type JSONP[T any] struct {
	v T
}

func (p *JSONP[T]) V() T {
	if p == nil {
		return JSONP[T]{}.v
	}
	return p.v
}

func (p *JSONP[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.v)
}

func (p *JSONP[T]) UnmarshalJSON(data []byte) error {
	*p = JSONP[T]{}
	return json.Unmarshal(data, &p.v)
}

// Safe nil checking on an interface, that does not panic
func IsNil(v interface{}) bool {
	tv := reflect.ValueOf(v)
	switch tv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return tv.IsNil()
	}
	return false
}

func (p *JSONP[T]) Value() (driver.Value, error) {
	// Ensure null goes to a null value in the DB (not the string "null")
	if p == nil || IsNil(p.v) {
		return nil, nil
	}
	return json.Marshal(p.v)
}

func (p *JSONP[T]) Scan(src interface{}) error {
	*p = JSONP[T]{}
	var b []byte
	switch s := src.(type) {
	case string:
		b = ([]byte)(s)
	case []byte:
		b = s
	case nil:
		return nil
	default:
		return i18n.NewError(context.Background(), msgs.MsgTypesScanFail, src, p.v)
	}
	return json.Unmarshal(b, &p.v)
}
