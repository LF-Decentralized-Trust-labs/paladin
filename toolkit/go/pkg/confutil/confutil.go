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

import (
	"io/fs"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/docker/go-units"
)

/*********** THIS FILE CONTAINS VERY SIMPLE HELPER FUNCTIONS FOR ACCESSING CONFIG AND DUPLICATING CODE **************/
// It shouldn't be confused with a full configuration processing system - that's separate (see "componentmgr")
// Most packages depend on this package, including the "log" package - so we can't use the logging framework.
// For that reason avoid placing any heavy lifting code here.

func Int(iVal *int, def int) int {
	if iVal == nil {
		return def
	}
	return *iVal
}

func IntMin(iVal *int, min int, def int) int {
	if iVal == nil {
		return def
	} else if *iVal < min {
		return min
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
	if iVal == nil {
		return def
	} else if *iVal < min {
		return min
	}
	return *iVal
}

func Float64Min(iVal *float64, min float64, def float64) float64 {
	if iVal == nil {
		return def
	} else if *iVal < min {
		return min
	}
	return *iVal
}

func Bool(bVal *bool, def bool) bool {
	if bVal == nil {
		return def
	}
	return *bVal
}

func StringNotEmpty(sVal *string, def string) string {
	if sVal == nil || *sVal == "" {
		return def
	}
	return *sVal
}

func StringOrEmpty(sVal *string, def string) string {
	if sVal == nil {
		return def
	}
	return *sVal
}

func StringSlice(sVal []string, def []string) []string {
	if sVal == nil {
		return def
	}
	return sVal
}

func UnixFileMode(sVal *string, def string) fs.FileMode {
	var iVal *fs.FileMode
	if sVal != nil {
		i64, err := strconv.ParseUint(*sVal, 8, 32)
		if err == nil {
			i := fs.FileMode(i64)
			iVal = &i
		}
	}
	if iVal == nil || *iVal > 0777 {
		i64, _ := strconv.ParseUint(def, 8, 32)
		i := fs.FileMode(i64)
		iVal = &i
	}
	return *iVal
}

func DurationMin(sVal *string, min time.Duration, def string) time.Duration {
	var dVal *time.Duration
	if sVal != nil {
		d, err := time.ParseDuration(*sVal)
		if err == nil {
			dVal = &d
		}
	}
	if dVal == nil {
		defDuration, _ := time.ParseDuration(def)
		dVal = &defDuration
	} else if *dVal < min {
		return min
	}
	return *dVal
}

func BigInt(sVal *string, def string) *big.Int {
	var biVal *big.Int
	if sVal != nil {
		bi, ok := new(big.Int).SetString(*sVal, 0)
		if ok {
			biVal = bi
		}
	}
	if biVal == nil {
		biVal, _ = new(big.Int).SetString(def, 0)
	}
	return biVal
}

func BigIntOrNil(sVal *string) *big.Int {
	var biVal *big.Int
	if sVal != nil {
		bi, ok := new(big.Int).SetString(*sVal, 0)
		if ok {
			biVal = bi
		}
	}
	return biVal
}

func DurationSeconds(sVal *string, min time.Duration, def string) int64 {
	d := DurationMin(sVal, min, def)
	return (int64)(math.Ceil(d.Seconds()))
}

func ByteSize(sVal *string, min int64, def string) int64 {
	var iVal *int64
	if sVal != nil {
		i, err := units.RAMInBytes(*sVal)
		if err == nil {
			iVal = &i
		}
	}
	if iVal == nil {
		i, _ := units.RAMInBytes(def)
		iVal = &i
	} else if *iVal < min {
		return min
	}
	return *iVal
}

func P[T any](v T) *T {
	return &v
}
