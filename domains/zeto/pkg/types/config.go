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

type Config struct {
	FactoryAddress string            `json:"factoryAddress"`
	Libraries      map[string]string `json:"libraries"`
}

type DomainConfig struct {
	DomainContracts DomainContracts `yaml:"contracts"`
}

type DomainContracts struct {
	Factory         DomainContract   `yaml:"factory"`
	Implementations []DomainContract `yaml:"implementations"`
}

type DomainContract struct {
	Name            string         `yaml:"name"`
	CircuitId       string         `yaml:"circuitId"`
	ContractAddress string         `yaml:"address"`
	AbiAndBytecode  AbiAndBytecode `yaml:"abiAndBytecode"`
	Libraries       []string       `yaml:"libraries"`
	Cloneable       bool           `yaml:"cloneable"`
}

type AbiAndBytecode struct {
	Path string             `yaml:"path"`
	Json AbiAndBytecodeJSON `yaml:"json"`
}

type AbiAndBytecodeJSON struct {
	Abi      map[string]interface{} `yaml:"abi"`
	Bytecode string                 `yaml:"bytecode"`
}

var DomainConfigABI = &abi.ParameterArray{}

type DomainHandler = domain.DomainHandler[DomainConfig]
type ParsedTransaction = domain.ParsedTransaction[DomainConfig]
