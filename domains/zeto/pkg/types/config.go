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
)

// DomainFactoryConfig is the configuration for a Zeto domain
// to provision new domain instances based on a factory contract
type DomainFactoryConfig struct {
	FactoryAddress string            `json:"factoryAddress"`
	Libraries      map[string]string `json:"libraries"`
	TokenName      string            `json:"tokenName"`
	CircuitId      string            `json:"circuitId"`
}

// DomainInstanceConfig is the domain instance config, which are
// sent to the domain contract deployment request to be published
// on-chain. This must include sufficient information for a Paladin
// node to fully initialize the domain instance, based on only
// on-chain information.
type DomainInstanceConfig struct {
	TokenName string `json:"tokenName"`
	CircuitId string `json:"circuitId"`
}

// DomainInstanceConfigABI is the ABI for the DomainInstanceConfig,
// used to encode and decode the on-chain data for the domain config
var DomainInstanceConfigABI = &abi.ParameterArray{
	{Type: "string", Name: "tokenName"},
	{Type: "string", Name: "circuitId"},
}

type DomainHandler = domain.DomainHandler[DomainInstanceConfig]
type ParsedTransaction = domain.ParsedTransaction[DomainInstanceConfig]
