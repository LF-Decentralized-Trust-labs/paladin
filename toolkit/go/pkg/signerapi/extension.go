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

package signerapi

import "github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"

type ConfigNoExt pldconf.SignerConfig

// See signerapi.ExtensibleConfig
func (c *ConfigNoExt) KeyStoreConfig() *pldconf.KeyStoreConfig {
	return &c.KeyStore
}

// See signerapi.ExtensibleConfig
func (c *ConfigNoExt) KeyDerivationConfig() *pldconf.KeyDerivationConfig {
	return &c.KeyDerivation
}

// To enable extending of the default configuration that Paladin uses when embedding this module,
// with additional configuration that is specific to particular environments
type ExtensibleConfig interface {
	KeyStoreConfig() *pldconf.KeyStoreConfig
	KeyDerivationConfig() *pldconf.KeyDerivationConfig
}

type Extensions[C ExtensibleConfig] struct {
	KeyStoreFactories       map[string]KeyStoreFactory[C]
	InMemorySignerFactories map[string]InMemorySignerFactory[C]
}
