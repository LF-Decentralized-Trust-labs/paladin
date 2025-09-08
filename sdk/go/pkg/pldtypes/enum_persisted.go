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
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

type EnumStringOptions interface {
	~string
	Options() []string
}

type EnumStringDefault interface {
	Default() string
}

// Enum is a persistence wrapper for an enum with a set of options
type Enum[O EnumStringOptions] string

func (p Enum[O]) V() O {
	return O(p)
}

func (p Enum[O]) Options() []string {
	return O(p).Options()
}

// Case insensitive validation, with default, returning a string value
func (p Enum[O]) MapToString() (string, error) {
	ps, err := p.Validate()
	return string(ps), err
}

// Case insensitive validation, with default
func (p Enum[O]) Validate() (O, error) {
	validator := (*new(O))
	if p == "" {
		var iVal any = validator
		enumDefault, ok := iVal.(EnumStringDefault)
		if ok {
			return O(enumDefault.Default()), nil
		}
	}
	for _, o := range validator.Options() {
		if strings.EqualFold(o, (string)(p)) {
			return O(o), nil
		}
	}
	return "", i18n.NewError(context.Background(), pldmsgs.MsgTypesEnumValueInvalid, strings.Join(validator.Options(), ","))
}

func MapEnum[O EnumStringOptions, T any](p Enum[O], mapped map[O]T) (ret T, err error) {
	v, err := p.Validate()
	if err != nil {
		return
	}
	ret, ok := mapped[v]
	if !ok {
		allOpts := (*new(O)).Options()
		mappedOpts := []string{}
		for k := range mapped {
			for _, o := range allOpts {
				if string(k) == o {
					mappedOpts = append(mappedOpts, o)
				}
			}
		}
		err = i18n.NewError(context.Background(), pldmsgs.MsgTypesEnumValueInvalid, strings.Join(mappedOpts, ","))
		return
	}
	return
}

// SQL valuer returns a string, and only allows the possible values
func (p Enum[O]) Value() (driver.Value, error) {
	return p.MapToString()
}

// SQL scanner handles strings, bytes, and nil - where nil will be set to the default
func (p *Enum[O]) Scan(src interface{}) error {
	switch s := src.(type) {
	case string:
		*p = Enum[O](s)
	case []byte:
		*p = Enum[O](s)
	case nil:
		*p = ""
	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesScanFail, src, "")
	}
	validated, err := p.Validate()
	if err != nil {
		return err
	}
	*p = Enum[O](validated)
	return nil
}
