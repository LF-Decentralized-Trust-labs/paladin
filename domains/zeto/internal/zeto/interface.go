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

package zeto

import (
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/common/pkg/domain"
)

type DomainInterface = domain.DomainInterface[ZetoDomainConfig]
type ParsedTransaction = domain.ParsedTransaction[ZetoDomainConfig]

func (z *Zeto) getInterface() DomainInterface {
	return DomainInterface{
		"constructor": {
			ABI: &abi.Entry{
				Type: abi.Constructor,
				Inputs: abi.ParameterArray{
					{Name: "from", Type: "string"},
					{Name: "depositVerifier", Type: "address"},
					{Name: "withdrawVerifier", Type: "address"},
					{Name: "verifier", Type: "address"},
				},
			},
		},
		"mint": {
			ABI: &abi.Entry{
				Name: "mint",
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
			Handler: &mintHandler{zeto: z},
		},
		"transfer": {
			ABI: &abi.Entry{
				Name: "transfer",
				Type: abi.Function,
				Inputs: abi.ParameterArray{
					{Name: "to", Type: "string"},
					{Name: "amount", Type: "uint256"},
				},
			},
			Handler: &transferHandler{zeto: z},
		},
	}
}

type ZetoConstructorParams struct {
	From             string `json:"from"`
	Verifier         string `json:"verifier"`
	DepositVerifier  string `json:"depositVerifier"`
	WithdrawVerifier string `json:"withdrawVerifier"`
}

type ZetoMintParams struct {
	To     string               `json:"to"`
	Amount *ethtypes.HexInteger `json:"amount"`
}

type ZetoTransferParams struct {
	To     string               `json:"to"`
	Amount *ethtypes.HexInteger `json:"amount"`
}
