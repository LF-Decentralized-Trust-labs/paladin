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

package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

// Intended to be embedded at root level of paladin config
type DomainManagerConfig struct {
	Domains       map[string]*DomainConfig   `json:"domains"`
	DomainManager DomainManagerManagerConfig `json:"domainManager"`
}

type DomainManagerManagerConfig struct {
	ContractCache CacheConfig `json:"contractCache"`
}

type DomainConfig struct {
	Init            DomainInitConfig `json:"init"`
	Plugin          PluginConfig     `json:"plugin"`
	Config          map[string]any   `json:"config"`
	RegistryAddress string           `json:"registryAddress"`
	AllowSigning    bool             `json:"allowSigning"`
	DefaultGasLimit *uint64          `json:"defaultGasLimit"`
}

var ContractCacheDefaults = &CacheConfig{
	Capacity: confutil.P(1000),
}

type DomainInitConfig struct {
	Retry RetryConfig `json:"retry"`
}
