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

type PaladinConfig struct {
	DomainManagerConfig    `json:",inline"`
	PluginManagerConfig    `json:",inline"`
	TransportManagerConfig `json:",inline"`
	RegistryManagerConfig  `json:",inline"`
	KeyManagerConfig       `json:",inline"`
	Startup                StartupConfig          `json:"startup"`
	Log                    LogConfig              `json:"log"`
	Blockchain             EthClientConfig        `json:"blockchain"`
	DB                     DBConfig               `json:"db"`
	RPCServer              RPCServerConfig        `json:"rpcServer"`
	MetricsServer          MetricsServerConfig    `json:"metricsServer"`
	DebugServer            DebugServerConfig      `json:"debugServer"`
	StateStore             StateStoreConfig       `json:"statestore"`
	BlockIndexer           BlockIndexerConfig     `json:"blockIndexer"`
	TempDir                *string                `json:"tempDir"`
	TxManager              TxManagerConfig        `json:"txManager"`
	PrivateTxManager       PrivateTxManagerConfig `json:"privateTxManager"`
	PublicTxManager        PublicTxManagerConfig  `json:"publicTxManager"`
	IdentityResolver       IdentityResolverConfig `json:"identityResolver"`
	GroupManager           GroupManagerConfig     `json:"groupManager"`
}
