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
	"unicode"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

const DefaultNameMaxLen = 128

func ValidateSafeCharsStartEndAlphaNum(ctx context.Context, val string, maxLen int, fieldName string) error {
	valid := len(val) > 0 && len(val) <= maxLen
	for i, c := range val {
		if !valid {
			break
		}
		switch {
		case c <= unicode.MaxASCII && (unicode.IsLetter(c) || unicode.IsNumber(c)):
		case i != 0 && i != (len(val)-1) && (c == '.' || c == '_' || c == '-'):
		default:
			valid = false
		}
	}
	if !valid {
		return i18n.NewError(ctx, pldmsgs.MsgTypesInvalidNameSafeCharAlphaBoxed, fieldName, maxLen, val)
	}
	return nil
}

func StrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
