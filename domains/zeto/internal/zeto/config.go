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
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	"gopkg.in/yaml.v2"
)

type LocalDomainConfig struct {
	DomainContracts *LocalDomainConfigContracts `yaml:"contracts"`
}

type LocalDomainConfigContracts struct {
	Factory         *LocalDomainContract   `yaml:"factory"`
	Implementations []*LocalDomainContract `yaml:"implementations"`
}

type LocalDomainContract struct {
	Name            string `yaml:"name"`
	CircuitId       string `yaml:"circuitId"`
	ContractAddress string `yaml:"address"`
	Abi             string `yaml:"abi"`
}

func (d *LocalDomainConfig) getContractAbi(name string) (abi.ABI, error) {
	for _, contract := range d.DomainContracts.Implementations {
		if contract.Name == name {
			var contractAbi abi.ABI
			err := json.Unmarshal([]byte(contract.Abi), &contractAbi)
			if err != nil {
				return nil, err
			}
			return contractAbi, nil
		}
	}
	return nil, fmt.Errorf("contract %s not found", name)
}

// TODO: how should the "local configuration" be specified?
func loadLocalConfig() (*LocalDomainConfig, error) {
	configFile := os.Getenv("LOCAL_CONFIG")
	if configFile == "" {
		return nil, fmt.Errorf("LOCAL_CONFIG environment variable not set")
	}
	configTestBytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var config LocalDomainConfig
	err = yaml.Unmarshal(configTestBytes, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
