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
	"math/big"
)

// PadHexBigUint returns the supplied buffer, with all the bytes to the left of the integer set to '0'
func PadHexBigUint(bi *big.Int, buff []byte) []byte {
	unPadded := bi.Abs(bi).Text(16)
	boundary := len(buff) - len(unPadded)
	for i := 0; i < len(buff); i++ {
		if i >= boundary {
			buff[i] = unPadded[i-boundary]
		} else {
			buff[i] = '0'
		}
	}
	return buff
}
