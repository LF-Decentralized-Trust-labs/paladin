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

package tktypes

import (
	"crypto/rand"
	"encoding/hex"
)

var randReader = rand.Reader

func RandHex(count int) string {
	return hex.EncodeToString(RandBytes(count))
}

func RandBytes(count int) []byte {
	b := make([]byte, count)
	i, err := randReader.Read(b)
	if err != nil || i != count {
		panic(err)
	}
	return b
}
