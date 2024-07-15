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

package confutil

import "time"

func Int(iVal *int, def int) int {
	if iVal == nil {
		return def
	}
	return *iVal
}

func IntMin(iVal *int, min int, def int) int {
	if iVal == nil || *iVal < min {
		return def
	}
	return *iVal
}

func Int64(iVal *int64, def int64) int64 {
	if iVal == nil {
		return def
	}
	return *iVal
}

func Int64Min(iVal *int64, min int64, def int64) int64 {
	if iVal == nil || *iVal < min {
		return def
	}
	return *iVal
}

func Bool(bVal *bool, def bool) bool {
	if bVal == nil {
		return def
	}
	return *bVal
}

func Duration(sVal *string, def time.Duration) time.Duration {
	var dVal *time.Duration
	if sVal != nil {
		d, err := time.ParseDuration(*sVal)
		if err == nil {
			dVal = &d
		}
	}
	if dVal == nil {
		return def
	}
	return *dVal
}

func P[T any](v T) *T {
	return &v
}
