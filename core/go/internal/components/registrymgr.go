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

package components

import (
	"context"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
)

type RegistryNodeTransportEntry struct {
	Node             string
	Transport        string
	TransportDetails string
}

type RegistryManagerToRegistry interface {
	plugintk.RegistryAPI
	Initialized()
}

type RegistryManager interface {
	ManagerLifecycle
	ConfiguredRegistries() map[string]*PluginConfig
	RegistryRegistered(name string, id uuid.UUID, toRegistry RegistryManagerToRegistry) (fromRegistry plugintk.RegistryCallbacks, err error)
	GetNodeTransports(ctx context.Context, node string) ([]*RegistryNodeTransportEntry, error)
}
