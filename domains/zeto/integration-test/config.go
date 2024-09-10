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

package integration_test

type domainConfig struct {
	DomainContracts domainContracts `yaml:"contracts"`
}

type domainContracts struct {
	Factory         domainContract   `yaml:"factory"`
	Implementations []domainContract `yaml:"implementations"`
}

type domainContract struct {
	Name           string         `yaml:"name"`
	Verifier       string         `yaml:"verifier"`
	CircuitId      string         `yaml:"circuitId"`
	AbiAndBytecode abiAndBytecode `yaml:"abiAndBytecode"`
	Libraries      []string       `yaml:"libraries"`
	Cloneable      bool           `yaml:"cloneable"`
}

type abiAndBytecode struct {
	Path string `yaml:"path"`
}

type setImplementationParams struct {
	Name           string             `json:"name"`
	Implementation implementationInfo `json:"implementation"`
}

type implementationInfo struct {
	Implementation   string `json:"implementation"`
	Verifier         string `json:"verifier"`
	DepositVerifier  string `json:"depositVerifier"`
	WithdrawVerifier string `json:"withdrawVerifier"`
}
