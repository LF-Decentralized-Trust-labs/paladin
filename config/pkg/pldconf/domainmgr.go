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
	"github.com/LFDT-Paladin/paladin/config/pkg/confutil"
)

// Intended to be embedded at root level of paladin config
type DomainManagerInlineConfig struct {
	Domains       map[string]*DomainConfig `json:"domains" configdefaults:"DomainsConfigDefaults"`
	DomainManager DomainManagerConfig      `json:"domainManager"`
}

type DomainManagerConfig struct {
	ContractCache CacheConfig `json:"contractCache"`
}

type DomainConfig struct {
	Init                 DomainInitConfig `json:"init"`
	Plugin               PluginConfig     `json:"plugin"`
	Config               map[string]any   `json:"config"`
	RegistryAddress      string           `json:"registryAddress"`
	AllowSigning         bool             `json:"allowSigning"`
	DefaultGasLimit      *uint64          `json:"defaultGasLimit"`
	FixedSigningIdentity string           `json:"fixedSigningIdentity"`
}

type DomainInitConfig struct {
	Retry RetryConfig `json:"retry"`
}

var DomainConfigDefaults = DomainConfig{
	Init: DomainInitConfig{
		Retry: GenericRetryDefaults.RetryConfig,
	},
}

var DomainManagerInlineConfigDefaults = DomainManagerInlineConfig{
	DomainManager: DomainManagerConfig{
		ContractCache: CacheConfig{
			Capacity: confutil.P(1000),
		},
	},
}
