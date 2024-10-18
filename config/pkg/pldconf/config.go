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
	"context"
	"os"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/internal/msgs"

	"sigs.k8s.io/yaml" // because it supports JSON tags, and we embed our structs in the k8s operator
)

type PaladinConfig struct {
	DomainManagerConfig    `json:",inline"`
	PluginManagerConfig    `json:",inline"`
	TransportManagerConfig `json:",inline"`
	RegistryManagerConfig  `json:",inline"`
	KeyManagerConfig       `json:",inline"`
	Log                    LogConfig              `json:"log"`
	Blockchain             EthClientConfig        `json:"blockchain"`
	DB                     DBConfig               `json:"db"`
	RPCServer              RPCServerConfig        `json:"rpcServer"`
	StateStore             StateStoreConfig       `json:"statestore"`
	BlockIndexer           BlockIndexerConfig     `json:"blockIndexer"`
	TempDir                *string                `json:"tempDir"`
	TxManager              TxManagerConfig        `json:"txManager"`
	PrivateTxManager       PrivateTxManagerConfig `json:"privateTxManager"`
	PublicTxManager        PublicTxManagerConfig  `json:"publicTxManager"`
}

func ReadAndParseYAMLFile(ctx context.Context, filePath string, config interface{}) error {
	// Note we use the YAML parser (like Kubernetes) that handles json tags
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return i18n.NewError(ctx, msgs.MsgConfigFileMissing, filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return i18n.NewError(ctx, msgs.MsgConfigFileReadError, filePath, err.Error())
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return i18n.NewError(ctx, msgs.MsgConfigFileParseError, err.Error())
	}

	return nil
}
