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

package types

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type DomainConfig struct {
	FactoryAddress string `json:"factoryAddress"`
}

var NotoConfigID_V0 = tktypes.MustParseHexBytes("0x00010000")

type NotoConfigInput_V0 struct {
	NotaryLookup string `json:"notaryLookup"`
}

var NotoConfigInputABI_V0 = &abi.ParameterArray{
	{Name: "notaryLookup", Type: "string"},
}

type NotoConfigOutput_V0 struct {
	NotaryLookup  string          `json:"notaryLookup"`
	NotaryAddress string          `json:"notaryAddress"`
	Variant       tktypes.Bytes32 `json:"variant"`
}

var NotoConfigOutputABI_V0 = &abi.ParameterArray{
	{Name: "notaryLookup", Type: "string"},
	{Name: "notaryAddress", Type: "address"},
	{Name: "variant", Type: "bytes32"},
}

var NotoTransactionData_V0 = tktypes.MustParseHexBytes("0x00010000")

type DomainHandler = domain.DomainHandler[NotoConfigOutput_V0]
type ParsedTransaction = domain.ParsedTransaction[NotoConfigOutput_V0]

var NotoVariantDefault = tktypes.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000000")
var NotoVariantSelfSubmit = tktypes.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000001")
