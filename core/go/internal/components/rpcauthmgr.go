/*
 * Copyright © 2025 Kaleido, Inc.
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

package components

import (
	"github.com/LFDT-Paladin/paladin/config/pkg/pldconf"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/rpcserver"
	"github.com/google/uuid"
)

// RPCAuthManagerToAuthorizer is the interface that the RPCAuthManager receives when registering an authorizer
type RPCAuthManagerToAuthorizer interface {
	plugintk.RPCAuthAPI
	Initialized()
}

// RPCAuthManager manages RPC authorization plugin configurations
type RPCAuthManager interface {
	ManagerLifecycle
	ConfiguredRPCAuthorizers() map[string]*pldconf.PluginConfig
	ConfiguredRPCAuthorizerConfig() string
	ConfiguredRPCAuthorizerConfigByName(name string) string
	RPCAuthorizerRegistered(name string, id uuid.UUID, toAuthorizer RPCAuthManagerToAuthorizer) (fromAuthorizer plugintk.RPCAuthCallbacks, err error)
	GetRPCAuthorizer(name string) rpcserver.Authorizer
}
