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
	"regexp"
	"strings"
)

func sqlLikeToRegexp(likeStr string, caseInsensitive bool, escapeChar rune) (*regexp.Regexp, error) {
	buff := new(strings.Builder)
	lastChar := rune(0)
	if caseInsensitive {
		buff.WriteString("(?i)")
	}
	buff.WriteRune('^')
	for _, c := range likeStr {
		clearEscape := false
		switch c {
		case escapeChar:
			if lastChar == escapeChar {
				// Assume regexp escape needed for the SQL escapeChar
				buff.WriteRune('\\')
				buff.WriteRune(escapeChar)
				// Clear the escape, rather than continuing it for the next char
				clearEscape = true
			}
		case '.', '^', '$', '*', '+', '-', '?', '(', ')', '[', ']', '{', '}', '|':
			// Escape this char in the regexp
			buff.WriteRune('\\')
			buff.WriteRune(c)
		case '_':
			if lastChar == escapeChar {
				// This was escaped in the source
				buff.WriteRune('_')
			} else {
				// Match a single character
				buff.WriteRune('.')
			}
		case '%':
			if lastChar == escapeChar {
				// This was escaped in the source
				buff.WriteRune('%')
			} else {
				// Do a lazy match
				buff.WriteString(".*?")
			}
		default:
			// Plain old character
			buff.WriteRune(c)
		}
		if clearEscape {
			lastChar = 0
		} else {
			lastChar = c
		}
	}
	buff.WriteRune('$')
	return regexp.Compile(buff.String())
}
