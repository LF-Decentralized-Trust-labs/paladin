/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ethclient

import (
	"fmt"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-signer/pkg/abi"
)

var (
	// See https://docs.soliditylang.org/en/v0.8.14/control-structures.html#revert
	// There default error for `revert("some error")` is a function Error(string)
	defaultError = &abi.Entry{
		Type: abi.Error,
		Name: "Error",
		Inputs: abi.ParameterArray{
			{
				Type: "string",
			},
		},
	}
	defaultErrorID = defaultError.FunctionSelectorBytes()
)

func ProtocolIDForReceipt(blockNumber, transactionIndex *fftypes.FFBigInt) string {
	if blockNumber != nil && transactionIndex != nil {
		return fmt.Sprintf("%.12d/%.6d", blockNumber.Int(), transactionIndex.Int())
	}
	return ""
}

func padHexData(hexString string) string {
	hexString = strings.TrimPrefix(hexString, "0x")
	if len(hexString)%2 == 1 {
		hexString = "0" + hexString
	}

	return hexString
}
